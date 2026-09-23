package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"apm-investigator/internal/incident"
	"apm-investigator/internal/replay"
	"apm-investigator/internal/tools"
)

func fixture(t *testing.T, name string) *replay.Scenario {
	t.Helper()
	s, err := replay.Load("../../testdata/scenarios", name)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestUniquePaymentsAndRetryCounts(t *testing.T) {
	s := fixture(t, "receiver-failure")
	p := s.Sources.PaymentSummary(s.Scope, s.StartedAt)
	if p.Total != 5 || p.Completed != 3 || p.Pending != 1 || p.Failed != 1 {
		t.Fatalf("%+v", p)
	}
	c := s.Sources.CallbackSummary(s.Scope, s.StartedAt, p.CompletedIDs)
	if c.PaymentsChecked != 3 || c.WithAttempts != 3 || c.HTTPStatuses["503"] != 4 || c.Successful != 0 {
		t.Fatalf("%+v", c)
	}
	impact, cohort := s.Sources.Base(s.Scope, s.StartedAt, false)
	if impact.AffectedPayments != 3 || len(cohort) != 3 {
		t.Fatalf("retry inflated impact: %+v", impact)
	}
}
func TestFutureAndCrossScopeDataUnavailable(t *testing.T) {
	s := fixture(t, "queue-delay")
	c := s.Sources.CallbackSummary(s.Scope, s.StartedAt, []string{"p1", "p2", "p3"})
	if c.Successful != 0 || c.WithAttempts != 0 {
		t.Fatalf("future leaked: %+v", c)
	}
	future := s.StartedAt.Add(21 * time.Minute)
	c = s.Sources.CallbackSummary(s.Scope, future, []string{"p1", "p2", "p3"})
	if c.Successful != 3 {
		t.Fatal("positive control: later evidence missing")
	}
	other := s.Scope
	other.MerchantID = "other"
	p := s.Sources.PaymentSummary(other, s.StartedAt)
	if p.Total != 0 || p.DataComplete {
		t.Fatal("cross-scope data leaked")
	}
	p = s.Sources.PaymentSummary(s.Scope, s.StartedAt.Add(-time.Second))
	if p.DataComplete || p.Total != 0 {
		t.Fatal("unavailable data leaked")
	}
}
func TestToolBoundary(t *testing.T) {
	s := fixture(t, "queue-delay")
	r := tools.Registry{Sources: s.Sources, Now: s.StartedAt, Cohort: []string{"p1", "p2", "p3"}}
	for _, tc := range []struct {
		name, args string
		want       error
	}{
		{"get_callback_attempts", `{"payment_ids":["other"]}`, incident.ErrToolScopeViolation},
		{"get_callback_attempts", `{"payment_ids":[]}`, incident.ErrInvalidInput},
		{"get_callback_attempts", `{"payment_ids":["p1"],"merchant_id":"other"}`, incident.ErrInvalidInput},
		{"get_queue_health", `{"url":"https://example.invalid"}`, incident.ErrInvalidInput},
		{"get_queue_health", `null`, incident.ErrInvalidInput},
		{"get_queue_health", `{} {}`, incident.ErrInvalidInput},
		{"get_payment_summary", `{"cohort":"all"}`, incident.ErrToolScopeViolation},
		{"refund", `{}`, incident.ErrUnknownTool},
	} {
		_, err := r.Execute(context.Background(), s.Scope, incident.ToolCall{Name: tc.name, Arguments: json.RawMessage(tc.args)})
		if !errors.Is(err, tc.want) {
			t.Errorf("%s %s: %v", tc.name, tc.args, err)
		}
	}
	e, err := r.Execute(context.Background(), s.Scope, incident.ToolCall{Name: "get_callback_attempts", Arguments: json.RawMessage(`{"payment_ids":["p1","p1"]}`)})
	if err != nil {
		t.Fatal(err)
	}
	var c tools.CallbackSummary
	json.Unmarshal(e.Data, &c)
	if c.PaymentsChecked != 1 {
		t.Fatal("duplicate ID counted twice")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = r.Execute(ctx, s.Scope, incident.ToolCall{Name: "get_queue_health", Arguments: json.RawMessage(`{}`)}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestMissingAndStaleSources(t *testing.T) {
	s := fixture(t, "insufficient-evidence")
	c := s.Sources.CallbackSummary(s.Scope, s.StartedAt, []string{"p1"})
	if c.DataComplete || c.SourceAvailable {
		t.Fatal("missing source became complete")
	}
	r := tools.Registry{Sources: s.Sources, Now: s.StartedAt}
	e, err := r.Execute(context.Background(), s.Scope, incident.ToolCall{Name: "get_provider_status", Arguments: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Fresh bool `json:"fresh_for_incident_window"`
	}
	json.Unmarshal(e.Data, &data)
	if data.Fresh {
		t.Fatal("stale green treated as current")
	}
	if s.Sources.PaymentSummary(s.Scope, s.StartedAt.Add(6*time.Minute)).DataComplete {
		t.Fatal("stale payment snapshot complete")
	}
}

func TestUndocumentedPaymentAliasesAreNotNormalized(t *testing.T) {
	for _, status := range []string{"success", "succeeded", "processing", "declined"} {
		t.Run(status, func(t *testing.T) {
			scenario := fixture(t, "queue-delay")
			for n := range scenario.Sources.Payments[0].Payments {
				scenario.Sources.Payments[0].Payments[n].Status = status
			}
			summary := scenario.Sources.PaymentSummary(scenario.Scope, scenario.StartedAt)
			if summary.DataComplete || summary.Unknown != summary.Total || summary.Completed != 0 || summary.Pending != 0 || summary.Failed != 0 {
				t.Fatalf("unrecognized status was interpreted as a known state: %+v", summary)
			}
		})
	}
}
