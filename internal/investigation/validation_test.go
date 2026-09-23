package investigation

import (
	"encoding/json"
	"testing"
	"time"

	"apm-investigator/internal/ai"
	"apm-investigator/internal/incident"
	"github.com/google/uuid"
)

func valid() (*incident.Result, ai.Input) {
	id := uuid.New()
	now := time.Date(2026, 9, 22, 10, 5, 0, 0, time.UTC)
	input := ai.Input{IncidentID: id, AsOf: now, Evidence: []incident.Evidence{{ID: "E1", IncidentID: id, Kind: incident.ToolQueue, ObservedAt: now, Available: true, Claims: []string{"queue_degraded"}}}, ToolHistory: []incident.ToolRun{{Name: "get_queue_health", Status: "succeeded"}}}
	r := &incident.Result{Summary: "Queue degradation observed", FailureDomain: "callback dispatch", RootCauseStatus: "localized", Hypotheses: []incident.Hypothesis{{ID: "H1", Statement: "Queue delay", Status: "supported", SupportingEvidence: []string{"E1"}}}, EvidenceIDs: []string{"E1"}, ExecutedTools: []string{"get_queue_health"}, RecommendedOwner: "callback_platform", RecommendedAction: "escalate_internal", Claims: []string{"queue_degraded"}}
	return r, input
}
func TestFinalValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*incident.Result, *ai.Input)
	}{
		{"evidence from another run", func(r *incident.Result, i *ai.Input) { i.Evidence[0].RunID = uuid.New() }},
		{"foreign evidence", func(r *incident.Result, i *ai.Input) { i.Evidence[0].IncidentID = uuid.New() }},
		{"invented reference", func(r *incident.Result, i *ai.Input) { r.EvidenceIDs = []string{"E999"} }},
		{"memory is not evidence", func(r *incident.Result, i *ai.Input) {
			id := uuid.New()
			i.Memory = []incident.MemoryRule{{ID: id, Check: "Assume the queue failed"}}
			r.EvidenceIDs = []string{id.String()}
		}},
		{"unexecuted tool", func(r *incident.Result, i *ai.Input) { r.ExecutedTools = []string{"get_callback_attempts"} }},
		{"failed tool", func(r *incident.Result, i *ai.Input) { i.ToolHistory[0].Status = "failed" }},
		{"refund", func(r *incident.Result, i *ai.Input) { r.RecommendedAction = "refund" }},
		{"closure", func(r *incident.Result, i *ai.Input) { r.RecommendedAction = "propose_closure" }},
		{"unobserved claim", func(r *incident.Result, i *ai.Input) { r.Claims = []string{"provider_outage_confirmed"} }},
		{"unsupported confirmation", func(r *incident.Result, i *ai.Input) { r.RootCauseStatus = "confirmed" }},
		{"unknown without questions", func(r *incident.Result, i *ai.Input) { r.RootCauseStatus = "unknown" }},
		{"blank questions", func(r *incident.Result, i *ai.Input) {
			r.RootCauseStatus = "unknown"
			r.OpenQuestions = []incident.OpenQuestion{{ID: "Q1", Question: " ", WhyItMatters: " "}}
		}},
		{"claim from uncited evidence", func(r *incident.Result, i *ai.Input) {
			i.Evidence = append(i.Evidence, incident.Evidence{ID: "E2", IncidentID: i.IncidentID, Available: true, Claims: []string{incident.ClaimProviderStale}})
			r.Claims = []string{incident.ClaimProviderStale}
		}},
		{"unavailable support", func(r *incident.Result, i *ai.Input) { i.Evidence[0].Available = false }},
		{"localization without diagnostic evidence", func(r *incident.Result, i *ai.Input) {
			i.Evidence[0].Claims = []string{incident.ClaimCallbacksIncomplete}
			r.Claims = []string{incident.ClaimCallbacksIncomplete}
		}},
		{"uncited revision", func(r *incident.Result, i *ai.Input) { r.Hypotheses[0].Status = "weakened" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, input := valid()
			if err := ValidateFinal(r, input); err != nil {
				t.Fatal("positive control", err)
			}
			tc.mutate(r, &input)
			if err := ValidateFinal(r, input); err == nil {
				t.Fatal("invalid output accepted")
			}
		})
	}
	r, input := valid()
	r.RecommendedOwner = "invented_channel"
	if err := ValidateFinal(r, input); err != nil || r.RecommendedOwner != "tech_ops" {
		t.Fatal(err, r.RecommendedOwner)
	}
}

