package store

import (
	"context"
	"encoding/json"
	"time"

	db "apm-investigator/internal/db/generated"
	"apm-investigator/internal/incident"
	"github.com/google/uuid"
)

type incidentDetails struct {
	Scenario  string          `json:"scenario"`
	Clock     time.Time       `json:"clock"`
	Cursor    int             `json:"cursor"`
	WindowEnd time.Time       `json:"window_end"`
	Impact    incident.Impact `json:"impact"`
	Cohort    []string        `json:"original_cohort"`
}

func encodeIncident(i incident.Incident) ([]byte, error) {
	return json.Marshal(incidentDetails{
		Scenario:  i.Scenario,
		Clock:     i.Clock,
		Cursor:    i.Cursor,
		WindowEnd: i.Scope.WindowEnd,
		Impact:    i.Impact,
		Cohort:    i.Cohort,
	})
}
func decodeIncident(row db.Incident, err error) (incident.Incident, error) {
	var i incident.Incident
	if err != nil {
		return i, err
	}
	var details incidentDetails
	if err = json.Unmarshal(row.Details, &details); err != nil {
		return i, err
	}
	i = incident.Incident{ID: row.ID, Status: incident.Status(row.Status), Revision: row.Revision,
		Scope: incident.Scope{
			MerchantID:  row.MerchantID,
			Method:      row.PaymentMethod,
			Environment: row.Environment,
			WindowStart: row.StartedAt.UTC(),
			WindowEnd:   details.WindowEnd,
		},
		Scenario: details.Scenario, Clock: details.Clock, Cursor: details.Cursor, Impact: details.Impact, Cohort: details.Cohort, CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC()}
	return i, nil
}
func (s *Store) Get(ctx context.Context, id uuid.UUID) (incident.Incident, error) {
	return decodeIncident(s.q.GetIncident(ctx, id))
}
func (s *Store) List(ctx context.Context) ([]incident.Incident, error) {
	rows, err := s.q.ListIncidents(ctx)
	if err != nil {
		return nil, err
	}
	out := []incident.Incident{}
	for _, row := range rows {
		i, e := decodeIncident(row, nil)
		if e != nil {
			return nil, e
		}
		out = append(out, i)
	}
	return out, nil
}
func appendTimeline(ctx context.Context, q *db.Queries, i incident.Incident, kind string, data any) error {
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return q.AppendTimeline(ctx, db.AppendTimelineParams{
		IncidentID: i.ID,
		At:         i.Clock,
		Kind:       kind,
		Data:       encoded,
	})
}
func update(ctx context.Context, q *db.Queries, i *incident.Incident, revision int64) error {
	i.Revision = revision + 1
	i.UpdatedAt = time.Now().UTC()
	data, err := encodeIncident(*i)
	if err != nil {
		return err
	}
	n, err := q.UpdateIncident(ctx, db.UpdateIncidentParams{
		ID:        i.ID,
		Status:    string(i.Status),
		Details:   data,
		UpdatedAt: i.UpdatedAt,
		Revision:  revision,
	})
	if err != nil {
		return err
	}
	if n != 1 {
		return incident.ErrStaleRevision
	}
	return nil
}
func (s *Store) Create(ctx context.Context, i incident.Incident) error {
	return s.tx(ctx, func(q *db.Queries) error {
		data, err := encodeIncident(i)
		if err != nil {
			return err
		}
		err = q.CreateIncident(ctx, db.CreateIncidentParams{
			ID:            i.ID,
			Status:        string(i.Status),
			Revision:      i.Revision,
			MerchantID:    i.Scope.MerchantID,
			PaymentMethod: i.Scope.Method,
			Environment:   i.Scope.Environment,
			StartedAt:     i.Scope.WindowStart,
			CreatedAt:     i.CreatedAt,
			UpdatedAt:     i.UpdatedAt,
			Details:       data,
		})
		if err != nil {
			return err
		}
		return appendTimeline(ctx, q, i, "incident_created", i)
	})
}

func (s *Store) Timeline(ctx context.Context, id uuid.UUID) ([]incident.TimelineEntry, error) {
	rows, err := s.q.Timeline(ctx, id)
	if err != nil {
		return nil, err
	}
	out := []incident.TimelineEntry{}
	for _, r := range rows {
		out = append(out, incident.TimelineEntry{At: r.At, Kind: r.Kind, Data: r.Data})
	}
	return out, nil
}

func changeStatus(i *incident.Incident, to incident.Status) error {
	if i.Status == to {
		return nil
	}
	if err := incident.Transition(i.Status, to); err != nil {
		return err
	}
	i.Status = to
	return nil
}
