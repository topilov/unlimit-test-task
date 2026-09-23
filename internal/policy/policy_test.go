package policy

import (
	"testing"

	"apm-investigator/internal/incident"
)

func TestSeverity(t *testing.T) {
	for _, tc := range []struct {
		i    incident.Impact
		want string
	}{{incident.Impact{}, "unknown"}, {incident.Impact{CohortComplete: true}, "low"}, {incident.Impact{CustomerVisible: true, AffectedPayments: 1}, "medium"}, {incident.Impact{CriticalMerchant: true, AffectedPayments: 100}, "high"}, {incident.Impact{CriticalMerchant: true, AffectedPayments: 99}, "unknown"}} {
		if got := Severity(tc.i); got != tc.want {
			t.Fatalf("%+v: %s", tc.i, got)
		}
	}
}
func TestRecoveryRequiresOriginalCohortAndNewTraffic(t *testing.T) {
	for _, tc := range []struct {
		name                string
		complete            bool
		acked               map[string]bool
		checked, success    int
		original, newStatus incident.CheckStatus
		allowed             bool
	}{
		{"partial backlog", true, map[string]bool{"p1": true}, 20, 20, incident.Fail, incident.Pass, false},
		{"recovered", true, map[string]bool{"p1": true, "p2": true}, 20, 20, incident.Pass, incident.Pass, true},
		{"unavailable even with acknowledgements", false, map[string]bool{"p1": true, "p2": true}, 20, 20, incident.Unknown, incident.Unknown, false},
		{"no new traffic", true, map[string]bool{"p1": true, "p2": true}, 0, 0, incident.Pass, incident.Unknown, false},
		{"new traffic failing", true, map[string]bool{"p1": true, "p2": true}, 20, 19, incident.Pass, incident.Fail, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := Recovery(RecoveryInput{Original: []string{"p1", "p2"}, CohortComplete: true, Acknowledged: tc.acked, CallbacksComplete: tc.complete, NewChecked: tc.checked, NewSuccessful: tc.success})
			if r.OriginalCohortRecovered != tc.original || r.NewTrafficHealthy != tc.newStatus || r.ClosureAllowed != tc.allowed {
				t.Fatalf("%+v", r)
			}
		})
	}
	if Recovery(RecoveryInput{CohortComplete: true, CallbacksComplete: true, NewChecked: 1, NewSuccessful: 1}).ClosureAllowed {
		t.Fatal("empty original cohort must not silently pass")
	}
}

func TestQueuePolicyBoundary(t *testing.T) {
	if QueueDegraded(100, 300, "healthy") {
		t.Fatal("boundary should be healthy")
	}
	if !QueueDegraded(101, 300, "healthy") || !QueueDegraded(100, 301, "healthy") || !QueueDegraded(0, 0, "degraded") {
		t.Fatal("degradation missed")
	}
}
