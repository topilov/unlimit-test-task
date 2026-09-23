package tools

import (
	"testing"
	"time"

	"apm-investigator/internal/incident"
	"apm-investigator/internal/policy"
)

func reviewTime(minute int) time.Time {
	return time.Date(2026, 9, 22, 10, minute, 0, 0, time.UTC)
}

func TestLateHistoricalSnapshotMustNotPermitClosure(t *testing.T) {
	scope := incident.Scope{MerchantID: "m", Method: "apm", Environment: "sandbox", WindowStart: reviewTime(0), WindowEnd: reviewTime(5)}
	bad := Snapshot{Scope: scope, ObservedAt: reviewTime(7), AvailableAt: reviewTime(7), Available: true, Complete: true, Attempts: []Attempt{{PaymentID: "p1", ID: "a1", HTTPStatus: 200, At: reviewTime(6)}}, NewTrafficChecked: 10, NewTrafficSuccessful: 0}
	oldGood := bad
	oldGood.ObservedAt, oldGood.AvailableAt, oldGood.NewTrafficSuccessful = reviewTime(6), reviewTime(8), 10
	check := func(sources Sources, at time.Time) incident.RecoveryResult {
		c := sources.CallbackSummary(scope, at, []string{"p1"})
		acked := map[string]bool{}
		for _, id := range c.Acknowledged {
			acked[id] = true
		}
		return policy.Recovery(policy.RecoveryInput{Original: []string{"p1"}, CohortComplete: true, Acknowledged: acked, CallbacksComplete: c.DataComplete, NewChecked: c.NewTrafficChecked, NewSuccessful: c.NewTrafficSuccessful})
	}
	if check(Sources{Callbacks: []Snapshot{bad}}, reviewTime(8)).ClosureAllowed {
		t.Fatal("positive control: unhealthy snapshot permits closure")
	}
	if check(Sources{Callbacks: []Snapshot{bad, oldGood}}, reviewTime(7)).ClosureAllowed {
		t.Fatal("future evidence leaked")
	}
	newGood := oldGood
	newGood.ObservedAt, newGood.AvailableAt = reviewTime(9), reviewTime(9)
	if !check(Sources{Callbacks: []Snapshot{bad, oldGood, newGood}}, reviewTime(9)).ClosureAllowed {
		t.Fatal("new healthy observation does not permit closure")
	}
	latest, _ := Latest([]Snapshot{bad, oldGood}, scope, reviewTime(8))
	got := check(Sources{Callbacks: []Snapshot{bad, oldGood}}, reviewTime(8))
	t.Logf("selected observed_at=%s available_at=%s new_traffic=%s closure_allowed=%t", latest.ObservedAt.Format("15:04"), latest.AvailableAt.Format("15:04"), got.NewTrafficHealthy, got.ClosureAllowed)
	if got.ClosureAllowed {
		t.Fatal("late healthy observation at 10:06 replaced known unhealthy observation at 10:07")
	}
}

func TestUnknownImpactMustNotDisplayLow(t *testing.T) {
	if got := policy.Severity(incident.Impact{CohortComplete: true}); got != "low" {
		t.Fatal("known empty impact control", got)
	}
	if got := policy.Severity(incident.Impact{CohortComplete: false, CriticalMerchant: true, AffectedPayments: 100}); got != "high" {
		t.Fatal("known high impact control", got)
	}
	got := policy.Severity(incident.Impact{CohortComplete: false})
	t.Logf("unknown zero impact severity=%s", got)
	if got == "low" {
		t.Fatal("unknown impact displayed as low")
	}
}

func TestPaymentMembershipAndCurrentStatus(t *testing.T) {
	scope := incident.Scope{MerchantID: "m", Method: "apm", Environment: "sandbox", WindowStart: reviewTime(0), WindowEnd: reviewTime(5)}
	sources := Sources{Payments: []Snapshot{{Scope: scope, ObservedAt: reviewTime(8), AvailableAt: reviewTime(8), Available: true, Complete: true, Payments: []Payment{{ID: "p1", Status: "pending", CreatedAt: reviewTime(1), UpdatedAt: reviewTime(1)}, {ID: "p1", Status: "completed", CreatedAt: reviewTime(1), UpdatedAt: reviewTime(7)}}}}}
	got := sources.PaymentSummary(scope, reviewTime(8))
	t.Logf("window_end=10:05 as_of=10:08 total=%d pending=%d completed=%d complete=%t", got.Total, got.Pending, got.Completed, got.DataComplete)
	if got.Total != 1 || got.Pending != 0 || got.Completed != 1 || !got.DataComplete {
		t.Fatal("current status of original payment was lost", got)
	}
	scope.WindowEnd = reviewTime(8)
	current := sources.PaymentSummary(scope, reviewTime(8))
	if current.Completed != 1 || current.Pending != 0 {
		t.Fatal("extended window control", current)
	}
}

