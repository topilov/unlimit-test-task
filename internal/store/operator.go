package store

import (
	"context"
	"slices"

	db "apm-investigator/internal/db/generated"
	"apm-investigator/internal/incident"
	"github.com/google/uuid"
)

func (s *Store) Retry(ctx context.Context, id uuid.UUID, revision int64) (incident.Incident, error) {
	var i incident.Incident
	err := s.tx(ctx, func(q *db.Queries) error {
		var err error
		i, err = decodeIncident(q.LockIncident(ctx, id))
		if err != nil {
			return err
		}
		if i.Revision != revision {
			return incident.ErrStaleRevision
		}
		if i.Status != incident.ManualTriage {
			return incident.ErrInvalidInput
		}
		i.Status = incident.Investigating
		if err := update(ctx, q, &i, revision); err != nil {
			return err
		}
		return appendTimeline(ctx, q, i, "investigation_retry", map[string]int64{"revision": i.Revision})
	})
	return i, err
}

func (s *Store) RefineCohort(ctx context.Context, i incident.Incident, unique int, completed []string, complete bool) (incident.Incident, error) {
	if i.Impact.CohortComplete {
		return i, nil
	}
	if unique < len(completed) {
		return i, incident.ErrInvalidInput
	}
	before := i.Impact
	cohort := append([]string{}, i.Cohort...)
	for _, id := range i.Cohort {
		complete = complete && slices.Contains(completed, id)
	}
	for _, id := range completed {
		if !slices.Contains(cohort, id) {
			cohort = append(cohort, id)
		}
	}
	slices.Sort(cohort)
	i.Impact.CohortComplete = complete && unique >= before.UniquePayments
	i.Impact.UniquePayments = max(unique, before.UniquePayments, len(cohort))
	i.Impact.AffectedPayments = len(cohort)
	i.Impact.CustomerVisible = len(cohort) > 0
	if before == i.Impact && slices.Equal(i.Cohort, cohort) {
		return i, nil
	}
	i.Cohort = cohort
	err := s.tx(ctx, func(q *db.Queries) error {
		if err := update(ctx, q, &i, i.Revision); err != nil {
			return err
		}
		return appendTimeline(ctx, q, i, "cohort_refined", map[string]any{"impact": i.Impact, "original_cohort": i.Cohort})
	})
	return i, err
}
