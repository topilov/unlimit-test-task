package policy

import (
	"slices"
	"time"

	"apm-investigator/internal/incident"
)

// CanRoute requires current diagnostic support for a targeted owner, independently
// of whether the underlying root cause is known. Historical evidence stays usable
// in the investigation narrative, but cannot override a newer observation.
func CanRoute(r *incident.Result, evidence []incident.Evidence, asOf time.Time) bool {
	if r == nil {
		return false
	}
	var kind, claim string
	switch Owner(r.RecommendedOwner) {
	case incident.OwnerTechOps:
		return true
	case incident.OwnerCallbacks:
		kind, claim = incident.ToolQueue, incident.ClaimQueueDegraded
	case incident.OwnerMerchant:
		kind, claim = incident.ToolCallbacks, incident.ClaimReceiver503
	default:
		// The prototype has no positive localization rule for these owners.
		return false
	}
	var latest time.Time
	for _, e := range evidence {
		if e.Kind == kind && !e.ObservedAt.After(asOf) && e.ObservedAt.After(latest) {
			latest = e.ObservedAt
		}
	}
	if asOf.IsZero() || latest.IsZero() || asOf.Sub(latest) > 5*time.Minute {
		return false
	}
	supported := false
	for _, e := range evidence {
		if e.Kind != kind || !e.ObservedAt.Equal(latest) {
			continue
		}
		if !e.Available || !slices.Contains(e.Claims, claim) {
			return false
		}
		if !slices.Contains(r.EvidenceIDs, e.ID) {
			continue
		}
		for _, h := range r.Hypotheses {
			if h.Status == incident.HypothesisSupported && slices.Contains(h.SupportingEvidence, e.ID) {
				supported = true
			}
		}
	}
	return supported
}
