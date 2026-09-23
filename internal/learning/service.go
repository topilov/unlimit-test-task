package learning

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"apm-investigator/internal/ai"
	"apm-investigator/internal/incident"
	"apm-investigator/internal/store"
	"github.com/google/uuid"
)

type Reflector interface {
	Reflect(context.Context, ai.ReflectionInput) (incident.Reflection, incident.Metadata, error)
}

type Service struct {
	Store *store.Store
	Agent Reflector
	Mode  string
}

type Result struct {
	Feedback incident.Feedback `json:"feedback"`
	Lesson   *incident.Lesson  `json:"lesson"`
}

func (s *Service) Reflect(ctx context.Context, id uuid.UUID) (Result, error) {
	f, err := s.Store.Feedback(ctx, id)
	if err != nil {
		return Result{}, err
	}
	if f.Reflection != nil {
		return s.result(ctx, id)
	}
	r, err := s.Store.Run(ctx, f.RunID)
	if err != nil {
		return Result{Feedback: f}, err
	}
	if r.Status != incident.RunCompleted || r.Result == nil || r.Mode != s.Mode {
		return Result{Feedback: f}, fmt.Errorf("reflection requires a completed run and the same AI mode")
	}
	i, err := s.Store.Get(ctx, r.IncidentID)
	if err != nil {
		return Result{Feedback: f}, err
	}
	evidence, err := s.Store.RunEvidence(ctx, r.ID)
	if err != nil {
		return Result{Feedback: f}, err
	}
	tools, err := s.Store.ToolRuns(ctx, r.ID)
	if err != nil {
		return Result{Feedback: f}, err
	}
	evidence = append(append([]incident.Evidence{}, r.Context.Evidence...), evidence...)
	tools = append(append([]incident.ToolRun{}, r.Context.ToolHistory...), tools...)
	input := ai.ReflectionInput{Context: r.Context, RunID: r.ID, Scope: i.Scope, Result: r.Result, Turns: r.Turns, Evidence: evidence, Tools: tools, Memory: r.Memory, Verdict: f.Verdict, Note: f.Note}
	reflection, meta, err := s.Agent.Reflect(ctx, input)
	if err == nil {
		err = Validate(reflection, input)
	}
	if err != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		saveErr := s.Store.ReflectionFailed(cleanup, f.ID, err, meta)
		f.Error, f.Metadata = err.Error(), meta
		return Result{Feedback: f}, errors.Join(err, saveErr)
	}
	if err := s.Store.SaveReflection(ctx, f, reflection, meta); err != nil {
		return Result{Feedback: f}, err
	}
	return s.result(ctx, id)
}

func (s *Service) result(ctx context.Context, id uuid.UUID) (Result, error) {
	f, err := s.Store.Feedback(ctx, id)
	if err != nil {
		return Result{}, err
	}
	l, err := s.Store.LessonForFeedback(ctx, id)
	return Result{Feedback: f, Lesson: l}, err
}

func Validate(result incident.Reflection, input ai.ReflectionInput) error {
	validText := func(text string, limit int) bool {
		return strings.TrimSpace(text) != "" && utf8.RuneCountInString(text) <= limit
	}
	if !validText(result.Summary, 1000) {
		return fmt.Errorf("%w: reflection summary", incident.ErrModelInvalidOutput)
	}
	if result.Lesson == nil {
		return nil
	}
	l := result.Lesson
	if !validText(l.Condition, 300) || !validText(l.Check, 600) || !validText(l.Rationale, 600) {
		return fmt.Errorf("%w: lesson text", incident.ErrModelInvalidOutput)
	}
	if len(l.EvidenceIDs) == 0 || len(l.EvidenceIDs) > 8 {
		return fmt.Errorf("%w: lesson needs 1..8 evidence references", incident.ErrModelInvalidOutput)
	}
	available := map[string]bool{}
	for _, e := range input.Evidence {
		historical := false
		for _, prior := range input.Context.Evidence {
			historical = historical || (prior.ID == e.ID && prior.RunID == e.RunID)
		}
		if (e.RunID == input.RunID || historical) && e.Available {
			available[e.ID] = true
		}
	}
	seen := map[string]bool{}
	for _, ref := range l.EvidenceIDs {
		if !available[ref] || seen[ref] {
			return fmt.Errorf("%w: unknown, unavailable or repeated lesson evidence", incident.ErrModelInvalidOutput)
		}
		seen[ref] = true
	}
	return nil
}
