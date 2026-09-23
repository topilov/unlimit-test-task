package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	migrations "apm-investigator/db"
	"apm-investigator/internal/eval"
	"apm-investigator/internal/incident"
	"apm-investigator/internal/tools"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func fixtureCopy(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "queue_delay")
	if err := os.CopyFS(dir, os.DirFS("../../testdata/scenarios/queue_delay")); err != nil {
		t.Fatal(err)
	}
	return root, dir
}
func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func TestIntegrationReplayDuplicateAdvancesCursor(t *testing.T) {
	s, _ := database(t)
	root, dir := fixtureCopy(t)
	path := filepath.Join(dir, "events.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	end := 0
	for raw[end] != '\n' {
		end++
	}
	duplicate := append(append([]byte{}, raw[:end+1]...), raw...)
	if err = os.WriteFile(path, duplicate, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	r := runner(s)
	r.Root = root
	out, err := r.Run(ctx, "queue-delay", uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Incident.Cursor != 2 || out.Incident.Revision != 3 || out.Proposal == nil {
		t.Fatalf("duplicate changed domain state: %+v", out)
	}
	events, err := s.Events(ctx, out.Incident.ID)
	if err != nil || len(events) != 1 {
		t.Fatal(events, err)
	}
}
func TestIntegrationIncompleteOriginalMembershipBlocksClosure(t *testing.T) {
	s, _ := database(t)
	root, dir := fixtureCopy(t)
	path := filepath.Join(dir, "sources", "payments.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snapshots []tools.Snapshot
	if err = json.Unmarshal(data, &snapshots); err != nil {
		t.Fatal(err)
	}
	snapshots[0].Complete = false
	writeJSON(t, path, snapshots)
	ctx := context.Background()
	r := runner(s)
	r.Root = root
	out, err := r.Run(ctx, "queue-delay", uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Incident.Impact.CohortComplete {
		t.Fatal("incomplete membership lost")
	}
	if _, err = s.Decide(ctx, out.Proposal.ID, true, "test reviewer"); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 2; n++ {
		out, err = r.Run(ctx, "queue-delay", out.Incident.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if out.Recovery == nil || out.Recovery.ClosureAllowed || out.Recovery.OriginalCohortRecovered != incident.Unknown || out.Proposal != nil || out.Incident.Status != incident.MonitoringRecovery {
		t.Fatalf("partial membership closed: %+v", out)
	}
}
func TestIntegrationEvaluationAcceptsExtraRecoveryBatch(t *testing.T) {
	s, _ := database(t)
	root, dir := fixtureCopy(t)
	path := filepath.Join(dir, "events.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var events []incident.Event
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		var e incident.Event
		if err = json.Unmarshal(line, &e); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	extra := events[1]
	extra.ExternalID = "extra-check"
	extra.Source = "manual"
	extra.Kind = incident.EventNote
	extra.AvailableAt = extra.AvailableAt.Add(time.Minute)
	extra.OccurredAt = extra.AvailableAt
	extra.Payload = json.RawMessage(`{"purpose":"recovery_check"}`)
	events = append(events[:2], extra, events[2])
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	for _, e := range events {
		if err = encoder.Encode(e); err != nil {
			t.Fatal(err)
		}
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	r := runner(s)
	r.Root = root
	report, err := eval.Run(context.Background(), r, "queue-delay")
	if err != nil || !report.Passed {
		t.Fatal(err, report)
	}
	if report.ProseEvaluated || report.EvaluationScope != "structured_contracts_and_workflow" {
		t.Fatal("overclaimed evaluation coverage")
	}
}
func TestIntegrationRecoveryDoesNotRequireModelCredentials(t *testing.T) {
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
	r.Config.Mode = "live"
	r.Config.APIKey = ""
	out, err = r.Run(ctx, "queue-delay", out.Incident.ID)
	if err != nil || out.Recovery == nil {
		t.Fatal("deterministic recovery initialized AI", err)
	}
}
func TestIntegrationMigrationPreservesExistingIncident(t *testing.T) {
	s, dsn := database(t)
	ctx := context.Background()
	before, _ := newInvestigating(t, s)
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	paths, err := filepath.Glob("../../db/migrations/*.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	for n := len(paths) - 1; n > 0; n-- {
		down, err := os.ReadFile(paths[n])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(ctx, string(down)); err != nil {
			t.Fatal(err)
		}
	}

	if _, err = conn.Exec(ctx, `UPDATE incidents SET state=state #- '{impact,cohort_complete}'; UPDATE schema_migrations SET version=1`); err != nil {
		t.Fatal(err)
	}
	if err = migrations.Apply(dsn); err != nil {
		t.Fatal(err)
	}
	after, err := s.Get(ctx, before.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != before.Status || after.Revision != before.Revision || after.Scope != before.Scope || after.Scenario != before.Scenario || len(after.Cohort) != len(before.Cohort) || after.Impact.CohortComplete {
		t.Fatalf("migration lost data or assumed completeness: %+v", after)
	}
	var duplicateKeys bool
	if err = conn.QueryRow(ctx, `SELECT details ?| ARRAY['id','status','revision','scope','created_at','updated_at'] FROM incidents WHERE id=$1`, before.ID).Scan(&duplicateKeys); err != nil || duplicateKeys {
		t.Fatal("duplicated authority in details", err)
	}
}

func TestIntegrationDuplicateProgressSurvivesInvestigationFinish(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	i, _ := newInvestigating(t, s)
	event := note(i)
	if _, err := s.Ingest(ctx, event, i.Clock, 1); err != nil {
		t.Fatal(err)
	}
	i, err := s.Get(ctx, i.ID)
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.StartRun(ctx, i, "mock", true)
	if err != nil {
		t.Fatal(err)
	}
	if inserted, err := s.Ingest(ctx, event, i.Clock, 2); err != nil || inserted {
		t.Fatal("duplicate not idempotent", err)
	}
	run.Status = incident.RunCompleted
	run.Result = &incident.Result{RecommendedAction: incident.ActionContinue}
	if _, err = s.Finish(ctx, i, run); err != nil {
		t.Fatal(err)
	}
	current, err := s.Get(ctx, i.ID)
	if err != nil || current.Cursor != 2 || current.Revision != i.Revision+1 {
		t.Fatal("completion rewound duplicate progress", current, err)
	}
}

func TestIntegrationLegacyClosureCannotBypassMembershipCheck(t *testing.T) {
	s, dsn := database(t)
	ctx := context.Background()
	r := runner(s)
	out, err := r.Run(ctx, "queue-delay", uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Decide(ctx, out.Proposal.ID, true, "reviewed"); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 2; n++ {
		out, err = r.Run(ctx, "queue-delay", out.Incident.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err = conn.Exec(ctx, `UPDATE incidents SET details=details #- '{impact,cohort_complete}' WHERE id=$1`, out.Incident.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Decide(ctx, out.Proposal.ID, true, "legacy proposal"); !errors.Is(err, incident.ErrInvalidInput) {
		t.Fatal("legacy closure bypassed missing membership proof", err)
	}
	if _, err = s.Decide(ctx, out.Proposal.ID, false, "need complete population"); err != nil {
		t.Fatal("must still be able to reject", err)
	}
}
