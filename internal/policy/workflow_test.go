package policy

import (
	"github.com/google/uuid"
	"strings"
	"testing"
	"time"

	"apm-investigator/internal/incident"
)

func TestOnlyExplicitEscalationCreatesProposal(t *testing.T) {
	for _, action := range incident.Actions() {
		out := AfterInvestigation(incident.Incident{}, incident.Run{Status: incident.RunCompleted, Result: &incident.Result{RecommendedAction: action, RecommendedOwner: incident.OwnerTechOps}}, nil)
		if action == incident.ActionEscalate {
			if out.Status != incident.AwaitingReview || out.Proposal == nil {
				t.Fatal(out)
			}
		} else if out.Status != incident.ManualTriage || out.Proposal != nil {
			t.Fatalf("%s unexpectedly escalated: %+v", action, out)
		}
	}
}
func TestIncompleteMembershipCannotRecover(t *testing.T) {
	r := Recovery(RecoveryInput{Original: []string{"known"}, Acknowledged: map[string]bool{"known": true}, CallbacksComplete: true, NewChecked: 10, NewSuccessful: 10})
	if r.ClosureAllowed || r.OriginalCohortRecovered != incident.Unknown || r.NewTrafficHealthy != incident.Pass {
		t.Fatal(r)
	}
}
func TestContradictoryRecoveryDoesNotCreateProposal(t *testing.T) {
	if _, err := AfterRecovery(incident.RecoveryResult{ClosureAllowed: true, OriginalCohortRecovered: incident.Unknown, NewTrafficHealthy: incident.Pass}); err == nil {
		t.Fatal("contradictory recovery accepted")
	}
}

func TestEscalationPacketContainsReviewContext(t *testing.T) {
	i := incident.Incident{ID: uuid.New(), Clock: time.Date(2026, 9, 22, 10, 5, 0, 0, time.UTC), Scope: incident.Scope{MerchantID: "merchant", Method: "apm", Environment: "sandbox"}, Impact: incident.Impact{UniquePayments: 5, AffectedPayments: 3, CohortComplete: true}}
	run := incident.Run{ID: uuid.New(), BaseRevision: 4, Status: incident.RunCompleted, Result: &incident.Result{Summary: "Queue delayed", FailureDomain: "dispatch", RootCauseStatus: incident.CauseLocalized, RecommendedAction: incident.ActionEscalate, RecommendedOwner: incident.OwnerCallbacks, EvidenceIDs: []string{"E1"}, Hypotheses: []incident.Hypothesis{{ID: "H1", Status: incident.HypothesisSupported, Statement: "Dispatch delay", SupportingEvidence: []string{"E1"}}}, OpenQuestions: []incident.OpenQuestion{{Question: "Why did the worker stop?", WhyItMatters: "Confirm underlying cause"}}}}
	e := incident.Evidence{ID: "E1", RunID: run.ID, Source: "queue", Kind: incident.ToolQueue, ObservedAt: i.Clock, Claims: []string{incident.ClaimQueueDegraded}, Summary: "845 jobs waiting", Available: true}
	out := AfterInvestigation(i, run, []incident.Evidence{e})
	for _, want := range []string{i.ID.String(), run.ID.String(), "revision: 4", "merchant / apm / sandbox", "3 potentially affected / 5 unique", "membership complete=true", "root cause: localized", "845 jobs waiting", "support: E1", "Next check: Why did the worker stop?"} {
		if !strings.Contains(out.Proposal.Body, want) {
			t.Fatal("missing handoff context", want, out.Proposal.Body)
		}
	}
}
