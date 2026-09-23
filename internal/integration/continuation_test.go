package integration

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"apm-investigator/internal/ai"
	"apm-investigator/internal/eval"
	"apm-investigator/internal/incident"
	"apm-investigator/internal/investigation"
	"apm-investigator/internal/learning"
	"apm-investigator/internal/replay"
	"apm-investigator/internal/tools"
	"github.com/google/uuid"
)

func TestIntegrationOperatorRetry(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	i, registry := newInvestigating(t, s)
	service := investigation.Service{Store: s, Registry: registry, Mode: "mock", MaxSteps: 6, Agent: agentFunc(func(context.Context, ai.Input) (incident.Turn, incident.Metadata, error) {
		return incident.Turn{}, incident.Metadata{}, incident.ErrModelUnavailable
	})}
	if _, err := service.Investigate(ctx, i.ID); !errors.Is(err, incident.ErrModelUnavailable) {
		t.Fatal(err)
	}
	r := runner(s)
	idle, err := r.Run(ctx, i.Scenario, i.ID)
	if err != nil || idle.Incident.Status != incident.ManualTriage {
		t.Fatal(idle, err)
	}
	runs, err := s.Runs(ctx, i.ID)
	if err != nil || len(runs) != 1 {
		t.Fatal(runs, err)
	}
	r.Retry = true
	retried, err := r.Run(ctx, i.Scenario, i.ID)
	if err != nil || retried.Proposal == nil || retried.Incident.Status != incident.AwaitingReview || !retried.Incident.Clock.Equal(i.Clock) {
		t.Fatal(retried, err)
	}
	runs, err = s.Runs(ctx, i.ID)
	if err != nil || len(runs) != 2 || runs[0].Status != incident.RunIncomplete || runs[1].Status != incident.RunCompleted {
		t.Fatal(runs, err)
	}
	if _, err = r.Run(ctx, i.Scenario, i.ID); err == nil {
		t.Fatal("retry accepted outside manual triage")
	}
	if _, err = r.Run(ctx, i.Scenario, uuid.Nil); err == nil {
		t.Fatal("retry created incident")
	}
}

