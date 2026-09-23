package recovery

import (
	"context"
	"encoding/json"
	"time"

	"apm-investigator/internal/incident"
	"apm-investigator/internal/policy"
	"apm-investigator/internal/store"
	"apm-investigator/internal/tools"
)

type Service struct {
	Store   *store.Store
	Sources *tools.Sources
}
type Facts struct {
	Scope          incident.Scope        `json:"scope"`
	Cohort         []string              `json:"original_cohort"`
	CohortComplete bool                  `json:"original_cohort_complete"`
	ObservedAt     time.Time             `json:"observed_at"`
	AvailableAt    time.Time             `json:"available_at"`
	Callbacks      tools.CallbackSummary `json:"callbacks"`
}

func (s *Service) Check(ctx context.Context, i incident.Incident) (incident.RecoveryResult, *incident.Proposal, error) {
	c := s.Sources.CallbackSummary(i.Scope, i.Clock, i.Cohort)
	acked := map[string]bool{}
	for _, id := range c.Acknowledged {
		acked[id] = true
	}
	result := policy.Recovery(policy.RecoveryInput{
		Original:          i.Cohort,
		CohortComplete:    i.Impact.CohortComplete,
		Acknowledged:      acked,
		CallbacksComplete: c.DataComplete,
		NewChecked:        c.NewTrafficChecked,
		NewSuccessful:     c.NewTrafficSuccessful,
	})
	snapshot, _ := tools.Latest(s.Sources.Callbacks, i.Scope, i.Clock)
	facts, err := json.Marshal(Facts{
		Scope:          i.Scope,
		Cohort:         i.Cohort,
		CohortComplete: i.Impact.CohortComplete,
		ObservedAt:     snapshot.ObservedAt,
		AvailableAt:    snapshot.AvailableAt,
		Callbacks:      c,
	})
	if err != nil {
		return result, nil, err
	}
	proposal, err := s.Store.RecordRecovery(ctx, i, result, facts)
	return result, proposal, err
}