func TestLatestCorrectionUnavailableAndAmbiguity(t *testing.T) {
	scope := incident.Scope{MerchantID: "m"}
	first := Snapshot{Scope: scope, ObservedAt: reviewTime(5), AvailableAt: reviewTime(5), Available: true, Complete: true}
	correction := first
	correction.AvailableAt = reviewTime(6)
	correction.QueueDepth = 12
	got, ok := Latest([]Snapshot{correction, first}, scope, reviewTime(6))
	if !ok || got.QueueDepth != 12 || !fresh(got, reviewTime(6)) {
		t.Fatal("same observation correction lost", got)
	}
	missing := first
	missing.ObservedAt = reviewTime(7)
	missing.AvailableAt = reviewTime(7)
	missing.Available = false
	got, ok = Latest([]Snapshot{first, missing}, scope, reviewTime(7))
	if ok || got.ObservedAt != missing.ObservedAt {
		t.Fatal("fell back past unavailable source")
	}
	conflict := first
	conflict.QueueDepth = 200
	for _, list := range [][]Snapshot{{first, conflict}, {conflict, first}} {
		got, _ = Latest(list, scope, reviewTime(5))
		if fresh(got, reviewTime(5)) {
			t.Fatal("ambiguous observation became complete")
		}
	}
	for _, field := range []string{"observed", "available"} {
		missingTime := first
		if field == "observed" {
			missingTime.ObservedAt = time.Time{}
		} else {
			missingTime.AvailableAt = time.Time{}
		}
		if _, ok := Latest([]Snapshot{missingTime}, scope, reviewTime(5)); ok {
			t.Fatal("undated observation used")
		}
	}
}

func TestPaymentMembershipAndIncompleteDates(t *testing.T) {
	scope := incident.Scope{MerchantID: "m", WindowStart: reviewTime(0), WindowEnd: reviewTime(5)}
	snapshot := Snapshot{Scope: scope, ObservedAt: reviewTime(8), AvailableAt: reviewTime(8), Available: true, Complete: true, Payments: []Payment{
		{ID: "inside", CreatedAt: reviewTime(1), UpdatedAt: reviewTime(1), Status: "pending"},
		{ID: "inside", CreatedAt: reviewTime(1), UpdatedAt: reviewTime(7), Status: "completed"},
		{ID: "future", CreatedAt: reviewTime(6), UpdatedAt: reviewTime(7), Status: "completed"},
		{ID: "before", CreatedAt: reviewTime(0).Add(-time.Minute), UpdatedAt: reviewTime(1), Status: "completed"},
	}}
	sources := Sources{Payments: []Snapshot{snapshot}}
	got := sources.PaymentSummary(scope, reviewTime(8))
	if !got.DataComplete || got.Total != 1 || got.Completed != 1 || got.CompletedIDs[0] != "inside" {
		t.Fatal(got)
	}
	sources.Payments[0].Payments = append(sources.Payments[0].Payments, Payment{ID: "undated", Status: "completed", UpdatedAt: reviewTime(7)})
	if sources.PaymentSummary(scope, reviewTime(8)).DataComplete {
		t.Fatal("missing creation time silently inferred")
	}
}