func TestIntegrationOperatorNextAndContinuation(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	r := runner(s)
	first, err := r.Run(ctx, "evolving-evidence", uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	idle, err := r.Run(ctx, "evolving-evidence", first.Incident.ID)
	if err != nil || idle.Incident.Cursor != first.Incident.Cursor {
		t.Fatal("implicit intake", idle, err)
	}
	r.AdvanceNext = true
	next, err := r.Run(ctx, "evolving-evidence", first.Incident.ID)
	if err != nil || next.Proposal == nil || next.Proposal.Owner != incident.OwnerMerchant {
		t.Fatal(next, err)
	}
	if next.Incident.Clock.Equal(first.Incident.Clock) || next.Incident.Revision <= first.Incident.Revision {
		t.Fatal("no new event", next)
	}
	if _, err := s.Decide(ctx, first.Proposal.ID, true, "old proposal"); !errors.Is(err, incident.ErrProposalStale) {
		t.Fatal("stale approval accepted", err)
	}
	for n := 0; n < 2; n++ {
		if _, err := s.Decide(ctx, next.Proposal.ID, true, "reviewed current evidence"); err != nil {
			t.Fatal(err)
		}
	}
	count, err := s.TicketCount(ctx, next.Proposal.ID)
	if err != nil || count != 1 {
		t.Fatal("ticket duplication", count, err)
	}
	for n := 0; n < 2; n++ {
		next, err = r.Run(ctx, "evolving-evidence", first.Incident.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if next.Recovery == nil || !next.Recovery.ClosureAllowed {
		t.Fatal(next)
	}
	if _, err := s.Decide(ctx, next.Proposal.ID, true, "original cohort and new traffic checked"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(ctx, "evolving-evidence", first.Incident.ID); err == nil {
		t.Fatal("advanced closed incident")
	}
}

func TestIntegrationContinuationAndFeedbackSnapshot(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	first, err := runner(s).Run(ctx, "queue-delay", uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	previous, err := s.Runs(ctx, first.Incident.ID)
	if err != nil {
		t.Fatal(err)
	}
	previousEvidence, err := s.RunEvidence(ctx, previous[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Decide(ctx, first.Proposal.ID, false, "reassess"); err != nil {
		t.Fatal(err)
	}
	scenario, err := replay.Load("../../testdata/scenarios", "queue-delay")
	if err != nil {
		t.Fatal(err)
	}
	mock, err := ai.LoadMock(scenario.Directory + "/mock_turns.json")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	service := investigation.Service{Store: s, Mode: "mock", MaxSteps: 6, Registry: &tools.Registry{Sources: scenario.Sources, Now: first.Incident.Clock, Cohort: first.Incident.Cohort}, Agent: agentFunc(func(ctx context.Context, input ai.Input) (incident.Turn, incident.Metadata, error) {
		if calls == 0 {
			if input.PreviousRunID != previous[0].ID || !reflect.DeepEqual(input.PreviousHypotheses, previous[0].Result.Hypotheses) || !reflect.DeepEqual(input.OpenQuestions, previous[0].Result.OpenQuestions) || len(input.ToolHistory) != 2 {
				t.Fatal("lost prior investigation", input)
			}
			for _, e := range previousEvidence {
				found := false
				for _, supplied := range input.Evidence {
					found = found || reflect.DeepEqual(e, supplied)
				}
				if !found {
					t.Fatal("evidence provenance lost", e.ID)
				}
			}
		}
		calls++
		return mock.Next(ctx, input)
	})}
	proposal, err := service.Investigate(ctx, first.Incident.ID)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := s.Runs(ctx, first.Incident.ID)
	if err != nil || len(runs) != 2 {
		t.Fatal(err, runs)
	}
	second := runs[1]
	if !reflect.DeepEqual(second.Context.Evidence, previousEvidence) {
		t.Fatal("context not persisted")
	}
	freshEvidence, err := s.RunEvidence(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range freshEvidence {
		if e.EventID != uuid.Nil {
			t.Fatal("historical event cloned into new run")
		}
	}
	f := feedbackFor(t, s, second)
	if _, err := s.Decide(ctx, proposal.ID, false, "another pass"); err != nil {
		t.Fatal(err)
	}
	current, err := s.Get(ctx, first.Incident.ID)
	if err != nil {
		t.Fatal(err)
	}
	event := note(current)
	event.Payload = json.RawMessage(`{"text":"FUTURE_CONTEXT_SENTINEL"}`)
	if _, err := s.Ingest(ctx, event, current.Clock, current.Cursor); err != nil {
		t.Fatal(err)
	}
	if _, err := runner(s).Run(ctx, current.Scenario, current.ID); err != nil {
		t.Fatal(err)
	}
	reflector := learning.Service{Store: s, Mode: "mock", Agent: reflectorFunc(func(ctx context.Context, input ai.ReflectionInput) (incident.Reflection, incident.Metadata, error) {
		encoded, _ := json.Marshal(input)
		if strings.Contains(string(encoded), "FUTURE_CONTEXT_SENTINEL") || input.RunID != second.ID || !reflect.DeepEqual(input.Context, second.Context) {
			t.Fatal("reflection changed with incident", string(encoded))
		}
		ref := ""
		for _, e := range input.Context.Evidence {
			if e.Available {
				ref = e.ID
				break
			}
		}
		if ref == "" {
			t.Fatal("missing historical evidence")
		}
		return incident.Reflection{Summary: "Retain the useful earlier check", Lesson: &incident.LessonDraft{Condition: "Callbacks are missing", Check: "Inspect dispatch before receiver", Rationale: "Distinguishes absence from rejection", EvidenceIDs: []string{ref}}}, incident.Metadata{}, nil
	})}
	if result, err := reflector.Reflect(ctx, f.ID); err != nil || result.Lesson == nil {
		t.Fatal(result, err)
	}
}

func TestIntegrationContinuationSkipsSupersededRun(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	first, err := runner(s).Run(ctx, "queue-delay", uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := s.Runs(ctx, first.Incident.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Decide(ctx, first.Proposal.ID, false, "reassess"); err != nil {
		t.Fatal(err)
	}
	i, err := s.Get(ctx, first.Incident.ID)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := s.StartRun(ctx, i, "mock", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Ingest(ctx, note(i), i.Clock, i.Cursor); err != nil {
		t.Fatal(err)
	}
	stale.Status = incident.RunCompleted
	stale.Result = &incident.Result{Summary: "SUPERSEDED_SENTINEL", RecommendedAction: incident.ActionContinue}
	if _, err := s.Finish(ctx, i, stale); !errors.Is(err, incident.ErrStaleRevision) {
		t.Fatal(err)
	}
	if _, err := runner(s).Run(ctx, i.Scenario, i.ID); err != nil {
		t.Fatal(err)
	}
	runs, err := s.Runs(ctx, i.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 3 || runs[1].Status != incident.RunSuperseded || runs[2].Context.PreviousRunID != applied[0].ID {
		t.Fatal("superseded result reused", runs)
	}
}

func TestIntegrationEvolvingEvaluation(t *testing.T) {
	s, _ := database(t)
	report, err := eval.Run(context.Background(), runner(s), "evolving-evidence")
	if err != nil || !report.Passed {
		t.Fatal(report, err)
	}
	if report.ModelCalls != 6 || report.ToolCalls != 4 || report.ProseEvaluated {
		t.Fatal("incorrect evaluation scope or totals", report)
	}
}

func TestIntegrationContinuationRefinesIncompleteCohort(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	i, _ := newInvestigating(t, s)
	i.Impact.CohortComplete = false
	i.ID = uuid.New()
	i.Cohort = []string{"p1"}
	i.Impact.UniquePayments = 1
	i.Impact.AffectedPayments = 1
	if err := s.Create(ctx, i); err != nil {
		t.Fatal(err)
	}
	partial, err := s.RefineCohort(ctx, i, 2, []string{"p2", "p3"}, true)
	if err != nil || partial.Impact.CohortComplete || len(partial.Cohort) != 3 {
		t.Fatal("lost original item", partial, err)
	}
	complete, err := s.RefineCohort(ctx, partial, 5, []string{"p1", "p2", "p3"}, true)
	if err != nil || !complete.Impact.CohortComplete || len(complete.Cohort) != 3 || complete.Impact.AffectedPayments != 3 {
		t.Fatal(complete, err)
	}
	unchanged, err := s.RefineCohort(ctx, complete, 1, []string{"p3"}, true)
	if err != nil || !reflect.DeepEqual(unchanged, complete) {
		t.Fatal("replaced original cohort", unchanged, err)
	}
	if _, err := s.RefineCohort(ctx, i, 5, []string{"p1", "p2", "p3"}, true); !errors.Is(err, incident.ErrStaleRevision) {
		t.Fatal("stale refinement accepted", err)
	}
}

func TestIntegrationContinuationLateCompleteMembershipAllowsRecovery(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	root, dir := fixtureCopy(t)
	scenario, err := replay.Load(root, "queue-delay")
	if err != nil {
		t.Fatal(err)
	}
	initial := scenario.Sources.Payments[0]
	initial.Complete = false
	later := initial
	later.Complete = true
	later.ObservedAt = scenario.Events[len(scenario.Events)-1].AvailableAt
	later.AvailableAt = later.ObservedAt
	writeJSON(t, dir+"/sources/payments.json", []tools.Snapshot{initial, later})
	r := runner(s)
	r.Root = root
	out, err := r.Run(ctx, "queue-delay", uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Incident.Impact.CohortComplete {
		t.Fatal("incomplete source became complete")
	}
	original := append([]string{}, out.Incident.Cohort...)
	if _, err := s.Decide(ctx, out.Proposal.ID, true, "reviewed"); err != nil {
		t.Fatal(err)
	}
	out, err = r.Run(ctx, "queue-delay", out.Incident.ID)
	if err != nil {
		t.Fatal(err)
	}
	if out.Recovery.ClosureAllowed || out.Incident.Impact.CohortComplete {
		t.Fatal("premature complete membership")
	}
	out, err = r.Run(ctx, "queue-delay", out.Incident.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Incident.Impact.CohortComplete || !out.Recovery.ClosureAllowed || !reflect.DeepEqual(out.Incident.Cohort, original) {
		t.Fatal("late complete membership not applied safely", out)
	}
}
