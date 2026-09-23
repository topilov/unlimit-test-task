package eval

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"apm-investigator/internal/incident"
	"apm-investigator/internal/replay"
	"github.com/google/uuid"
)

func Run(ctx context.Context, runner *replay.Runner, name string) (report Report, err error) {
	start := time.Now()
	report = Report{
		EvaluationScope: "structured_contracts_and_workflow",
		MemoryIDs:       []uuid.UUID{},
		Scenario:        name,
		Failures:        []string{},
		Warnings:        []string{},
		RequiredTools:   map[string]bool{},
		ForbiddenClaims: []string{},
	}
	defer func() {
		report.LatencyMS = time.Since(start).Milliseconds()
		report.Passed = len(report.Failures) == 0 && err == nil
	}()
	s, err := replay.Load(runner.Root, name)
	if err != nil {
		return report, err
	}
	expected, err := loadExpected(s.Directory)
	if err != nil {
		return report, err
	}
	check := func(ok bool, message string) {
		if !ok {
			report.Failures = append(report.Failures, message)
		}
	}
	out, runErr := runner.Run(ctx, name, uuid.Nil)
	report.IncidentID = out.Incident.ID
	if runErr != nil {
		report.Failures = append(report.Failures, runErr.Error())
	}
	if out.Incident.ID == uuid.Nil {
		return report, nil
	}
	runs, err := runner.Store.Runs(ctx, out.Incident.ID)
	if err != nil {
		return report, err
	}
	if len(runs) == 0 {
		check(false, "no investigation run")
		return report, nil
	}
	run := runs[len(runs)-1]
	if err := report.checkAssessment(ctx, runner, run, expected.AssessmentExpected, check); err != nil {
		return report, err
	}
	check(out.Incident.Status == expected.InitialStatus, "unexpected initial incident status")
	if expected.InitialStatus == incident.ManualTriage && len(expected.Investigations) == 0 {
		check(out.Proposal == nil, "manual follow-up created an escalation proposal")
		check(len(expected.Recovery) == 0, "manual follow-up cannot auto-start recovery")
		return report, nil
	}
	if out.Proposal == nil && out.Incident.Status != incident.ManualTriage {
		check(false, "missing review proposal")
		return report, nil
	}
	if out.Proposal != nil && !expected.AdvanceBeforeApproval {
		if err = approveEscalation(ctx, runner, out.Proposal, check); err != nil {
			return report, err
		}
	}
	err = report.checkWorkflow(ctx, runner, s, out.Incident, expected, check)
	return report, err
}

func (report *Report) checkAssessment(ctx context.Context, runner *replay.Runner, run incident.Run, expected AssessmentExpected, check checkFunc) error {
	for _, rule := range run.Memory {
		if !slices.Contains(report.MemoryIDs, rule.ID) {
			report.MemoryIDs = append(report.MemoryIDs, rule.ID)
		}
	}
	toolsUsed := []string{}
	for _, turn := range run.Turns {
		report.ModelCalls++
		report.APICalls += turn.Metadata.Attempts
		report.InputTokens += turn.Metadata.InputTokens
		report.OutputTokens += turn.Metadata.OutputTokens
	}
	toolRuns, err := runner.Store.ToolRuns(ctx, run.ID)
	if err != nil {
		return err
	}
	report.ToolCalls += len(toolRuns)
	for _, t := range toolRuns {
		if t.Status == incident.ToolSucceeded {
			toolsUsed = append(toolsUsed, t.Name)
		}
	}
	for _, name := range expected.MustUseTools {
		used := slices.Contains(toolsUsed, name)
		report.RequiredTools[name] = used
		check(used, "required tool missing: "+name)
	}
	for _, name := range expected.ShouldUseTools {
		if !slices.Contains(toolsUsed, name) {
			report.Warnings = append(report.Warnings, "suggested tool missing: "+name)
		}
	}
	check(run.Status == incident.RunCompleted, "investigation did not complete")
	if run.Result != nil {
		result := run.Result
		for _, claim := range expected.MustDiscover {
			check(slices.Contains(result.Claims, claim), "missing observed claim: "+claim)
		}
		for _, claim := range expected.MustNotClaim {
			if slices.Contains(result.Claims, claim) {
				report.ForbiddenClaims = append(report.ForbiddenClaims, claim)
			}
		}
		check(len(report.ForbiddenClaims) == 0, "forbidden claims present")
		check(result.RecommendedAction == expected.Action, "unexpected recommended action: "+string(result.RecommendedAction))
		check(result.RecommendedOwner == expected.Owner, "unexpected owner: "+string(result.RecommendedOwner))
		check(result.RootCauseStatus == expected.RootCauseStatus, "unexpected root cause status")
	}

	return nil
}

func Summary(reports []Report) error {
	failed := []string{}
	for _, r := range reports {
		if !r.Passed {
			failed = append(failed, r.Scenario)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("scenario evaluation failed: %s", strings.Join(failed, ", "))
	}
	return nil
}
