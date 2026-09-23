package store

import (
	"context"
	"encoding/json"
	"time"

	db "apm-investigator/internal/db/generated"
	"apm-investigator/internal/incident"
	"apm-investigator/internal/policy"
	"github.com/google/uuid"
)

func (s *Store) Ingest(ctx context.Context, e incident.Event, clock time.Time, cursor int) (bool, error) {
	if cursor < 0 || e.AvailableAt.After(clock) || e.OccurredAt.After(e.AvailableAt) || !json.Valid(e.Payload) {
		return false, incident.ErrInvalidInput
	}
	kinds := map[string]string{
		incident.EventFiring:   "grafana",
		incident.EventResolved: "grafana",
		incident.EventSlack:    "slack",
		incident.EventSupport:  "support",
		incident.EventNote:     "manual",
	}
	if e.ExternalID == "" || kinds[e.Kind] != e.Source || e.Source == "" {
		return false, incident.ErrInvalidInput
	}
	inserted := false
	err := s.tx(ctx, func(q *db.Queries) error {
		i, err := decodeIncident(q.LockIncident(ctx, e.IncidentID))
		if err != nil {
			return err
		}
		n, err := q.InsertEvent(ctx, db.InsertEventParams{
			ID:          e.ID,
			IncidentID:  e.IncidentID,
			Source:      e.Source,
			ExternalID:  e.ExternalID,
			Kind:        e.Kind,
			OccurredAt:  e.OccurredAt,
			AvailableAt: e.AvailableAt,
			Payload:     e.Payload,
		})
		if err != nil {
			return err
		}
		if n == 0 {

			if cursor > i.Cursor {
				return q.AdvanceReplayCursor(ctx, db.AdvanceReplayCursorParams{ID: i.ID, Cursor: int32(cursor)})
			}
			return nil
		}
		if clock.Before(i.Clock) || cursor < i.Cursor || i.Status == incident.Closed {
			return incident.ErrInvalidInput
		}
		rev := i.Revision
		i.Clock = clock
		i.Cursor = cursor
		to := policy.AfterEvent(i.Status, e)
		if err = changeStatus(&i, to); err != nil {
			return err
		}
		if err = update(ctx, q, &i, rev); err != nil {
			return err
		}
		inserted = true
		return appendTimeline(ctx, q, i, "event_received", e)
	})
	return inserted, err
}
func (s *Store) Events(ctx context.Context, id uuid.UUID) ([]incident.Event, error) {
	rows, err := s.q.ListEvents(ctx, id)
	if err != nil {
		return nil, err
	}
	out := []incident.Event{}
	for _, r := range rows {
		out = append(out, incident.Event{
			ID:          r.ID,
			IncidentID:  r.IncidentID,
			Source:      r.Source,
			ExternalID:  r.ExternalID,
			Kind:        r.Kind,
			OccurredAt:  r.OccurredAt,
			AvailableAt: r.AvailableAt,
			Payload:     r.Payload,
		})
	}
	return out, nil
}
