package investigation

import (
	"fmt"
	"slices"
	"strings"

	"apm-investigator/internal/ai"
	"apm-investigator/internal/incident"
	"apm-investigator/internal/policy"
)

func invalid(why string) error { return fmt.Errorf("%w: %s", incident.ErrModelInvalidOutput, why) }
func ValidateHypotheses(hypotheses []incident.Hypothesis, input ai.Input) error {
	known := map[string]incident.Evidence{}
	for _, e := range input.Evidence {
		if allowedEvidence(e, input) {
			known[e.ID] = e
		}
	}
	ids := map[string]bool{}
	for _, h := range hypotheses {
		if strings.TrimSpace(h.ID) == "" || strings.TrimSpace(h.Statement) == "" || ids[h.ID] {
			return invalid("invalid/duplicate hypothesis")
		}
		ids[h.ID] = true
		if !slices.Contains([]incident.HypothesisStatus{incident.HypothesisOpen, incident.HypothesisSupported, incident.HypothesisWeakened, incident.HypothesisRejected}, h.Status) {
			return invalid("hypothesis status")
		}
		if h.Status == incident.HypothesisSupported && len(h.SupportingEvidence) == 0 {
			return invalid("unsupported hypothesis")
		}
		if (h.Status == incident.HypothesisWeakened || h.Status == incident.HypothesisRejected) && len(h.ContradictingEvidence) == 0 {
			return invalid("uncited hypothesis revision")
		}
		if len(h.SupportingEvidence)+len(h.ContradictingEvidence) == 0 && (h.Status != incident.HypothesisOpen || len(h.MissingEvidence) == 0) {
			return invalid("uncited hypothesis")
		}
		for _, ref := range append(append([]string{}, h.SupportingEvidence...), h.ContradictingEvidence...) {
			if _, ok := known[ref]; !ok {
				return invalid("unknown evidence " + ref)
			}
		}
		if h.Status == incident.HypothesisSupported {
			available := false
			for _, ref := range h.SupportingEvidence {
				e := known[ref]
				available = available || e.Available || (e.Kind == incident.ToolCallbacks && slices.Contains(e.Claims, incident.ClaimCallbacksIncomplete))
			}
			if !available {
				return invalid("support consists only of unavailable evidence")
			}
		}
	}
	return nil
}
func ValidateFinal(r *incident.Result, input ai.Input) error {
	if r == nil || strings.TrimSpace(r.Summary) == "" || strings.TrimSpace(r.FailureDomain) == "" || len(r.EvidenceIDs) == 0 || len(r.Hypotheses) == 0 {
		return invalid("missing result fields")
	}
	if !slices.Contains([]incident.RootCauseStatus{incident.CauseUnknown, incident.CauseLocalized, incident.CauseConfirmed}, r.RootCauseStatus) {
		return invalid("root cause status")
	}
	if !slices.Contains(incident.Actions(), r.RecommendedAction) {
		return invalid("forbidden action")
	}
	if r.RecommendedAction == incident.ActionProposeClosure {
		return invalid("recovery must be verified by Go")
	}
	if err := ValidateHypotheses(r.Hypotheses, input); err != nil {
		return err
	}
	known := map[string]incident.Evidence{}
	claims := map[string]bool{}
	confirmed := false
	for _, e := range input.Evidence {
		if !allowedEvidence(e, input) {
			continue
		}
		known[e.ID] = e
		if slices.Contains(r.EvidenceIDs, e.ID) {
			for _, c := range e.Claims {
				claims[c] = true
			}
		}
		if e.Available && e.Kind == "root_cause_confirmation" {
			confirmed = true
		}
	}
	for _, ref := range r.EvidenceIDs {
		if _, ok := known[ref]; !ok {
			return invalid("unknown result evidence")
		}
	}
	for _, c := range r.Claims {
		if !claims[c] {
			return invalid("unobserved claim " + c)
		}
	}
	if r.RootCauseStatus == incident.CauseConfirmed && !confirmed {
		return invalid("no direct root cause confirmation")
	}
	if r.RootCauseStatus == incident.CauseLocalized {
		if !claims[incident.ClaimCallbacksMissing] && !claims[incident.ClaimReceiver503] && !claims[incident.ClaimQueueDegraded] {
			return invalid("localization without diagnostic evidence")
		}
		supported := false
		for _, h := range r.Hypotheses {
			supported = supported || h.Status == incident.HypothesisSupported
		}
		if !supported {
			return invalid("localization without supporting hypothesis")
		}
	}
	if r.RootCauseStatus == incident.CauseUnknown && len(r.OpenQuestions) == 0 {
		return invalid("unknown cause requires missing evidence questions")
	}
	questions := map[string]bool{}
	for _, q := range r.OpenQuestions {
		if strings.TrimSpace(q.ID) == "" || strings.TrimSpace(q.Question) == "" || strings.TrimSpace(q.WhyItMatters) == "" || questions[q.ID] {
			return invalid("empty/duplicate open question")
		}
		questions[q.ID] = true
	}
	for _, name := range r.ExecutedTools {
		found := false
		for _, t := range input.ToolHistory {
			found = found || (t.Name == name && t.Status == incident.ToolSucceeded)
		}
		if !found {
			return invalid("unexecuted tool " + name)
		}
	}
	r.RecommendedOwner = policy.Owner(r.RecommendedOwner)
	if r.RecommendedAction == incident.ActionEscalate {
		evidence := make([]incident.Evidence, 0, len(known))
		for _, e := range known {
			evidence = append(evidence, e)
		}
		if !policy.CanRoute(r, evidence, input.AsOf) {
			return invalid("targeted escalation lacks current localization evidence")
		}
	}
	return nil
}

func ValidateTurn(turn incident.Turn, input ai.Input) error {
	switch turn.Kind {
	case incident.TurnToolCall:
		if turn.ToolCall == nil || turn.Final != nil {
			return invalid("exactly one top-level action required")
		}
		return ValidateHypotheses(turn.Hypotheses, input)
	case incident.TurnFinal:
		if turn.Final == nil || turn.ToolCall != nil {
			return invalid("exactly one top-level action required")
		}
		if err := ValidateHypotheses(turn.Hypotheses, input); err != nil {
			return err
		}
		return ValidateFinal(turn.Final, input)
	default:
		return invalid("exactly one top-level action required")
	}
}

func allowedEvidence(e incident.Evidence, input ai.Input) bool {
	if e.IncidentID != input.IncidentID || (!input.AsOf.IsZero() && e.ObservedAt.After(input.AsOf)) {
		return false
	}
	if e.RunID == input.RunID {
		return true
	}
	for _, prior := range input.Context.Evidence {
		if e.ID == prior.ID && e.RunID == prior.RunID {
			return true
		}
	}
	return false
}
