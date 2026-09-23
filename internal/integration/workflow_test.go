package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	migrations "apm-investigator/db"
	"apm-investigator/internal/ai"
	"apm-investigator/internal/config"
	db "apm-investigator/internal/db/generated"
	"apm-investigator/internal/eval"
	"apm-investigator/internal/incident"
	"apm-investigator/internal/investigation"
	"apm-investigator/internal/replay"
	"apm-investigator/internal/store"
	"apm-investigator/internal/tools"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func database(t *testing.T) (*store.Store, string) {
	t.Helper()
	if os.Getenv("APM_INTEGRATION") != "1" {
		t.Skip("set APM_INTEGRATION=1 to use real PostgreSQL")
	}
	ctx := context.Background()
	base := os.Getenv("DATABASE_URL")
	if base == "" {
		t.Fatal("DATABASE_URL required")
	}
	conn, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	schema := "apm_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	ident := pgx.Identifier{schema}.Sanitize()

	if _, err = conn.Exec(ctx, "CREATE SCHEMA "+ident); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		conn.Exec(context.Background(), "DROP SCHEMA "+ident+" CASCADE")
		conn.Close(context.Background())
	})
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	dsn := u.String()
	if err = migrations.Apply(dsn); err != nil {
		t.Fatal("migrate from empty schema:", err)
	}
	if err = migrations.Apply(dsn); err != nil {
		t.Fatal("repeat migration:", err)
	}
	s, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, dsn
}
func runner(s *store.Store) *replay.Runner {
	return &replay.Runner{Store: s, Root: "../../testdata/scenarios", Config: config.AI{Mode: "mock", MaxSteps: 6}}
}
func newInvestigating(t *testing.T, s *store.Store) (incident.Incident, *tools.Registry) {
	t.Helper()
	scenario, err := replay.Load("../../testdata/scenarios", "queue-delay")
	if err != nil {
		t.Fatal(err)
	}
	impact, cohort := scenario.Sources.Base(scenario.Scope, scenario.StartedAt, false)
	i := incident.Incident{ID: uuid.New(), Status: incident.Investigating, Revision: 1, Scope: scenario.Scope, Scenario: scenario.Name, Clock: scenario.StartedAt, Impact: impact, Cohort: cohort, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err = s.Create(context.Background(), i); err != nil {
		t.Fatal(err)
	}
	return i, &tools.Registry{Sources: scenario.Sources, Now: i.Clock, Cohort: cohort}
}
func note(i incident.Incident) incident.Event {
	return incident.Event{ID: uuid.New(), IncidentID: i.ID, Source: "manual", ExternalID: uuid.NewString(), Kind: "manual.note", OccurredAt: i.Clock, AvailableAt: i.Clock, Payload: json.RawMessage(`{"text":"new evidence"}`)}
}
func TestIntegrationScenariosAndPersistence(t *testing.T) {
	s, dsn := database(t)
	ctx := context.Background()
	for _, name := range []string{"queue-delay", "receiver-failure", "insufficient-evidence"} {
		t.Run(name, func(t *testing.T) {
			report, err := eval.Run(ctx, runner(s), name)
			if err != nil {
				t.Fatal(err)
			}
			if !report.Passed {
				t.Fatalf("%+v", report)
			}
			reloaded, err := store.Open(ctx, dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer reloaded.Close()
			i, err := reloaded.Get(ctx, report.IncidentID)
			if err != nil {
				t.Fatal(err)
			}
			if i.Impact.UniquePayments != 5 || len(i.Cohort) != 3 {
				t.Fatalf("lost state: %+v", i)
			}
			entries, err := reloaded.Timeline(ctx, i.ID)
			if err != nil || len(entries) < 10 {
				t.Fatal("timeline incomplete", err, len(entries))
			}
			e, err := reloaded.Evidence(ctx, i.ID)
			if err != nil || len(e) < 4 {
				t.Fatal("evidence incomplete", err, len(e))
			}
			runs, err := reloaded.Runs(ctx, i.ID)
			if err != nil || len(runs) != 1 || len(runs[0].Turns) != 3 {
				t.Fatal("turn history lost", err, runs)
			}
			if name == "insufficient-evidence" && i.Status == incident.Closed {
				t.Fatal("unknown data closed")
			}
		})
	}
}
func TestIntegrationEventIdempotencyAndStaleProposal(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	out, err := runner(s).Run(ctx, "queue-delay", uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	events, err := s.Events(ctx, out.Incident.ID)
	if err != nil {
		t.Fatal(err)
	}
	inserted, err := s.Ingest(ctx, events[0], out.Incident.Clock, out.Incident.Cursor)
	if err != nil || inserted {
		t.Fatal("duplicate accepted", err)
	}
	i, err := s.Get(ctx, out.Incident.ID)
	if err != nil || i.Revision != out.Incident.Revision {
		t.Fatal("duplicate changed revision", err)
	}
	e := note(i)
	inserted, err = s.Ingest(ctx, e, i.Clock, i.Cursor)
	if err != nil || !inserted {
		t.Fatal(err)
	}
	if _, err = s.Decide(ctx, out.Proposal.ID, true, "stale"); !errors.Is(err, incident.ErrProposalStale) {
		t.Fatal("stale approval accepted", err)
	}
	n, err := s.TicketCount(ctx, out.Proposal.ID)
	if err != nil || n != 0 {
		t.Fatal("stale proposal created ticket", err)
	}
	future := note(i)
	future.AvailableAt = i.Clock.Add(time.Hour)
	if _, err = s.Ingest(ctx, future, i.Clock, i.Cursor); !errors.Is(err, incident.ErrInvalidInput) {
		t.Fatal("future event ingested", err)
	}
}
func TestIntegrationConcurrentApprovalExactlyOnce(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	out, err := runner(s).Run(ctx, "queue-delay", uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for k := 0; k < 8; k++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Decide(ctx, out.Proposal.ID, true, "concurrent reviewer")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.TicketCount(ctx, out.Proposal.ID)
	if err != nil || n != 1 {
		t.Fatal("ticket uniqueness", n, err)
	}
	i, err := s.Get(ctx, out.Incident.ID)
	if err != nil || i.Revision != out.Incident.Revision+1 || i.Status != incident.MonitoringRecovery {
		t.Fatal(i, err)
	}
}
func TestIntegrationRejectedEscalationCanReinvestigate(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	r := runner(s)
	out, err := r.Run(ctx, "queue-delay", uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Decide(ctx, out.Proposal.ID, false, "Need another review"); err != nil {
		t.Fatal(err)
	}
	again, err := r.Run(ctx, "queue-delay", out.Incident.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Proposal == nil || again.Proposal.ID == out.Proposal.ID {
		t.Fatal("new proposal missing")
	}
	ev, err := s.Evidence(ctx, out.Incident.ID)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range ev {
		if seen[e.ID] {
			t.Fatal("duplicate evidence ref")
		}
		seen[e.ID] = true
	}
}
func TestIntegrationStaleClosure(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	r := runner(s)
	out, err := r.Run(ctx, "queue-delay", uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Decide(ctx, out.Proposal.ID, true, "reviewed"); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Run(ctx, "queue-delay", out.Incident.ID); err != nil {
		t.Fatal(err)
	}
	full, err := r.Run(ctx, "queue-delay", out.Incident.ID)
	if err != nil || full.Proposal == nil {
		t.Fatal(err)
	}
	if full.Incident.Status == incident.Closed {
		t.Fatal("autonomous closure")
	}
	if _, err = s.Ingest(ctx, note(full.Incident), full.Incident.Clock, full.Incident.Cursor); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Decide(ctx, full.Proposal.ID, true, "stale closure"); !errors.Is(err, incident.ErrProposalStale) {
		t.Fatal(err)
	}
}
func TestIntegrationRunAndEvidenceConstraints(t *testing.T) {
	s, dsn := database(t)
	ctx := context.Background()
	i, _ := newInvestigating(t, s)
	r, err := s.StartRun(ctx, i, "mock", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartRun(ctx, i, "mock", true); err == nil {
		t.Fatal("two active investigations")
	}
	other, _ := newInvestigating(t, s)
	e := incident.Evidence{ID: "E001", IncidentID: other.ID, RunID: r.ID, Source: "payments", Kind: "base", ObservedAt: i.Clock, Data: json.RawMessage(`{}`)}
	if err = s.RecordEvidence(ctx, other, e); err == nil {
		t.Fatal("cross-incident evidence accepted")
	}
	e.IncidentID = i.ID
	if err = s.RecordEvidence(ctx, i, e); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordEvidence(ctx, i, e); err == nil {
		t.Fatal("duplicate evidence accepted")
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	_, err = db.New(conn).UpdateIncident(ctx, db.UpdateIncidentParams{ID: i.ID, Status: "invalid", Details: []byte(`{}`), UpdatedAt: time.Now(), Revision: i.Revision})
	if err == nil {
		t.Fatal("invalid status accepted")
	}
}

type agentFunc func(context.Context, ai.Input) (incident.Turn, incident.Metadata, error)

func (f agentFunc) Next(c context.Context, i ai.Input) (incident.Turn, incident.Metadata, error) {
	return f(c, i)
}
func TestIntegrationStaleModelResult(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	i, registry := newInvestigating(t, s)
	agent := agentFunc(func(ctx context.Context, input ai.Input) (incident.Turn, incident.Metadata, error) {
		if _, err := s.Ingest(ctx, note(i), i.Clock, 0); err != nil {
			t.Fatal(err)
		}
		result := &incident.Result{Summary: "Need more telemetry", FailureDomain: "unknown", RootCauseStatus: "unknown", Hypotheses: []incident.Hypothesis{{ID: "H1", Statement: "Callback dispatch may be delayed", Status: "open", SupportingEvidence: []string{input.Evidence[0].ID}}}, OpenQuestions: []incident.OpenQuestion{{ID: "Q1", Question: "Inspect attempts", WhyItMatters: "Distinguish failure"}}, EvidenceIDs: []string{input.Evidence[0].ID}, RecommendedOwner: "tech_ops", RecommendedAction: "continue_investigation"}
		return incident.Turn{Kind: "final", Final: result}, incident.Metadata{}, nil
	})
	service := investigation.Service{Store: s, Agent: agent, Registry: registry, MaxSteps: 2, Mode: "mock"}
	if _, err := service.Investigate(ctx, i.ID); !errors.Is(err, incident.ErrStaleRevision) {
		t.Fatal("stale result saved", err)
	}
	runs, err := s.Runs(ctx, i.ID)
	if err != nil || runs[0].Status != "superseded" {
		t.Fatal(err, runs)
	}
	props, err := s.Proposals(ctx, i.ID)
	if err != nil || len(props) != 0 {
		t.Fatal(err, props)
	}
}
func TestIntegrationManualTriageKeepsEvidence(t *testing.T) {
	for _, kind := range []string{"unavailable", "invalid", "budget"} {
		t.Run(kind, func(t *testing.T) {
			s, _ := database(t)
			ctx := context.Background()
			i, registry := newInvestigating(t, s)
			agent := agentFunc(func(ctx context.Context, in ai.Input) (incident.Turn, incident.Metadata, error) {
				switch kind {
				case "unavailable":
					return incident.Turn{}, incident.Metadata{}, incident.ErrModelUnavailable
				case "invalid":
					return incident.Turn{Kind: "bad"}, incident.Metadata{}, nil
				default:
					return incident.Turn{Kind: "tool_call", ToolCall: &incident.ToolCall{Name: "refund", Arguments: json.RawMessage(`{}`)}}, incident.Metadata{}, nil
				}
			})
			service := investigation.Service{Store: s, Agent: agent, Registry: registry, MaxSteps: 1, Mode: "mock"}
			_, err := service.Investigate(ctx, i.ID)
			if err == nil {
				t.Fatal("failure ignored")
			}
			current, err := s.Get(ctx, i.ID)
			if err != nil || current.Status != incident.ManualTriage {
				t.Fatal(err, current)
			}
			ev, err := s.Evidence(ctx, i.ID)
			if err != nil || len(ev) == 0 {
				t.Fatal("lost facts", err)
			}
			runs, err := s.Runs(ctx, i.ID)
			if err != nil || runs[0].Status != "incomplete" || len(runs[0].Turns) != 1 {
				t.Fatal(err, runs)
			}
			if kind == "budget" {
				tr, err := s.ToolRuns(ctx, runs[0].ID)
				if err != nil || len(tr) != 1 || tr[0].Status != "failed" {
					t.Fatal(err, tr)
				}
			}
		})
	}
}
func TestIntegrationConcurrentRunStart(t *testing.T) {
	s, _ := database(t)
	i, _ := newInvestigating(t, s)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for n := 0; n < 2; n++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.StartRun(context.Background(), i, "mock", true); results <- err }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal(fmt.Sprintf("%d starts succeeded", success))
	}
}
