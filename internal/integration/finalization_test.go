package integration

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"apm-investigator/internal/ai"
	"apm-investigator/internal/incident"
	"apm-investigator/internal/investigation"
	"apm-investigator/internal/recovery"
	"apm-investigator/internal/tools"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestIntegrationLateSnapshotCannotProposeClosure(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	base := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	scope := incident.Scope{MerchantID: "m", Method: "apm", Environment: "sandbox", WindowStart: base, WindowEnd: base.Add(5 * time.Minute)}
	i := incident.Incident{ID: uuid.New(), Status: incident.MonitoringRecovery, Revision: 1, Clock: base.Add(8 * time.Minute), Scope: scope, Cohort: []string{"p1"}, Impact: incident.Impact{CohortComplete: true}, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := s.Create(ctx, i); err != nil {
		t.Fatal(err)
	}
	bad := tools.Snapshot{Scope: scope, ObservedAt: base.Add(7 * time.Minute), AvailableAt: base.Add(7 * time.Minute), Available: true, Complete: true, Attempts: []tools.Attempt{{ID: "a1", PaymentID: "p1", HTTPStatus: 200, At: base.Add(6 * time.Minute)}}, NewTrafficChecked: 10, NewTrafficSuccessful: 0}
	oldGood := bad
	oldGood.ObservedAt, oldGood.AvailableAt, oldGood.NewTrafficSuccessful = base.Add(6*time.Minute), base.Add(8*time.Minute), 10
	service := recovery.Service{Store: s, Sources: &tools.Sources{Callbacks: []tools.Snapshot{bad, oldGood}}}
	result, proposal, err := service.Check(ctx, i)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.Get(ctx, i.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("actual recovery service: new_traffic=%s closure_allowed=%t proposal=%t stored_status=%s", result.NewTrafficHealthy, result.ClosureAllowed, proposal != nil, saved.Status)
	if proposal != nil || result.ClosureAllowed {
		t.Fatal("a closure proposal was persisted despite the newer unhealthy observation")
	}
}

type cancelAfterFinalHandler struct{ cancel context.CancelFunc }

func (h cancelAfterFinalHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h cancelAfterFinalHandler) Handle(_ context.Context, record slog.Record) error {
	record.Attrs(func(a slog.Attr) bool {
		if a.Key == "step" && a.Value.Int64() == 3 {
			h.cancel()
		}
		return true
	})
	return nil
}
func (h cancelAfterFinalHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h cancelAfterFinalHandler) WithGroup(string) slog.Handler      { return h }

func TestIntegrationFinalization(t *testing.T) {
	for _, kind := range []string{"cancel_after_final_turn", "database_rejects_final_proposal"} {
		t.Run(kind, func(t *testing.T) {
			s, dsn := database(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			i, registry := newInvestigating(t, s)
			mock, err := ai.LoadMock("../../testdata/scenarios/queue_delay/mock_turns.json")
			if err != nil {
				t.Fatal(err)
			}
			service := investigation.Service{Store: s, Registry: registry, Agent: mock, Mode: "mock", MaxSteps: 6}
			if kind == "cancel_after_final_turn" {
				service.Logger = slog.New(cancelAfterFinalHandler{cancel})
			} else {
				conn, err := pgx.Connect(ctx, dsn)
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close(context.Background())
				_, err = conn.Exec(ctx, `CREATE FUNCTION review_reject_proposal() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'review injected final-write failure'; END; $$; CREATE TRIGGER review_reject_proposal BEFORE INSERT ON proposals FOR EACH ROW EXECUTE FUNCTION review_reject_proposal();`)
				if err != nil {
					t.Fatal(err)
				}
			}
			proposal, investigateErr := service.Investigate(ctx, i.ID)
			if investigateErr == nil || proposal != nil {
				t.Fatal("failure injection did not fire")
			}
			fresh := context.Background()
			runs, err := s.Runs(fresh, i.ID)
			if err != nil || len(runs) != 1 || len(runs[0].Turns) != 3 {
				t.Fatal("failure was not after final turn", runs, err)
			}
			current, err := s.Get(fresh, i.ID)
			if err != nil {
				t.Fatal(err)
			}
			if runs[0].Status != incident.RunIncomplete || current.Status != incident.ManualTriage {
				t.Fatal("final failure is not retryable", runs[0].Status, current.Status)
			}
			if kind == "database_rejects_final_proposal" {
				conn, err := pgx.Connect(fresh, dsn)
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close(fresh)
				if _, err := conn.Exec(fresh, "DROP TRIGGER review_reject_proposal ON proposals"); err != nil {
					t.Fatal(err)
				}
			}
			retry := runner(s)
			retry.Retry = true
			resumed, err := retry.Run(fresh, i.Scenario, i.ID)
			if err != nil || resumed.Proposal == nil || resumed.Incident.Status != incident.AwaitingReview {
				t.Fatal("could not retry after final-write failure", resumed, err)
			}
		})
	}
}

func TestIntegrationFinalizationCannotRewriteCompletedRun(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	i, _ := newInvestigating(t, s)
	run, err := s.StartRun(ctx, i, "mock", false)
	if err != nil {
		t.Fatal(err)
	}
	run.Status = incident.RunCompleted
	run.Result = &incident.Result{Summary: "Applied result", RecommendedAction: incident.ActionContinue}
	if _, err := s.Finish(ctx, i, run); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Finish(ctx, i, run); !errors.Is(err, incident.ErrStaleRevision) {
		t.Fatal(err)
	}
	saved, err := s.Run(ctx, run.ID)
	if err != nil || saved.Status != incident.RunCompleted || saved.Result.Summary != "Applied result" {
		t.Fatal("terminal run overwritten", saved, err)
	}
}
