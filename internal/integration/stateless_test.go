package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"apm-investigator/internal/ai"
	"apm-investigator/internal/incident"
	"apm-investigator/internal/investigation"
	"github.com/openai/openai-go/v3/option"
)

func TestIntegrationStatelessSDK(t *testing.T) {
	s, _ := database(t)
	i, registry := newInvestigating(t, s)
	mock, err := ai.LoadMock("../../testdata/scenarios/queue_delay/mock_turns.json")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]json.RawMessage
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		var snapshot string
		if err := json.Unmarshal(body["input"], &snapshot); err != nil {
			t.Error("not a stateless string snapshot", err)
			w.WriteHeader(400)
			return
		}
		var input ai.Input
		if err := json.Unmarshal([]byte(snapshot), &input); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if string(body["store"]) != "false" || body["previous_response_id"] != nil {
			t.Error("unexpected state control")
		}
		calls++
		if len(input.ToolHistory) != calls-1 {
			t.Errorf("request %d tool history=%d", calls, len(input.ToolHistory))
		}
		if calls > 1 && len(input.PreviousHypotheses) == 0 {
			t.Error("lost previous hypotheses within the run")
		}
		for _, tool := range input.ToolHistory {
			matched := false
			for _, evidence := range input.Evidence {
				if evidence.ID == tool.EvidenceRef && evidence.RunID == input.RunID && evidence.IncidentID == i.ID && len(evidence.Data) > 0 {
					matched = true
				}
			}
			if !matched || tool.Status != incident.ToolSucceeded {
				t.Error("actual tool result missing", tool)
			}
		}
		t.Logf("SDK request %d: actual tools=%d evidence=%d previous hypotheses=%d", calls, len(input.ToolHistory), len(input.Evidence), len(input.PreviousHypotheses))
		turn, _, err := mock.Next(req.Context(), input)
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		var item any
		if turn.ToolCall != nil {
			args, err := json.Marshal(map[string]any{"arguments": turn.ToolCall.Arguments, "hypotheses": turn.Hypotheses, "open_questions": turn.OpenQuestions})
			if err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
			item = map[string]any{"type": "function_call", "name": turn.ToolCall.Name, "call_id": fmt.Sprintf("call_%d", calls), "arguments": string(args), "status": "completed"}
		} else {
			result, err := json.Marshal(turn.Final)
			if err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
			item = map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": string(result)}}}
		}
		json.NewEncoder(w).Encode(map[string]any{"id": fmt.Sprintf("resp_%d", calls), "status": "completed", "output": []any{item}})
	}))
	defer server.Close()
	agent, err := ai.NewOpenAI("local-test-only", "gpt-5.6-terra", "medium", time.Second, option.WithBaseURL(server.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	service := investigation.Service{Store: s, Registry: registry, Agent: agent, Mode: "live", MaxSteps: 6}
	proposal, err := service.Investigate(context.Background(), i.ID)
	if err != nil || proposal == nil || calls != 3 {
		t.Fatal("multi-request loop failed", calls, proposal, err)
	}
	t.Log("three real SDK requests against localhost, two real tool executions, final result saved; no external model used")
}
