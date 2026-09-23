package tools

import (
	"context"
	"time"

	"apm-investigator/internal/incident"
)

type Registry struct {
	Sources *Sources
	Now     time.Time
	Cohort  []string
}

func (r *Registry) Execute(ctx context.Context, scope incident.Scope, call incident.ToolCall) (incident.Evidence, error) {
	if err := ctx.Err(); err != nil {
		return incident.Evidence{}, err
	}
	e := incident.Evidence{
		Source:      "",
		Kind:        call.Name,
		ObservedAt:  r.Now,
		WindowStart: scope.WindowStart,
		WindowEnd:   scope.WindowEnd,
		Available:   true,
		Claims:      []string{},
	}
	var err error
	switch call.Name {
	case incident.ToolPayments:
		e, err = r.payments(scope, call, e)
	case incident.ToolCallbacks:
		e, err = r.callbacks(scope, call, e)
	case incident.ToolQueue:
		e, err = r.queue(scope, call, e)
	case incident.ToolProvider:
		e, err = r.provider(scope, call, e)
	default:
		return e, incident.ErrUnknownTool
	}
	if err != nil {
		return e, err
	}

	var source []Snapshot
	switch e.Source {
	case "payments":
		source = r.Sources.Payments
	case "callbacks":
		source = r.Sources.Callbacks
	case "queue":
		source = r.Sources.Queue
	case "provider":
		source = r.Sources.Provider
	}
	snapshot, _ := Latest(source, scope, r.Now)
	if !snapshot.ObservedAt.IsZero() {
		e.ObservedAt = snapshot.ObservedAt
	}
	if err := ctx.Err(); err != nil {
		return incident.Evidence{}, err
	}
	return e, nil
}
