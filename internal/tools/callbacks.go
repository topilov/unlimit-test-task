package tools

import (
	"encoding/json"
	"fmt"
	"slices"

	"apm-investigator/internal/incident"
	"apm-investigator/internal/jsonutil"
)

func (r *Registry) callbacks(scope incident.Scope, call incident.ToolCall, e incident.Evidence) (incident.Evidence, error) {
	var args struct {
		IDs []string `json:"payment_ids"`
	}
	if err := jsonutil.DecodeObject(call.Arguments, &args); err != nil {
		return e, err
	}
	if len(args.IDs) == 0 || len(args.IDs) > incident.MaxCallbackIDs {
		return e, incident.ErrInvalidInput
	}
	seen := map[string]bool{}
	ids := []string{}
	for _, id := range args.IDs {
		if !slices.Contains(r.Cohort, id) {
			return e, incident.ErrToolScopeViolation
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	c := r.Sources.CallbackSummary(scope, r.Now, ids)
	e.Source = "callbacks"
	data, err := json.Marshal(c)
	if err != nil {
		return e, err
	}
	e.Data = data
	e.Available = c.DataComplete
	e.Summary = fmt.Sprintf("%d unique payments checked; %d with attempts, %d acknowledged; complete=%t", c.PaymentsChecked, c.WithAttempts, c.Successful, c.DataComplete)
	if c.DataComplete {
		if c.WithAttempts == 0 {
			e.Claims = append(e.Claims, incident.ClaimCallbacksMissing)
		}
		if c.HTTPStatuses["503"] > 0 {
			e.Claims = append(e.Claims, incident.ClaimReceiver503)
		}
	} else {
		e.Claims = append(e.Claims, incident.ClaimCallbacksIncomplete)
	}
	return e, nil
}
