package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	db "apm-investigator/internal/db/generated"
	"apm-investigator/internal/incident"
	"apm-investigator/internal/policy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) StartRun(ctx context.Context, i incident.Incident, mode string, useMemory bool) (incident.Run, error) {
	r := incident.Run{
		ID:           uuid.New(),
		IncidentID:   i.ID,
		BaseRevision: i.Revision,
		Mode:         mode,
		Status:       incident.RunRunning,
		Turns:        []incident.ModelTurn{},
		Memory:       []incident.MemoryRule{},
	}
	err := s.tx(ctx, func(q *db.Queries) error {
		current, err := decodeIncident(q.LockIncident(ctx, i.ID))
		if err != nil {
			return err
		}
		if current.Revision != i.Revision {
			return incident.ErrStaleRevision
		}
		if current.Status != incident.Investigating {
			return incident.ErrInvalidInput
		}
		if useMemory {
			r.Memory, err = activeMemory(ctx, q, current, mode)
			if err != nil {
				return err
			}
		}
		r.Context, err = previousContext(ctx, q, current, mode)
		if err != nil {
			return err
		}
		contextData, err := json.Marshal(r.Context)
		if err != nil {
			return err
		}
		memory, err := json.Marshal(r.Memory)
		if err != nil {
			return err
		}
		return q.StartRun(ctx, db.StartRunParams{
			ID:           r.ID,
			IncidentID:   i.ID,
			BaseRevision: i.Revision,
			Mode:         mode,
			Memory:       memory,
			Context:      contextData,
		})
	})
	return r, err
}
func saveRun(ctx context.Context, q *db.Queries, r incident.Run) error {
	var result []byte
	var err error
	if r.Result != nil {
		result, err = json.Marshal(r.Result)
		if err != nil {
			return err
		}
	}
	turns, err := json.Marshal(r.Turns)
	if err != nil {
		return err
	}
	return q.UpdateRun(ctx, db.UpdateRunParams{
		ID:           r.ID,
		Status:       string(r.Status),
		Result:       result,
		ErrorMessage: r.Error,
		Turns:        turns,
	})
}
func (s *Store) RecordTurn(ctx context.Context, i incident.Incident, r incident.Run, turn incident.ModelTurn) error {
	return s.tx(ctx, func(q *db.Queries) error {
		if err := saveRun(ctx, q, r); err != nil {
			return err
		}
		return appendTimeline(ctx, q, i, "model_turn", turn)
	})
}
func insertEvidence(ctx context.Context, q *db.Queries, e incident.Evidence) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return q.InsertEvidence(ctx, db.InsertEvidenceParams{
		ID:          uuid.New(),
		IncidentID:  e.IncidentID,
		RunID:       e.RunID,
		EvidenceRef: e.ID,
		Source:      e.Source,
		Kind:        e.Kind,
		ObservedAt:  e.ObservedAt,
		Available:   e.Available,
		Summary:     e.Summary,
		Data:        data,
	})
}
func (s *Store) Evidence(ctx context.Context, id uuid.UUID) ([]incident.Evidence, error) {
	rows, err := s.q.ListEvidence(ctx, id)
	if err != nil {
		return nil, err
	}
	out := []incident.Evidence{}
	for _, r := range rows {
		var e incident.Evidence
		if err = json.Unmarshal(r, &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}
func (s *Store) RecordEvidence(ctx context.Context, i incident.Incident, e incident.Evidence) error {
	return s.tx(ctx, func(q *db.Queries) error {
		if err := insertEvidence(ctx, q, e); err != nil {
			return err
		}
		return appendTimeline(ctx, q, i, "evidence", e)
	})
}
func (s *Store) RecordTool(ctx context.Context, i incident.Incident, t incident.ToolRun, e *incident.Evidence) error {
	return s.tx(ctx, func(q *db.Queries) error {
		ref := pgtype.Text{}
		if e != nil {
			if err := insertEvidence(ctx, q, *e); err != nil {
				return err
			}
			ref = pgtype.Text{String: e.ID, Valid: true}
			if err := appendTimeline(ctx, q, i, "evidence", e); err != nil {
				return err
			}
		}
		err := q.InsertToolRun(ctx, db.InsertToolRunParams{
			ID:                 t.ID,
			InvestigationRunID: t.RunID,
			Step:               int32(t.Step),
			ToolName:           t.Name,
			Arguments:          t.Arguments,
			Status:             string(t.Status),
			EvidenceRef:        ref,
			ErrorCode:          t.Error,
			StartedAt:          t.StartedAt,
			FinishedAt:         t.FinishedAt,
		})
		if err != nil {
			return err
		}
		return appendTimeline(ctx, q, i, "tool_run", t)
	})
}
func (s *Store) Finish(ctx context.Context, i incident.Incident, r incident.Run) (*incident.Proposal, error) {
	var p *incident.Proposal
	err := s.tx(ctx, func(q *db.Queries) error {
		evidence, err := runEvidence(ctx, q, r.ID)
		if err != nil {
			return err
		}
		evidence = append(append([]incident.Evidence{}, r.Context.Evidence...), evidence...)
		outcome := policy.AfterInvestigation(i, r, evidence)
		to := outcome.Status
		if err := incident.Transition(i.Status, to); err != nil {
			return err
		}
		i.Status = to
		if err := update(ctx, q, &i, r.BaseRevision); err != nil {
			return err
		}
		if err := saveRun(ctx, q, r); err != nil {
			return err
		}
		if outcome.Proposal != nil {
			v := newProposal(i, *outcome.Proposal)
			p = &v
			if err := insertProposal(ctx, q, i, v); err != nil {
				return err
			}
		}
		return appendTimeline(ctx, q, i, "investigation_"+string(r.Status), r)
	})
	if errors.Is(err, incident.ErrStaleRevision) {
		r.Status = incident.RunSuperseded
		r.Error = err.Error()
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if saveErr := saveRun(cleanup, s.q, r); saveErr != nil {
			return nil, errors.Join(err, saveErr)
		}
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}
func (s *Store) Runs(ctx context.Context, id uuid.UUID) ([]incident.Run, error) {
	rows, err := s.q.ListRuns(ctx, id)
	if err != nil {
		return nil, err
	}
	out := []incident.Run{}
	for _, v := range rows {
		r, err := decodeRun(v)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}
func (s *Store) ToolRuns(ctx context.Context, id uuid.UUID) ([]incident.ToolRun, error) {
	return toolRuns(ctx, s.q, id)
}

func toolRuns(ctx context.Context, q *db.Queries, id uuid.UUID) ([]incident.ToolRun, error) {
	rows, err := q.ListToolRuns(ctx, id)
	if err != nil {
		return nil, err
	}
	out := []incident.ToolRun{}
	for _, r := range rows {
		out = append(out, incident.ToolRun{
			ID:          r.ID,
			RunID:       r.InvestigationRunID,
			Step:        int(r.Step),
			Name:        r.ToolName,
			Arguments:   r.Arguments,
			Status:      incident.ToolStatus(r.Status),
			EvidenceRef: r.EvidenceRef.String,
			Error:       r.ErrorCode,
			StartedAt:   r.StartedAt,
			FinishedAt:  r.FinishedAt,
		})
	}
	return out, nil
}

func (s *Store) Run(ctx context.Context, id uuid.UUID) (incident.Run, error) {
	row, err := s.q.GetRun(ctx, id)
	if err != nil {
		return incident.Run{}, err
	}
	return decodeRun(row)
}

func decodeRun(v db.InvestigationRun) (incident.Run, error) {
	r := incident.Run{ID: v.ID, IncidentID: v.IncidentID, BaseRevision: v.BaseRevision, Mode: v.Mode, Status: incident.RunStatus(v.Status), Error: v.ErrorMessage}
	if v.Result != nil {
		if err := json.Unmarshal(v.Result, &r.Result); err != nil {
			return r, err
		}
	}
	if err := json.Unmarshal(v.Turns, &r.Turns); err != nil {
		return r, err
	}
	if err := json.Unmarshal(v.Memory, &r.Memory); err != nil {
		return r, err
	}
	if err := json.Unmarshal(v.Context, &r.Context); err != nil {
		return r, err
	}
	return r, nil
}

func (s *Store) RunEvidence(ctx context.Context, id uuid.UUID) ([]incident.Evidence, error) {
	return runEvidence(ctx, s.q, id)
}

func runEvidence(ctx context.Context, q *db.Queries, id uuid.UUID) ([]incident.Evidence, error) {
	rows, err := q.RunEvidence(ctx, id)
	if err != nil {
		return nil, err
	}
	out := []incident.Evidence{}
	for _, row := range rows {
		var e incident.Evidence
		if err := json.Unmarshal(row, &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}
