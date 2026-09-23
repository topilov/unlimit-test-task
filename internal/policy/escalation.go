package policy

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"apm-investigator/internal/incident"
)

func escalationBody(i incident.Incident, run incident.Run, evidence []incident.Evidence) string {
	r := run.Result
	var b strings.Builder
	fmt.Fprintf(&b, "Incident: %s | revision: %d | run: %s\n", i.ID, run.BaseRevision, run.ID)
	fmt.Fprintf(&b, "Scope: %s / %s / %s\nWindow: %s — %s | as of: %s\n", i.Scope.MerchantID, i.Scope.Method, i.Scope.Environment, i.Scope.WindowStart.Format(time.RFC3339), i.Scope.WindowEnd.Format(time.RFC3339), i.Clock.Format(time.RFC3339))
	fmt.Fprintf(&b, "Impact: %d potentially affected / %d unique payments; membership complete=%t; severity=%s\n", i.Impact.AffectedPayments, i.Impact.UniquePayments, i.Impact.CohortComplete, Severity(i.Impact))
	fmt.Fprintf(&b, "Owner: %s | root cause: %s\nAssessment: %s\n", Owner(r.RecommendedOwner), r.RootCauseStatus, r.Summary)
	refs := append([]string{}, r.EvidenceIDs...)
	for _, h := range r.Hypotheses {
		fmt.Fprintf(&b, "Hypothesis %s (%s): %s [support: %s; contradict: %s]\n", h.ID, h.Status, h.Statement, strings.Join(h.SupportingEvidence, ", "), strings.Join(h.ContradictingEvidence, ", "))
		refs = append(refs, h.SupportingEvidence...)
		refs = append(refs, h.ContradictingEvidence...)
	}
	for _, e := range evidence {
		if !slices.Contains(refs, e.ID) {
			continue
		}
		fmt.Fprintf(&b, "Evidence %s | %s | observed %s | usable=%t | run %s: %s\n", e.ID, e.Source, e.ObservedAt.Format(time.RFC3339), e.Available, e.RunID, e.Summary)
	}
	for _, q := range r.OpenQuestions {
		fmt.Fprintf(&b, "Next check: %s — %s\n", q.Question, q.WhyItMatters)
	}
	fmt.Fprint(&b, "Closure requires fresh callback acknowledgements for every original item and healthy new traffic, followed by human approval.")
	return b.String()
}
