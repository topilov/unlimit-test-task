package investigation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"apm-investigator/internal/ai"
	"apm-investigator/internal/incident"
	"apm-investigator/internal/store"
	"apm-investigator/internal/tools"
	"github.com/google/uuid"
)

const diagnosticTimeout = 5 * time.Second
const persistenceCleanupTimeout = 5 * time.Second

type Agent interface {
	Next(context.Context, ai.Input) (incident.Turn, incident.Metadata, error)
}

type Service struct {
	Store         *store.Store
	Agent         Agent
	Registry      *tools.Registry
	MaxSteps      int
	Mode          string
	Logger        *slog.Logger
	WithoutMemory bool
}

func (s *Service) Investigate(ctx context.Context, id uuid.UUID) (*incident.Proposal, error) {
	i, err := s.Store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	registry := *s.Registry
	registry.Now, registry.Cohort = i.Clock, i.Cohort
	run, err := s.Store.StartRun(ctx, i, s.Mode, !s.WithoutMemory)
	if err != nil {
		return nil, err
	}
	input := ai.Input{
		RunID:              run.ID,
		Context:            run.Context,
		AsOf:               run.Context.AsOf,
		PreviousRunID:      run.Context.PreviousRunID,
		Memory:             run.Memory,
		IncidentID:         id,
		Revision:           i.Revision,
		Scope:              i.Scope,
		Impact:             i.Impact,
		Cohort:             i.Cohort,
		Evidence:           append([]incident.Evidence{}, run.Context.Evidence...),
		PreviousHypotheses: run.Context.Hypotheses,
		OpenQuestions:      run.Context.OpenQuestions,
		ToolHistory:        append([]incident.ToolRun{}, run.Context.ToolHistory...),
	}
	n := 0
	addEvidence := func(e incident.Evidence) incident.Evidence {
		n++
		e.ID = fmt.Sprintf("E%03d", n)
		e.IncidentID = i.ID
		e.RunID = run.ID
		return e
	}
	fail := func(cause error) (*incident.Proposal, error) {
		run.Status = incident.RunIncomplete
		run.Error = cause.Error()
		run.Result = &incident.Result{
			Summary:         "Investigation incomplete",
			RootCauseStatus: incident.CauseUnknown,
			Hypotheses:      input.PreviousHypotheses,
			OpenQuestions:   input.OpenQuestions,
		}
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), persistenceCleanupTimeout)
		defer cancel()
		p, e := s.Store.Finish(cleanup, i, run)
		if e != nil {
			return p, errors.Join(cause, e)
		}
		return p, cause
	}

	prior, err := s.Store.Evidence(ctx, id)
	if err != nil {
		return fail(err)
	}
	n = len(prior)
	events, err := s.Store.Events(ctx, id)
	if err != nil {
		return fail(err)
	}
	base, err := baseEvidence(i, registry.Sources)
	if err != nil {
		return fail(err)
	}
	base = addEvidence(base)
	if err = s.Store.RecordEvidence(ctx, i, base); err != nil {
		return fail(err)
	}
	input.Evidence = append(input.Evidence, base)
	for _, event := range events {
		if event.AvailableAt.After(i.Clock) || hasEvent(input.Evidence, event.ID) {
			continue
		}
		e := addEvidence(incident.Evidence{
			EventID:    event.ID,
			Source:     "event",
			Kind:       event.Kind,
			ObservedAt: event.OccurredAt,
			Available:  true,
			Summary:    "Untrusted source event: " + event.Kind,
			Data:       event.Payload,
		})
		if err = s.Store.RecordEvidence(ctx, i, e); err != nil {
			return fail(err)
		}
		input.Evidence = append(input.Evidence, e)
	}
	for step := 1; step <= s.MaxSteps; step++ {
		input.StepsRemaining = s.MaxSteps - step + 1
		turn, meta, callErr := s.Agent.Next(ctx, input)
		if callErr == nil {
			callErr = ValidateTurn(turn, input)
		}
		mt := incident.ModelTurn{Step: step, Turn: turn, Metadata: meta}
		if callErr != nil {
			mt.Error = callErr.Error()
		}
		run.Turns = append(run.Turns, mt)
		if err = s.Store.RecordTurn(ctx, i, run, mt); err != nil {
			return fail(err)
		}
		if s.Logger != nil {
			s.Logger.Info("model turn", "incident_id", id, "run_id", run.ID, "mode", s.Mode, "model", meta.Model, "step", step, "duration_ms", meta.DurationMS)
		}
		if callErr != nil {
			return fail(callErr)
		}
		input.PreviousHypotheses = turn.Hypotheses
		input.OpenQuestions = turn.OpenQuestions
		if turn.Kind == incident.TurnFinal {
			run.Status = incident.RunCompleted
			run.Result = turn.Final
			p, finishErr := s.Store.Finish(ctx, i, run)
			if finishErr != nil && !errors.Is(finishErr, incident.ErrStaleRevision) {
				return fail(finishErr)
			}
			return p, finishErr
		}
		call := *turn.ToolCall
		tr := incident.ToolRun{
			ID:        uuid.New(),
			RunID:     run.ID,
			Step:      step,
			Name:      call.Name,
			Arguments: call.Arguments,
			StartedAt: time.Now().UTC(),
		}
		toolCtx, cancel := context.WithTimeout(ctx, diagnosticTimeout)
		e, toolErr := registry.Execute(toolCtx, i.Scope, call)
		cancel()
		tr.FinishedAt = time.Now().UTC()
		var persisted *incident.Evidence
		if toolErr != nil {
			tr.Status = incident.ToolFailed
			tr.Error = toolErr.Error()
		} else {
			tr.Status = incident.ToolSucceeded
			e = addEvidence(e)
			tr.EvidenceRef = e.ID
			persisted = &e
			input.Evidence = append(input.Evidence, e)
		}
		if err = s.Store.RecordTool(ctx, i, tr, persisted); err != nil {
			return fail(err)
		}
		input.ToolHistory = append(input.ToolHistory, tr)
	}
	return fail(incident.ErrStepLimitExceeded)
}
