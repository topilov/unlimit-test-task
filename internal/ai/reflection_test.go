package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"apm-investigator/internal/incident"
	"github.com/google/uuid"
)

func TestReflectionRequestHasNoToolsAndReturnsPublicLesson(t *testing.T) {
	want := incident.Reflection{Summary: "Inspect dispatch first", Lesson: &incident.LessonDraft{Condition: "No attempts", Check: "Inspect queue", Rationale: "Dispatch precedes delivery", EvidenceIDs: []string{"E003"}}}
	a, _ := adapter(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if tools, ok := body["tools"].([]any); ok && len(tools) != 0 {
			t.Error("reflection has tool access")
		}
		if body["store"] != false {
			t.Error("response storage enabled")
		}
		format := body["text"].(map[string]any)["format"].(map[string]any)
		if format["name"] != "investigation_reflection" || format["strict"] != true {
			t.Error(format)
		}
		if !strings.Contains(body["instructions"].(string), "lesson=null") {
			t.Error("reflection forced to invent a lesson")
		}
		var input map[string]any
		if err := json.Unmarshal([]byte(body["input"].(string)), &input); err != nil {
			t.Fatal(err)
		}
		if input["feedback"] != "Check dispatch before blaming the receiver" {
			t.Error(input)
		}
		for _, forbidden := range []string{"scenario", "expected", "mock_turns", "future_events"} {
			if _, ok := input[forbidden]; ok {
				t.Error("leaked", forbidden)
			}
		}
		json.NewEncoder(w).Encode(response([]any{map[string]any{"type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": "PRIVATE_SENTINEL"}}}, textItem(string(testJSON(t, want)))}))
	}, time.Second)
	got, meta, err := a.Reflect(context.Background(), ReflectionInput{RunID: uuid.New(), Verdict: incident.FeedbackIncorrect, Note: "Check dispatch before blaming the receiver"})
	if err != nil || got.Lesson == nil || got.Lesson.Check != want.Lesson.Check || meta.Attempts != 1 {
		t.Fatal(got, meta, err)
	}
	if strings.Contains(string(testJSON(t, got)), "PRIVATE_SENTINEL") {
		t.Fatal("private reasoning persisted")
	}
}

func TestReflectionRejectsToolCallsAndInvalidResults(t *testing.T) {
	for _, output := range [][]any{
		{toolItem("get_queue_health", `{}`)},
		{textItem(`{"summary":"ok","lesson":null,"extra":true}`)},
		{textItem(`{"summary":"ok","lesson":null}`), textItem(`{}`)},
		{map[string]any{"type": "message", "content": []any{map[string]any{"type": "refusal", "refusal": "no"}}}},
	} {
		a, _ := adapter(t, func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(response(output)) }, time.Second)
		if _, _, err := a.Reflect(context.Background(), ReflectionInput{}); !errors.Is(err, incident.ErrModelInvalidOutput) {
			t.Fatal(err)
		}
	}
}

func TestApprovedMemoryIsSeparateFromEvidenceInRequest(t *testing.T) {
	id := uuid.New()
	a, _ := adapter(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		var input Input
		json.Unmarshal([]byte(body["input"].(string)), &input)
		if len(input.Memory) != 1 || input.Memory[0].ID != id || len(input.Evidence) != 0 {
			t.Error(input)
		}
		json.NewEncoder(w).Encode(response([]any{toolItem("get_queue_health", `{"arguments":{},"hypotheses":[],"open_questions":[]}`)}))
	}, time.Second)
	_, _, err := a.Next(context.Background(), Input{Memory: []incident.MemoryRule{{ID: id, Condition: "No attempts", Check: "Inspect queue", Rationale: "Check dispatch"}}})
	if err != nil {
		t.Fatal(err)
	}
}
