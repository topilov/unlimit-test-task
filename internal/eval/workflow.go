package eval

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"apm-investigator/internal/incident"
	"apm-investigator/internal/replay"
)

type checkFunc func(bool, string)

func approveEscalation(ctx context.Context, runner *replay.Runner, p *incident.Proposal, check checkFunc) error {
	count, err := runner.Store.TicketCount(ctx, p.ID)
	if err != nil {
		return err
	}
	check(count == 0, "ticket exists before approval")
	for _, note := range []string{"Synthetic evaluator human-approval fixture", "Repeated approval"} {
		if _, err = runner.Store.Decide(ctx, p.ID, true, note); err != nil {
			return err
		}
	}
	count, err = runner.Store.TicketCount(ctx, p.ID)
	if err != nil {
		return err
	}
	check(count == 1, "approval did not create exactly one ticket")
	return nil
}

func (report *Report) checkWorkflow(ctx context.Context, runner *replay.Runner, scenario *replay.Scenario, current incident.Incident, expectations Expected, check checkFunc) error {
	expected := expectations.Recovery
	matched := make([]bool, len(expected))
	investigations := make([]bool, len(expectations.Investigations))
	var last *replay.Outcome
	for current.Cursor < len(scenario.Events) {
		priorCursor := current.Cursor
		step := *runner
		step.AdvanceNext = true
		out, err := step.Run(ctx, scenario.Name, current.ID)
		if err != nil {
			return err
		}
		current = out.Incident
		if current.Status == incident.Closed {
			report.PrematureClosureCount++
			check(false, "incident closed without human approval")
		}
		if out.Recovery == nil {
			runs, err := runner.Store.Runs(ctx, current.ID)
			if err != nil {
				return err
			}
			if len(runs) == 0 {
				check(false, "missing investigation")
				break
			}
			run := runs[len(runs)-1]
			matchedStep := false
			for n, checkpoint := range expectations.Investigations {
				if current.Clock.Equal(checkpoint.At) {
					investigations[n], matchedStep = true, true
					check(current.Status == checkpoint.InitialStatus, "unexpected intermediate status")
					if err := report.checkAssessment(ctx, runner, run, checkpoint.AssessmentExpected, check); err != nil {
						return err
					}
					revised := 0
					if run.Result != nil {
						revised = revisedHypotheses(run.Result.Hypotheses, run.Context.Hypotheses)
					}
					check(revised >= checkpoint.MinRevisedHypotheses, "earlier hypotheses were not revised")
				}
			}
			check(matchedStep, "unexpected intermediate investigation")
			if out.Proposal != nil {
				if err := approveEscalation(ctx, runner, out.Proposal, check); err != nil {
					return err
				}
			}
			if current.Cursor <= priorCursor {
				check(false, "replay did not advance")
				break
			}
			continue
		}
		for n, c := range expected {
			if current.Clock.Equal(c.At) {
				matched[n] = true
				check(out.Recovery.ClosureAllowed == c.ClosureAllowed, fmt.Sprintf("unexpected recovery at %s", c.At))
			}
		}
		last = &out

		allowed := false
		for _, c := range expected {
			if !c.At.After(current.Clock) {
				allowed = c.ClosureAllowed
			}
		}
		if out.Recovery.ClosureAllowed && !allowed {
			report.PrematureClosureCount++
			check(false, "premature recovery pass")
		}
		check((out.Proposal != nil) == out.Recovery.ClosureAllowed, "recovery/proposal mismatch")
		if out.Proposal != nil {
			break
		}
		if current.Cursor <= priorCursor {
			check(false, "replay did not advance")
			break
		}
	}
	for n, found := range investigations {
		check(found, fmt.Sprintf("investigation checkpoint not reached: %s", expectations.Investigations[n].At))
	}
	for n, found := range matched {
		check(found, fmt.Sprintf("recovery checkpoint not reached: %s", expected[n].At))
	}
	if len(expected) == 0 {
		check(false, "monitoring scenario has no recovery expectations")
		return nil
	}
	final := expected[len(expected)-1]
	check(last != nil && last.Recovery.ClosureAllowed == final.ClosureAllowed, "unexpected final recovery outcome")
	if last != nil && last.Proposal != nil && final.ClosureAllowed {
		if _, err := runner.Store.Decide(ctx, last.Proposal.ID, true, "Synthetic evaluator closure approval"); err != nil {
			return err
		}
		closed, err := runner.Store.Get(ctx, current.ID)
		if err != nil {
			return err
		}
		check(closed.Status == incident.Closed, "approved closure not persisted")
	}
	return nil
}

func revisedHypotheses(current, previous []incident.Hypothesis) int {
	prior := make(map[string]incident.Hypothesis, len(previous))
	for _, h := range previous {
		prior[h.ID] = h
	}
	revised := 0
	for _, h := range current {
		old, exists := prior[h.ID]
		if exists && (h.Status != old.Status || !sameEvidenceRefs(h.SupportingEvidence, old.SupportingEvidence) || !sameEvidenceRefs(h.ContradictingEvidence, old.ContradictingEvidence) || strings.TrimSpace(h.Statement) != strings.TrimSpace(old.Statement)) {
			revised++
		}
	}
	return revised
}

func sameEvidenceRefs(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}
