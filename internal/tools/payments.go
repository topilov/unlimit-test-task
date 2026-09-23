package tools

import (
	"encoding/json"
	"fmt"

	"apm-investigator/internal/incident"
	"apm-investigator/internal/jsonutil"
)

func (r *Registry) payments(scope incident.Scope, call incident.ToolCall, e incident.Evidence) (incident.Evidence, error) {
	var args struct {
		Cohort string `json:"cohort"`
	}
	if err := jsonutil.DecodeObject(call.Arguments, &args); err != nil {
		return e, err
	}
	if args.Cohort != "incident" {
		return e, incident.ErrToolScopeViolation
	}
	p := r.Sources.PaymentSummary(scope, r.Now)
	e.Source = "payments"
	data, err := json.Marshal(p)
	if err != nil {
		return e, err
	}
	e.Data = data
	e.Available = p.DataComplete
	e.Summary = fmt.Sprintf("%d unique payments: %d completed, %d pending, %d failed; complete=%t", p.Total, p.Completed, p.Pending, p.Failed, p.DataComplete)
	return e, nil
}
