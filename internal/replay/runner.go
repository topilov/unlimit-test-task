package replay

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"apm-investigator/internal/ai"
	"apm-investigator/internal/config"
	"apm-investigator/internal/incident"
	"apm-investigator/internal/investigation"
	"apm-investigator/internal/recovery"
	"apm-investigator/internal/store"
	"apm-investigator/internal/tools"
	"github.com/google/uuid"
)

type Runner struct {
	Store         *store.Store
	Config        config.AI
	Root          string
	Logger        *slog.Logger
	WithoutMemory bool
	AdvanceNext   bool
	Retry         bool
}
type Outcome struct {
	Incident incident.Incident        `json:"incident"`
	Proposal *incident.Proposal       `json:"proposal,omitempty"`
	Recovery *incident.RecoveryResult `json:"recovery,omitempty"`
	Message  string                   `json:"message"`
}

func (r *Runner) Run(ctx context.Context, name string, id uuid.UUID) (out Outcome, err error) {
	if (r.AdvanceNext || r.Retry) && id == uuid.Nil || (r.AdvanceNext && r.Retry) {
		return out, fmt.Errorf("--next and --retry require --incident and cannot be combined")
	}
	s, err := Load(r.Root, name)
	if err != nil {
		return out, err
	}
	if err := r.Config.Mode.Validate(); err != nil {
		return out, err
	}
	var i incident.Incident
	if id == uuid.Nil {
		impact, cohort := s.Sources.Base(s.Scope, s.StartedAt, s.CriticalMerchant)
		now := time.Now().UTC()
		i = incident.Incident{
			ID:        uuid.New(),
			Status:    incident.Investigating,
			Revision:  1,
			Scope:     s.Scope,
			Scenario:  s.Name,
			Clock:     s.StartedAt,
			Impact:    impact,
			Cohort:    cohort,
			CreatedAt: now,
			UpdatedAt: now,
		}
	} else {
		i, err = r.Store.Get(ctx, id)
		if err != nil {
			return out, err
		}
		if i.Scenario != name {
			return out, fmt.Errorf("scenario does not match incident")
		}
	}
	if i.Status == incident.Closed {
		out.Incident = i
		if r.AdvanceNext || r.Retry {
			return out, fmt.Errorf("incident is closed")
		}
		return out, nil
	}
	if r.Retry && i.Status != incident.ManualTriage {
		return out, fmt.Errorf("--retry requires manual_triage")
	}
	if (i.Status == incident.AwaitingReview || i.Status == incident.ClosureProposed || i.Status == incident.ManualTriage) && !r.AdvanceNext && !r.Retry {
		out.Incident = i
		out.Message = "Review the proposal, or use --next for new evidence / --retry after manual triage."
		return out, nil
	}
	clock := i.Clock
	if r.AdvanceNext && i.Cursor >= len(s.Events) {
		return out, fmt.Errorf("no further scenario events")
	}
	if (r.AdvanceNext || i.Status == incident.MonitoringRecovery) && i.Cursor < len(s.Events) {
		clock = s.Events[i.Cursor].AvailableAt
	}

	var agent investigation.Agent
	if i.Status == incident.Investigating || r.Retry {
		agent, err = r.agent(s)
		if err != nil {
			return out, err
		}
	}
	if r.Retry {
		i, err = r.Store.Retry(ctx, i.ID, i.Revision)
		if err != nil {
			return out, err
		}
	}
	if id == uuid.Nil {
		if err = r.Store.Create(ctx, i); err != nil {
			return out, err
		}
	}
	for i.Cursor < len(s.Events) && !s.Events[i.Cursor].AvailableAt.After(clock) {
		e := s.Events[i.Cursor]
		e.ID = uuid.New()
		e.IncidentID = i.ID
		e.ExternalID = i.ID.String() + ":" + e.ExternalID
		_, err = r.Store.Ingest(ctx, e, clock, i.Cursor+1)
		if err != nil {
			return out, err
		}
		i, err = r.Store.Get(ctx, i.ID)
		if err != nil {
			return out, err
		}
	}
	if !i.Impact.CohortComplete {
		payments := s.Sources.PaymentSummary(i.Scope, i.Clock)
		i, err = r.Store.RefineCohort(ctx, i, payments.Total, payments.CompletedIDs, payments.DataComplete)
		if err != nil {
			return out, err
		}
	}
	if i.Status != incident.Investigating && i.Status != incident.MonitoringRecovery {
		out.Incident = i
		out.Message = "New evidence recorded; manual follow-up required"
		return out, nil
	}
	if i.Status == incident.Investigating {

		if agent == nil {
			agent, err = r.agent(s)
			if err != nil {
				out.Incident = i
				return out, err
			}
		}
		service := investigation.Service{
			Store:         r.Store,
			Agent:         agent,
			Registry:      &tools.Registry{Sources: s.Sources, Now: i.Clock, Cohort: i.Cohort},
			MaxSteps:      r.Config.MaxSteps,
			Mode:          string(r.Config.Mode),
			Logger:        r.Logger,
			WithoutMemory: r.WithoutMemory,
		}
		out.Proposal, err = service.Investigate(ctx, i.ID)
		out.Message = "Investigation requires manual follow-up"
		if out.Proposal != nil {
			out.Message = "Escalation proposal: approval required"
		}
		if err != nil {
			out.Message = "Investigation stopped; collected evidence is preserved: " + err.Error()
		}
	} else {
		service := recovery.Service{Store: r.Store, Sources: s.Sources}
		var result incident.RecoveryResult
		result, out.Proposal, err = service.Check(ctx, i)
		out.Recovery = &result
		out.Message = "Closure blocked: original cohort or new traffic not verified"
		if result.ClosureAllowed {
			out.Message = "Closure proposal: approval required"
		}
	}
	current, iErr := r.Store.Get(ctx, i.ID)
	if iErr != nil {
		return out, iErr
	}
	out.Incident = current
	return out, err
}

func (r *Runner) agent(s *Scenario) (investigation.Agent, error) {
	if r.Config.Mode == config.Live {
		return ai.NewOpenAI(r.Config.APIKey, r.Config.Model, r.Config.ReasoningEffort, r.Config.Timeout)
	}
	return ai.LoadMock(filepath.Join(s.Directory, "mock_turns.json"))
}
