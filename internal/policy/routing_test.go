package policy

import (
	"apm-investigator/internal/incident"
	"testing"
	"time"
)

func TestTargetedEscalationNeedsCurrentLocalization(t *testing.T) {
	now := time.Date(2026, 9, 22, 10, 5, 0, 0, time.UTC)
	i := incident.Incident{Clock: now}
	r := incident.Result{RecommendedAction: incident.ActionEscalate, RecommendedOwner: incident.OwnerCallbacks, RootCauseStatus: incident.CauseUnknown, EvidenceIDs: []string{"E1"}, Hypotheses: []incident.Hypothesis{{Status: incident.HypothesisSupported, SupportingEvidence: []string{"E1"}}}}
	evidence := []incident.Evidence{{ID: "E1", Kind: incident.ToolQueue, ObservedAt: now.Add(-time.Minute), Available: true, Claims: []string{incident.ClaimQueueDegraded}}}
	run := incident.Run{Status: incident.RunCompleted, Result: &r}
	if got := AfterInvestigation(i, run, evidence); got.Proposal == nil || got.Status != incident.AwaitingReview {
		t.Fatal("localized failure with unknown cause must reach human review", got)
	}
	for _, tc := range []struct {
		name  string
		alter func(*incident.Result, *[]incident.Evidence)
	}{
		{"no evidence", func(_ *incident.Result, e *[]incident.Evidence) { *e = nil }},
		{"healthy latest", func(_ *incident.Result, e *[]incident.Evidence) {
			*e = append(*e, incident.Evidence{ID: "E2", Kind: incident.ToolQueue, ObservedAt: now, Available: true, Claims: []string{incident.ClaimQueueHealthy}})
		}},
		{"unavailable latest", func(_ *incident.Result, e *[]incident.Evidence) {
			*e = append(*e, incident.Evidence{ID: "E2", Kind: incident.ToolQueue, ObservedAt: now})
		}},
		{"stale", func(_ *incident.Result, e *[]incident.Evidence) { (*e)[0].ObservedAt = now.Add(-6 * time.Minute) }},
		{"unsupported hypothesis", func(r *incident.Result, _ *[]incident.Evidence) { r.Hypotheses = nil }},
		{"wrong owner", func(r *incident.Result, _ *[]incident.Evidence) { r.RecommendedOwner = incident.OwnerMerchant }},
		{"uncited finding", func(r *incident.Result, _ *[]incident.Evidence) { r.EvidenceIDs = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copyR := r
			copyE := append([]incident.Evidence{}, evidence...)
			tc.alter(&copyR, &copyE)
			got := AfterInvestigation(i, incident.Run{Status: incident.RunCompleted, Result: &copyR}, copyE)
			if got.Proposal != nil || got.Status != incident.ManualTriage {
				t.Fatal("unsupported routing created proposal", got)
			}
		})
	}
}
