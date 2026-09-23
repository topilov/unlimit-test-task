package learning

import (
	"strings"
	"testing"

	"apm-investigator/internal/ai"
	"apm-investigator/internal/incident"
	"github.com/google/uuid"
)

func TestReflectionValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*incident.Reflection, *ai.ReflectionInput)
	}{
		{"blank summary", func(r *incident.Reflection, _ *ai.ReflectionInput) { r.Summary = " " }},
		{"long check", func(r *incident.Reflection, _ *ai.ReflectionInput) { r.Lesson.Check = strings.Repeat("x", 601) }},
		{"missing evidence", func(r *incident.Reflection, _ *ai.ReflectionInput) { r.Lesson.EvidenceIDs = nil }},
		{"invented evidence", func(r *incident.Reflection, _ *ai.ReflectionInput) { r.Lesson.EvidenceIDs = []string{"E999"} }},
		{"duplicate evidence", func(r *incident.Reflection, _ *ai.ReflectionInput) { r.Lesson.EvidenceIDs = []string{"E1", "E1"} }},
		{"unavailable evidence", func(_ *incident.Reflection, i *ai.ReflectionInput) { i.Evidence[0].Available = false }},
		{"foreign run", func(_ *incident.Reflection, i *ai.ReflectionInput) { i.Evidence[0].RunID = uuid.New() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := uuid.New()
			input := ai.ReflectionInput{RunID: run, Evidence: []incident.Evidence{{ID: "E1", RunID: run, Available: true}}}
			result := incident.Reflection{Summary: "Useful observation", Lesson: &incident.LessonDraft{Condition: "No attempts", Check: "Inspect queue", Rationale: "Find dispatch failures", EvidenceIDs: []string{"E1"}}}
			if err := Validate(result, input); err != nil {
				t.Fatal("positive control", err)
			}
			tc.mutate(&result, &input)
			if err := Validate(result, input); err == nil {
				t.Fatal("invalid reflection accepted")
			}
		})
	}
	if err := Validate(incident.Reflection{Summary: "Feedback is too vague to generalize"}, ai.ReflectionInput{}); err != nil {
		t.Fatal(err)
	}
}