func TestMalformedCallbacksCannotPermitClosure(t *testing.T) {
	scope := incident.Scope{MerchantID: "m", WindowStart: reviewTime(0), WindowEnd: reviewTime(5)}
	valid := Attempt{PaymentID: "p1", ID: "a1", HTTPStatus: 200, At: reviewTime(6)}
	for _, tc := range []struct {
		name  string
		alter func(*Attempt)
	}{
		{"late valid recovery", func(_ *Attempt) {}},
		{"zero timestamp", func(a *Attempt) { a.At = time.Time{} }},
		{"future timestamp", func(a *Attempt) { a.At = reviewTime(9) }},
		{"missing ID", func(a *Attempt) { a.ID = " " }},
		{"missing payment", func(a *Attempt) { a.PaymentID = "" }},
		{"missing response", func(a *Attempt) { a.HTTPStatus = 0 }},
		{"invalid response", func(a *Attempt) { a.HTTPStatus = 600 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := valid
			tc.alter(&a)
			s := Sources{Callbacks: []Snapshot{{Scope: scope, ObservedAt: reviewTime(8), AvailableAt: reviewTime(8), Available: true, Complete: true, Attempts: []Attempt{a}, NewTrafficChecked: 10, NewTrafficSuccessful: 10}}}
			got := s.CallbackSummary(scope, reviewTime(8), []string{"p1"})
			acked := map[string]bool{}
			for _, id := range got.Acknowledged {
				acked[id] = true
			}
			recovery := policy.Recovery(policy.RecoveryInput{Original: []string{"p1"}, CohortComplete: true, Acknowledged: acked, CallbacksComplete: got.DataComplete, NewChecked: got.NewTrafficChecked, NewSuccessful: got.NewTrafficSuccessful})
			if tc.name == "late valid recovery" {
				if !recovery.ClosureAllowed {
					t.Fatal("valid late acknowledgement rejected", got)
				}
				return
			}
			if got.DataComplete || len(got.Acknowledged) > 0 || recovery.ClosureAllowed {
				t.Fatal("malformed callback accepted", got, recovery)
			}
			s.Callbacks[0].Attempts = append(s.Callbacks[0].Attempts, valid)
			if s.CallbackSummary(scope, reviewTime(8), []string{"p1"}).DataComplete {
				t.Fatal("valid record hid malformed record")
			}
		})
	}
}

func TestPaymentConflictsAreOrderIndependent(t *testing.T) {
	scope := incident.Scope{MerchantID: "m", WindowStart: reviewTime(0), WindowEnd: reviewTime(5)}
	completed := Payment{ID: "p1", Status: "completed", CreatedAt: reviewTime(1), UpdatedAt: reviewTime(4)}
	failed := completed
	failed.Status = "failed"
	summary := func(rows []Payment) PaymentSummary {
		s := Sources{Payments: []Snapshot{{Scope: scope, ObservedAt: reviewTime(5), AvailableAt: reviewTime(5), Available: true, Complete: true, Payments: rows}}}
		return s.PaymentSummary(scope, reviewTime(5))
	}
	if got := summary([]Payment{completed, completed}); !got.DataComplete || got.Total != 1 || got.Completed != 1 {
		t.Fatal("identical duplicate lost", got)
	}
	for _, rows := range [][]Payment{{completed, failed}, {failed, completed}} {
		got := summary(rows)
		if got.DataComplete || got.Total != 0 || len(got.CompletedIDs) != 0 {
			t.Fatal("conflict asserted a payment state", got)
		}
	}
	newer := completed
	newer.UpdatedAt = reviewTime(5)
	for _, rows := range [][]Payment{{completed, failed, newer}, {newer, failed, completed}, {failed, newer, completed}} {
		got := summary(rows)
		if !got.DataComplete || got.Completed != 1 {
			t.Fatal("newer unambiguous state lost", got)
		}
	}
}

func TestConflictingCallbackIdentitiesCannotAcknowledgePayment(t *testing.T) {
	scope := incident.Scope{MerchantID: "m"}
	success := Attempt{PaymentID: "p1", ID: "a1", HTTPStatus: 200, At: reviewTime(6)}
	failure := success
	failure.HTTPStatus = 503
	for _, rows := range [][]Attempt{{success, failure}, {failure, success}} {
		s := Sources{Callbacks: []Snapshot{{Scope: scope, ObservedAt: reviewTime(8), AvailableAt: reviewTime(8), Available: true, Complete: true, Attempts: rows}}}
		got := s.CallbackSummary(scope, reviewTime(8), []string{"p1"})
		if got.DataComplete || got.Successful != 0 || len(got.Acknowledged) != 0 || got.WithAttempts != 0 {
			t.Fatal("conflicted identity counted", got)
		}
	}
	s := Sources{Callbacks: []Snapshot{{Scope: scope, ObservedAt: reviewTime(8), AvailableAt: reviewTime(8), Available: true, Complete: true, Attempts: []Attempt{success, success}}}}
	if got := s.CallbackSummary(scope, reviewTime(8), []string{"p1"}); !got.DataComplete || got.Successful != 1 || got.HTTPStatuses["200"] != 1 {
		t.Fatal("identical callback duplicate rejected", got)
	}
}