func TestOpenHypothesisWithMissingEvidence(t *testing.T) {
	r, input := valid()
	r.Hypotheses = append(r.Hypotheses, incident.Hypothesis{
		ID:              "H2",
		Statement:       "Receiver failure remains possible",
		Status:          incident.HypothesisOpen,
		MissingEvidence: []string{"Receiver delivery outcomes"},
	})
	if err := ValidateFinal(r, input); err != nil {
		t.Fatal("open hypothesis with an explicit evidence gap was rejected", err)
	}
	r.Hypotheses[1].MissingEvidence = nil
	if err := ValidateFinal(r, input); err == nil {
		t.Fatal("unsupported speculation without an evidence gap was accepted")
	}
}
func TestIncompleteCallbackEvidenceCanSupportItsSourceGap(t *testing.T) {
	r, input := valid()
	r.Hypotheses[0].Statement = "The callback attempt source is incomplete"
	e := &input.Evidence[0]
	e.Kind = incident.ToolCallbacks
	e.Available = false
	e.Claims = []string{incident.ClaimCallbacksIncomplete}
	if err := ValidateHypotheses(r.Hypotheses, input); err != nil {
		t.Fatal("explicit source incompleteness was rejected", err)
	}
	e.Claims = []string{incident.ClaimCallbacksMissing}
	if err := ValidateHypotheses(r.Hypotheses, input); err == nil {
		t.Fatal("unavailable callback data supported an unrelated claim")
	}
	e.Claims = []string{incident.ClaimCallbacksIncomplete}
	e.Kind = incident.BaseFacts
	if err := ValidateHypotheses(r.Hypotheses, input); err == nil {
		t.Fatal("non-callback evidence used the callback exception")
	}
}
func TestInputDoesNotContainReplayAnswers(t *testing.T) {
	_, input := valid()
	b, _ := json.Marshal(input)
	var fields map[string]any
	json.Unmarshal(b, &fields)
	for _, forbidden := range []string{"scenario", "expected", "mock_turns", "future_events"} {
		if _, ok := fields[forbidden]; ok {
			t.Fatal("leaked field", forbidden)
		}
	}
}

func TestHistoricalEvidenceMustBeInAcceptedContext(t *testing.T) {
	r, input := valid()
	input.Evidence[0].RunID = uuid.New()
	input.Evidence[0].ObservedAt = time.Date(2026, 9, 22, 10, 5, 0, 0, time.UTC)
	input.AsOf = input.Evidence[0].ObservedAt
	input.Context.Evidence = append([]incident.Evidence{}, input.Evidence...)
	if err := ValidateFinal(r, input); err != nil {
		t.Fatal("accepted prior evidence rejected", err)
	}
	input.AsOf = input.AsOf.Add(-time.Second)
	if err := ValidateFinal(r, input); err == nil {
		t.Fatal("future evidence accepted")
	}
	input.AsOf = input.AsOf.Add(time.Second)
	input.Evidence[0].RunID = uuid.New()
	if err := ValidateFinal(r, input); err == nil {
		t.Fatal("same ID from unrelated run accepted")
	}
}

func TestUnknownCauseStillRequiresLocalizedTarget(t *testing.T) {
	r, input := valid()
	r.RootCauseStatus = incident.CauseUnknown
	r.OpenQuestions = []incident.OpenQuestion{{ID: "Q1", Question: "Why did the worker stop?", WhyItMatters: "Find the underlying cause"}}
	if err := ValidateFinal(r, input); err != nil {
		t.Fatal("unknown underlying cause rejected despite localization", err)
	}
	input.Evidence[0].Kind = incident.ToolCallbacks
	input.Evidence[0].Available = false
	input.Evidence[0].Claims = []string{incident.ClaimCallbacksIncomplete}
	r.Claims = []string{incident.ClaimCallbacksIncomplete}
	if err := ValidateFinal(r, input); err == nil {
		t.Fatal("source gap accepted as owner localization")
	}
	r.RecommendedOwner = incident.OwnerTechOps
	r.RecommendedAction = incident.ActionRequestInformation
	if err := ValidateFinal(r, input); err != nil {
		t.Fatal("valid manual triage rejected", err)
	}
}

func TestHistoryRemainsUsableButCannotOverrideCurrentRouting(t *testing.T) {
	r, input := valid()
	old := input.Evidence[0]
	old.ID = "Eold"
	old.ObservedAt = input.AsOf.Add(-10 * time.Minute)
	input.Evidence = append(input.Evidence, old)
	r.EvidenceIDs = append(r.EvidenceIDs, old.ID)
	if err := ValidateFinal(r, input); err != nil {
		t.Fatal("historical context was banned", err)
	}
	input.Evidence[0].Claims = []string{incident.ClaimQueueHealthy}
	r.Hypotheses[0].SupportingEvidence = []string{old.ID}
	if err := ValidateFinal(r, input); err == nil {
		t.Fatal("old degraded state overrode healthy current state")
	}
}
