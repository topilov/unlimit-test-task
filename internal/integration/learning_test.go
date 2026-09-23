package integration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"apm-investigator/internal/ai"
	"apm-investigator/internal/incident"
	"apm-investigator/internal/investigation"
	"apm-investigator/internal/learning"
	"apm-investigator/internal/store"
	"github.com/google/uuid"
)

type reflectorFunc func(context.Context, ai.ReflectionInput) (incident.Reflection, incident.Metadata, error)

func (f reflectorFunc) Reflect(ctx context.Context, input ai.ReflectionInput) (incident.Reflection, incident.Metadata, error) {
	return f(ctx, input)
}

func completedRun(t *testing.T, s *store.Store) incident.Run {
	t.Helper()
	out, err := runner(s).Run(context.Background(), "queue-delay", uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := s.Runs(context.Background(), out.Incident.ID)
	if err != nil || len(runs) != 1 {
		t.Fatal(runs, err)
	}
	return runs[0]
}

func feedbackFor(t *testing.T, s *store.Store, r incident.Run) incident.Feedback {
	t.Helper()
	f, err := s.AddFeedback(context.Background(), r.ID, incident.FeedbackHelpful, "Checking dispatch first was useful; preserve this diagnostic ordering.")
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func scriptedReflection() ai.MockReflection {
	return ai.MockReflection{Path: "../../testdata/learning/reflection.json"}
}

func TestIntegrationLearningLifecycleAndTraceIsolation(t *testing.T) {
	s, dsn := database(t)
	ctx := context.Background()
	source := completedRun(t, s)
	f := feedbackFor(t, s, source)
	i, err := s.Get(ctx, source.IncidentID)
	if err != nil {
		t.Fatal(err)
	}
	e := note(i)
	e.Payload = json.RawMessage(`{"text":"NEW_RUN_ONLY"}`)
	if _, err := s.Ingest(ctx, e, i.Clock, i.Cursor); err != nil {
		t.Fatal(err)
	}
	if _, err := runner(s).Run(ctx, i.Scenario, i.ID); err != nil {
		t.Fatal(err)
	}
	var reflections int
	agent := reflectorFunc(func(ctx context.Context, input ai.ReflectionInput) (incident.Reflection, incident.Metadata, error) {
		reflections++
		if input.RunID != source.ID || len(input.Tools) != 2 || len(input.Turns) != 3 {
			t.Fatal("wrong source trace", input)
		}
		for _, evidence := range input.Evidence {
			if evidence.RunID != source.ID {
				t.Fatal("cross-run evidence")
			}
		}
		data, err := json.Marshal(input)
		if err != nil || strings.Contains(string(data), "NEW_RUN_ONLY") {
			t.Fatal("later evidence leaked", err)
		}
		return scriptedReflection().Reflect(ctx, input)
	})
	service := learning.Service{Store: s, Agent: agent, Mode: "mock"}
	result, err := service.Reflect(ctx, f.ID)
	if err != nil || result.Lesson == nil || result.Lesson.Status != incident.LessonPending {
		t.Fatal(result, err)
	}
	lessonID := result.Lesson.ID
	if _, err := service.Reflect(ctx, f.ID); err != nil || reflections != 1 {
		t.Fatal("completed reflection repeated", err, reflections)
	}
	pending := completedRun(t, s)
	if len(pending.Memory) != 0 {
		t.Fatal("unapproved lesson used")
	}
	if _, err := s.SetLessonStatus(ctx, lessonID, incident.LessonActive); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetLessonStatus(ctx, lessonID, incident.LessonActive); err != nil {
		t.Fatal("repeat activation", err)
	}
	current, registry := newInvestigating(t, s)
	mock, err := ai.LoadMock("../../testdata/scenarios/queue_delay/mock_turns.json")
	if err != nil {
		t.Fatal(err)
	}
	turns := 0
	investigator := agentFunc(func(ctx context.Context, input ai.Input) (incident.Turn, incident.Metadata, error) {
		turns++
		if len(input.Memory) != 1 || input.Memory[0].ID != lessonID {
			t.Fatal("memory snapshot changed", input.Memory)
		}
		if turns == 1 {
			if _, err := s.SetLessonStatus(ctx, lessonID, incident.LessonDisabled); err != nil {
				t.Fatal(err)
			}
		}
		return mock.Next(ctx, input)
	})
	investigationService := investigation.Service{Store: s, Agent: investigator, Registry: registry, MaxSteps: 6, Mode: "mock"}
	if _, err := investigationService.Investigate(ctx, current.ID); err != nil {
		t.Fatal(err)
	}
	if turns != 3 {
		t.Fatal(turns)
	}
	reloaded, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	runs, err := reloaded.Runs(ctx, current.ID)
	if err != nil || len(runs[0].Memory) != 1 || runs[0].Memory[0].ID != lessonID {
		t.Fatal("memory snapshot lost", runs, err)
	}
	later := completedRun(t, s)
	if len(later.Memory) != 0 {
		t.Fatal("disabled lesson used")
	}
	if _, err := s.SetLessonStatus(ctx, lessonID, incident.LessonActive); err == nil {
		t.Fatal("disabled rule reactivated")
	}
	old, err := s.Run(ctx, source.ID)
	if err != nil || len(old.Memory) != 0 || old.Result.Summary != source.Result.Summary {
		t.Fatal("original run changed", old, err)
	}
	entries, err := s.Timeline(ctx, source.IncidentID)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, entry := range entries {
		kinds[entry.Kind]++
	}
	for _, kind := range []string{"feedback_added", "feedback_reflected", "lesson_proposed", "lesson_active", "lesson_disabled"} {
		if kinds[kind] != 1 {
			t.Fatal("incomplete or duplicate audit", kind, kinds)
		}
	}
}

func TestIntegrationMemoryScopeModeBaselineAndLimit(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	source := completedRun(t, s)
	service := learning.Service{Store: s, Agent: scriptedReflection(), Mode: "mock"}
	for n := 0; n < 6; n++ {
		f := feedbackFor(t, s, source)
		result, err := service.Reflect(ctx, f.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.SetLessonStatus(ctx, result.Lesson.ID, incident.LessonActive); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name, mode string
		memory     bool
		change     func(*incident.Incident)
		want       int
	}{
		{"matching", "mock", true, func(*incident.Incident) {}, 5},
		{"baseline", "mock", false, func(*incident.Incident) {}, 0},
		{"live", "live", true, func(*incident.Incident) {}, 0},
		{"merchant", "mock", true, func(i *incident.Incident) { i.Scope.MerchantID = "other" }, 0},
		{"method", "mock", true, func(i *incident.Incident) { i.Scope.Method = "other" }, 0},
		{"environment", "mock", true, func(i *incident.Incident) { i.Scope.Environment = "other" }, 0},
		{"earlier window", "mock", true, func(i *incident.Incident) {
			i.Scope.WindowStart = i.Scope.WindowStart.Add(-time.Hour)
			i.Scope.WindowEnd = i.Scope.WindowEnd.Add(-time.Hour)
		}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original, err := s.Get(ctx, source.IncidentID)
			if err != nil {
				t.Fatal(err)
			}
			original.ID, original.Revision, original.Status = uuid.New(), 1, incident.Investigating
			tc.change(&original)
			if err := s.Create(ctx, original); err != nil {
				t.Fatal(err)
			}
			r, err := s.StartRun(ctx, original, tc.mode, tc.memory)
			if err != nil || len(r.Memory) != tc.want {
				t.Fatal(r, err)
			}
		})
	}
}

func TestIntegrationMemoryExcludesLaterReinvestigation(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	original := completedRun(t, s)
	i, err := s.Get(ctx, original.IncidentID)
	if err != nil {
		t.Fatal(err)
	}
	future := i.Clock.Add(time.Hour)
	event := note(i)
	event.OccurredAt, event.AvailableAt = future, future
	if _, err := s.Ingest(ctx, event, future, i.Cursor); err != nil {
		t.Fatal(err)
	}
	i, err = s.Get(ctx, i.ID)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.StartRun(ctx, i, "mock", true)
	if err != nil {
		t.Fatal(err)
	}
	evidence := incident.Evidence{ID: "E999", IncidentID: i.ID, RunID: r.ID, Source: "manual", Kind: "manual.note", ObservedAt: future, Available: true, Data: json.RawMessage(`{"text":"late diagnostic evidence"}`)}
	if err := s.RecordEvidence(ctx, i, evidence); err != nil {
		t.Fatal(err)
	}
	r.Status = incident.RunCompleted
	r.Result = &incident.Result{Summary: "Further diagnosis needed", RecommendedAction: incident.ActionContinue}
	if _, err := s.Finish(ctx, i, r); err != nil {
		t.Fatal(err)
	}
	service := learning.Service{Store: s, Mode: "mock", Agent: reflectorFunc(func(context.Context, ai.ReflectionInput) (incident.Reflection, incident.Metadata, error) {
		return incident.Reflection{Summary: "A diagnostic check", Lesson: &incident.LessonDraft{
			Condition: "When delivery is delayed", Check: "Check dispatch", Rationale: "Separate sending from receiving", EvidenceIDs: []string{evidence.ID},
		}}, incident.Metadata{}, nil
	})}
	result, err := service.Reflect(ctx, feedbackFor(t, s, r).ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetLessonStatus(ctx, result.Lesson.ID, incident.LessonActive); err != nil {
		t.Fatal(err)
	}
	earlier := completedRun(t, s)
	if len(earlier.Memory) != 0 {
		t.Fatal("later investigation leaked into earlier replay", earlier.Memory)
	}
	later, _ := newInvestigating(t, s)
	event = note(later)
	event.OccurredAt, event.AvailableAt = future, future
	if _, err := s.Ingest(ctx, event, future, later.Cursor); err != nil {
		t.Fatal(err)
	}
	later, err = s.Get(ctx, later.ID)
	if err != nil {
		t.Fatal(err)
	}
	laterRun, err := s.StartRun(ctx, later, "mock", true)
	if err != nil || len(laterRun.Memory) != 1 || laterRun.Memory[0].ID != result.Lesson.ID {
		t.Fatal("eligible lesson excluded", laterRun.Memory, err)
	}
}

func TestIntegrationReflectionFailureRetryAndNoLesson(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	r := completedRun(t, s)
	f := feedbackFor(t, s, r)
	service := learning.Service{Store: s, Mode: "mock", Agent: reflectorFunc(func(context.Context, ai.ReflectionInput) (incident.Reflection, incident.Metadata, error) {
		return incident.Reflection{}, incident.Metadata{Attempts: 1}, incident.ErrModelUnavailable
	})}
	if _, err := service.Reflect(ctx, f.ID); !errors.Is(err, incident.ErrModelUnavailable) {
		t.Fatal(err)
	}
	saved, err := s.Feedback(ctx, f.ID)
	if err != nil || saved.Error == "" || saved.Reflection != nil || saved.Note != f.Note {
		t.Fatal(saved, err)
	}
	service.Agent = reflectorFunc(func(context.Context, ai.ReflectionInput) (incident.Reflection, incident.Metadata, error) {
		return incident.Reflection{Summary: "No reusable lesson is supported"}, incident.Metadata{}, nil
	})
	result, err := service.Reflect(ctx, f.ID)
	if err != nil || result.Feedback.Reflection == nil || result.Feedback.Error != "" || result.Lesson != nil {
		t.Fatal(result, err)
	}
	f = feedbackFor(t, s, r)
	service.Mode = "live"
	if _, err := service.Reflect(ctx, f.ID); err == nil {
		t.Fatal("cross-mode reflection accepted")
	}
	i, _ := newInvestigating(t, s)
	active, err := s.StartRun(ctx, i, "mock", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFeedback(ctx, active.ID, incident.FeedbackHelpful, "not finished"); err == nil {
		t.Fatal("unfinished run accepted")
	}
	if _, err := s.AddFeedback(ctx, r.ID, incident.FeedbackHelpful, " "); err == nil {
		t.Fatal("blank feedback accepted")
	}
}

func TestIntegrationConcurrentReflectionCreatesOneLesson(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	r := completedRun(t, s)
	f := feedbackFor(t, s, r)
	var calls atomic.Int32
	ready := make(chan struct{})
	service := learning.Service{Store: s, Mode: "mock", Agent: reflectorFunc(func(ctx context.Context, input ai.ReflectionInput) (incident.Reflection, incident.Metadata, error) {
		if calls.Add(1) == 2 {
			close(ready)
		}
		select {
		case <-ready:
		case <-ctx.Done():
			return incident.Reflection{}, incident.Metadata{}, ctx.Err()
		}
		return scriptedReflection().Reflect(ctx, input)
	})}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	results := make(chan learning.Result, 2)
	errs := make(chan error, 2)
	for n := 0; n < 2; n++ {
		wg.Add(1)
		go func() { defer wg.Done(); result, err := service.Reflect(ctx, f.ID); results <- result; errs <- err }()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	id := uuid.Nil
	for result := range results {
		if result.Lesson == nil {
			t.Fatal(result)
		}
		if id != uuid.Nil && id != result.Lesson.ID {
			t.Fatal("different lessons returned")
		}
		id = result.Lesson.ID
	}
	lessons, err := s.Lessons(ctx)
	if err != nil || len(lessons) != 1 {
		t.Fatal(lessons, err)
	}
}
