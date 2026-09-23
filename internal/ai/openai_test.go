package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"apm-investigator/internal/incident"
	"github.com/openai/openai-go/v3/option"
)

func response(output []any) map[string]any {
	return map[string]any{"id": "resp_test", "status": "completed", "output": output, "usage": map[string]any{"input_tokens": 11, "output_tokens": 7, "input_tokens_details": map[string]int{"cached_tokens": 2}}}
}
func toolItem(name, args string) map[string]any {
	return map[string]any{"type": "function_call", "name": name, "arguments": args, "call_id": "call_1", "status": "completed"}
}
func textItem(text string) map[string]any {
	return map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": text}}}
}
func adapter(t *testing.T, handler http.HandlerFunc, timeout time.Duration) (*OpenAI, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	a, err := NewOpenAI("test-secret", "gpt-5.6-terra", "medium", timeout, option.WithBaseURL(server.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	return a, server
}
func TestRequestShapeAndToolParsing(t *testing.T) {
	a, _ := adapter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("wrong API: %s", r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["store"] != false || body["parallel_tool_calls"] != false || body["model"] != "gpt-5.6-terra" {
			t.Errorf("bad request controls: %+v", body)
		}
		defs := body["tools"].([]any)
		if len(defs) != 4 {
			t.Errorf("tools=%d", len(defs))
		}
		for _, def := range defs {
			d := def.(map[string]any)
			if d["strict"] != true || d["type"] != "function" {
				t.Error("tools not strict")
			}
		}
		format := body["text"].(map[string]any)["format"].(map[string]any)
		if format["type"] != "json_schema" || format["strict"] != true {
			t.Error("final output not strict")
		}
		if !strings.Contains(body["instructions"].(string), "untrusted evidence") {
			t.Error("missing trust boundary")
		}
		json.NewEncoder(w).Encode(response([]any{toolItem("get_queue_health", `{"arguments":{},"hypotheses":[],"open_questions":[]}`)}))
	}, time.Second)
	turn, meta, err := a.Next(context.Background(), Input{})
	if err != nil {
		t.Fatal(err)
	}
	if turn.Kind != "tool_call" || turn.ToolCall.Name != "get_queue_health" || meta.Attempts != 1 || meta.InputTokens != 11 || meta.OutputTokens != 7 {
		t.Fatalf("%+v %+v", turn, meta)
	}
}
func TestLastStepRequestsFinalOnly(t *testing.T) {
	result := incident.Result{Summary: "Unknown", RootCauseStatus: incident.CauseUnknown}
	a, _ := adapter(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["tools"] != nil {
			t.Errorf("last step still offers tools: %v", body["tools"])
		}
		var input Input
		if err := json.Unmarshal([]byte(body["input"].(string)), &input); err != nil || input.StepsRemaining != 1 {
			t.Errorf("missing step budget: %+v, %v", input, err)
		}
		json.NewEncoder(w).Encode(response([]any{textItem(string(testJSON(t, result)))}))
	}, time.Second)
	turn, _, err := a.Next(context.Background(), Input{StepsRemaining: 1})
	if err != nil || turn.Kind != incident.TurnFinal {
		t.Fatal(err, turn)
	}
}
func TestFinalParsingWithoutReasoningPersistence(t *testing.T) {
	r := incident.Result{Summary: "Unknown", RootCauseStatus: "unknown"}
	a, _ := adapter(t, func(w http.ResponseWriter, rq *http.Request) {
		json.NewEncoder(w).Encode(response([]any{map[string]any{"type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": "PRIVATE_SENTINEL"}}}, textItem(string(testJSON(t, r)))}))
	}, time.Second)
	turn, _, err := a.Next(context.Background(), Input{})
	if err != nil || turn.Final == nil || turn.Final.Summary != "Unknown" {
		t.Fatal(err, turn)
	}
	if strings.Contains(string(testJSON(t, turn)), "PRIVATE_SENTINEL") {
		t.Fatal("reasoning persisted")
	}
}
func TestInvalidResponses(t *testing.T) {
	for _, tc := range []struct {
		name string
		body any
	}{
		{"refusal", response([]any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "refusal", "refusal": "no"}}}})},
		{"incomplete", map[string]any{"status": "incomplete", "output": []any{}}},
		{"invalid json", response([]any{textItem(`{invalid`)})},
		{"unknown final property", response([]any{textItem(`{"unsafe":"refund"}`)})},
		{"multiple calls", response([]any{toolItem("get_queue_health", `{"arguments":{}}`), toolItem("get_queue_health", `{"arguments":{}}`)})},
		{"call and final", response([]any{toolItem("get_queue_health", `{"arguments":{}}`), textItem(`{}`)})},
		{"invalid args", response([]any{toolItem("get_queue_health", `{"arguments":null}`)})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := adapter(t, func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(tc.body) }, time.Second)
			_, _, err := a.Next(context.Background(), Input{})
			if !errors.Is(err, incident.ErrModelInvalidOutput) {
				t.Fatalf("wanted invalid output: %v", err)
			}
		})
	}
}
func TestRetryBudgetAndNoSecretErrors(t *testing.T) {
	for _, status := range []int{429, 500, 401} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			a, _ := adapter(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				w.Write([]byte(`{"error":{"message":"test-secret","type":"server_error"}}`))
			}, time.Second)
			_, meta, err := a.Next(context.Background(), Input{})
			want := int32(2)
			if status == 401 {
				want = 1
			}
			if calls.Load() != want || meta.Attempts != int(want) || !errors.Is(err, incident.ErrModelUnavailable) {
				t.Fatal(calls.Load(), meta, err)
			}
			if strings.Contains(err.Error(), "test-secret") {
				t.Fatal("secret in error")
			}
		})
	}
}
func TestTransientRetryThenSuccess(t *testing.T) {
	var calls atomic.Int32
	a, _ := adapter(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(503)
			return
		}
		json.NewEncoder(w).Encode(response([]any{toolItem("get_queue_health", `{"arguments":{},"hypotheses":[],"open_questions":[]}`)}))
	}, time.Second)
	_, meta, err := a.Next(context.Background(), Input{})
	if err != nil || meta.Attempts != 2 {
		t.Fatal(err, meta)
	}
}
func TestTimeoutAndMissingKey(t *testing.T) {
	if _, err := NewOpenAI("", "model", "medium", time.Second); err == nil {
		t.Fatal("missing key accepted")
	}
	a, _ := adapter(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	}, 10*time.Millisecond)
	_, meta, err := a.Next(context.Background(), Input{})
	if !errors.Is(err, incident.ErrModelUnavailable) || meta.Attempts != 1 {
		t.Fatal(err, meta)
	}
}

func testJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
