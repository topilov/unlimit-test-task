package incident

import "testing"

func TestTransitions(t *testing.T) {
	allowed := map[Status][]Status{Investigating: {AwaitingReview, ManualTriage}, AwaitingReview: {Investigating, MonitoringRecovery}, MonitoringRecovery: {Investigating, ClosureProposed}, ClosureProposed: {MonitoringRecovery, Closed, Investigating}, ManualTriage: {Investigating}}
	all := []Status{Investigating, AwaitingReview, MonitoringRecovery, ClosureProposed, Closed, ManualTriage}
	for _, from := range all {
		for _, to := range all {
			want := false
			for _, v := range allowed[from] {
				want = want || v == to
			}
			if (Transition(from, to) == nil) != want {
				t.Errorf("%s -> %s", from, to)
			}
		}
	}
}
