package investigation

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"

	"apm-investigator/internal/incident"
	"apm-investigator/internal/tools"
)

type baseFacts struct {
	Impact            incident.Impact      `json:"impact"`
	Cohort            []string             `json:"original_cohort"`
	Payments          tools.PaymentSummary `json:"payment_summary"`
	CallbacksComplete bool                 `json:"callback_confirmations_complete"`
}

func baseEvidence(i incident.Incident, sources *tools.Sources) (incident.Evidence, error) {
	payments := sources.PaymentSummary(i.Scope, i.Clock)
	callbacks := sources.CallbackSummary(i.Scope, i.Clock, i.Cohort)
	data, err := json.Marshal(baseFacts{
		Impact:            i.Impact,
		Cohort:            i.Cohort,
		Payments:          payments,
		CallbacksComplete: callbacks.DataComplete,
	})
	if err != nil {
		return incident.Evidence{}, err
	}
	return incident.Evidence{
		Source:      "payments",
		Kind:        incident.BaseFacts,
		ObservedAt:  i.Clock,
		WindowStart: i.Scope.WindowStart,
		WindowEnd:   i.Scope.WindowEnd,
		Available:   payments.DataComplete,
		Summary:     fmt.Sprintf("%d unique payments; %d completed payments missing confirmed callbacks (potential impact)", i.Impact.UniquePayments, i.Impact.AffectedPayments),
		Data:        data,
	}, nil
}

func hasEvent(evidence []incident.Evidence, id uuid.UUID) bool {
	for _, e := range evidence {
		if e.EventID == id {
			return true
		}
	}
	return false
}
