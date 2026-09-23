package policy

import "apm-investigator/internal/incident"

func Severity(i incident.Impact) string {
	if i.CriticalMerchant && i.AffectedPayments >= CriticalMerchantAffectedThreshold {
		return "high"
	}
	if i.CustomerVisible && i.AffectedPayments > 0 {
		return "medium"
	}
	if !i.CohortComplete {
		return "unknown"
	}
	return "low"
}
func Owner(owner incident.Owner) incident.Owner {
	switch owner {
	case incident.OwnerTechOps, incident.OwnerCallbacks, incident.OwnerPayments, incident.OwnerMerchant, incident.OwnerProvider:
		return owner
	}
	return incident.OwnerTechOps
}

type RecoveryInput struct {
	Original                  []string
	CohortComplete            bool
	Acknowledged              map[string]bool
	CallbacksComplete         bool
	NewChecked, NewSuccessful int
}

func Recovery(input RecoveryInput) incident.RecoveryResult {
	r := incident.RecoveryResult{NewTrafficHealthy: incident.Unknown, OriginalCohortRecovered: incident.Unknown, UnresolvedOriginalItems: len(input.Original)}
	for _, id := range input.Original {
		if input.Acknowledged[id] {
			r.UnresolvedOriginalItems--
		}
	}
	if input.CohortComplete && input.CallbacksComplete && len(input.Original) > 0 {
		r.OriginalCohortRecovered = incident.Fail
		if r.UnresolvedOriginalItems == 0 {
			r.OriginalCohortRecovered = incident.Pass
		}
	}
	if input.CallbacksComplete && input.NewChecked > 0 && input.NewSuccessful >= 0 && input.NewSuccessful <= input.NewChecked {
		r.NewTrafficHealthy = incident.Fail
		if input.NewSuccessful == input.NewChecked {
			r.NewTrafficHealthy = incident.Pass
		}
	}
	r.ClosureAllowed = r.NewTrafficHealthy == incident.Pass && r.OriginalCohortRecovered == incident.Pass
	return r
}

const (
	CriticalMerchantAffectedThreshold = 100
	QueueDepthThreshold               = 100
	QueueAgeThresholdSeconds          = 300
)

func QueueDegraded(depth, oldestSeconds int, worker string) bool {
	return depth > QueueDepthThreshold || oldestSeconds > QueueAgeThresholdSeconds || worker != "healthy"
}
