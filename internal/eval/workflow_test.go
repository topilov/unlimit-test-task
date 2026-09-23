package eval

import (
	"testing"

	"apm-investigator/internal/incident"
)

func TestRevisedHypothesesCountsEvidenceChanges(t *testing.T) {
	previous := []incident.Hypothesis{
		{ID: "H1", Status: incident.HypothesisSupported, SupportingEvidence: []string{"E1", "E2"}},
		{ID: "H2", Status: incident.HypothesisOpen, SupportingEvidence: []string{"E1"}},
		{ID: "H3", Status: incident.HypothesisOpen},
	}
	current := []incident.Hypothesis{
		{ID: "H1", Status: incident.HypothesisSupported, SupportingEvidence: []string{"E2", "E1"}},
		{ID: "H2", Status: incident.HypothesisOpen, SupportingEvidence: []string{"E1", "E3"}},
		{ID: "H3", Status: incident.HypothesisWeakened, ContradictingEvidence: []string{"E3"}},
		{ID: "H4", Status: incident.HypothesisSupported, SupportingEvidence: []string{"E3"}},
	}
	if got := revisedHypotheses(current, previous); got != 2 {
		t.Fatalf("revised hypotheses = %d, want 2", got)
	}
}

func TestRevisedHypothesesCountsChangedStatement(t *testing.T) {
	previous := []incident.Hypothesis{{ID: "H1", Statement: "The queue caused the missing attempts", Status: incident.HypothesisOpen}}
	current := []incident.Hypothesis{{ID: "H1", Statement: "The queue may have contributed to delayed delivery", Status: incident.HypothesisOpen}}
	if got := revisedHypotheses(current, previous); got != 1 {
		t.Fatalf("revised hypotheses = %d, want 1", got)
	}
}
