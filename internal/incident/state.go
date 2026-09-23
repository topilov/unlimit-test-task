package incident

import "fmt"

func Transition(from, to Status) error {
	allowed := false
	switch from {
	case Investigating:
		allowed = to == AwaitingReview || to == ManualTriage
	case AwaitingReview:
		allowed = to == Investigating || to == MonitoringRecovery
	case MonitoringRecovery:
		allowed = to == Investigating || to == ClosureProposed
	case ClosureProposed:
		allowed = to == MonitoringRecovery || to == Closed || to == Investigating
	case ManualTriage:
		allowed = to == Investigating
	}
	if !allowed {
		return fmt.Errorf("%w: transition %s -> %s", ErrInvalidInput, from, to)
	}
	return nil
}
