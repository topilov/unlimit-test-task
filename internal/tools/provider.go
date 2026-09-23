package tools

import (
	"encoding/json"
	"fmt"
	"time"

	"apm-investigator/internal/incident"
	"apm-investigator/internal/jsonutil"
)

type ProviderStatus struct {
	Status          string    `json:"status"`
	Component       string    `json:"component"`
	PublishedAt     time.Time `json:"published_at"`
	FreshForWindow  bool      `json:"fresh_for_incident_window"`
	SourceAvailable bool      `json:"source_available"`
	UntrustedText   string    `json:"untrusted_text"`
}

func (r *Registry) provider(scope incident.Scope, call incident.ToolCall, e incident.Evidence) (incident.Evidence, error) {
	var args struct{}
	if err := jsonutil.DecodeObject(call.Arguments, &args); err != nil {
		return e, err
	}
	s, ok := Latest(r.Sources.Provider, scope, r.Now)
	freshForWindow := ok && !s.PublishedAt.Before(scope.WindowStart) && !s.PublishedAt.After(r.Now)
	e.Source = "provider"
	e.Available = ok
	data, err := json.Marshal(ProviderStatus{
		Status:          s.ProviderStatus,
		Component:       scope.Method,
		PublishedAt:     s.PublishedAt,
		FreshForWindow:  freshForWindow,
		SourceAvailable: ok,
		UntrustedText:   s.Text,
	})
	if err != nil {
		return e, err
	}
	e.Data = data
	e.Summary = fmt.Sprintf("Provider status=%s; fresh for incident=%t", s.ProviderStatus, freshForWindow)
	if ok && !freshForWindow {
		e.Claims = []string{incident.ClaimProviderStale}
	}
	return e, nil
}
