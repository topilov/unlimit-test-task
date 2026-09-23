package store

import (
	"context"
	"encoding/json"

	db "apm-investigator/internal/db/generated"
	"apm-investigator/internal/incident"
	"apm-investigator/internal/policy"
)

func (s *Store) RecordRecovery(ctx context.Context, i incident.Incident, r incident.RecoveryResult, facts json.RawMessage) (*incident.Proposal, error) {
	var p *incident.Proposal
	err := s.tx(ctx, func(q *db.Queries) error {
		current, err := decodeIncident(q.LockIncident(ctx, i.ID))
		if err != nil {
			return err
		}
		if current.Revision != i.Revision {
			return incident.ErrStaleRevision
		}
		if current.Status != incident.MonitoringRecovery {
			return incident.ErrInvalidInput
		}
		outcome, err := policy.AfterRecovery(r)
		if err != nil {
			return err
		}
		if err = changeStatus(&i, outcome.Status); err != nil {
			return err
		}
		if err = update(ctx, q, &i, i.Revision); err != nil {
			return err
		}
		if err = appendTimeline(ctx, q, i, "recovery_evidence", facts); err != nil {
			return err
		}
		if err = appendTimeline(ctx, q, i, incident.RecoveryCheckPurpose, r); err != nil {
			return err
		}
		if outcome.Proposal != nil {
			v := newProposal(i, *outcome.Proposal)
			p = &v
			return insertProposal(ctx, q, i, v)
		}
		return nil
	})
	return p, err
}
