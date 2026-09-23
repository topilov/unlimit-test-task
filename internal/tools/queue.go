package tools

import (
	"encoding/json"
	"fmt"

	"apm-investigator/internal/incident"
	"apm-investigator/internal/jsonutil"
	"apm-investigator/internal/policy"
)

type QueueHealth struct {
	Depth        int    `json:"queue_depth"`
	OldestJobAge int    `json:"oldest_job_age_seconds"`
	WorkerStatus string `json:"worker_status"`
	DataComplete bool   `json:"data_complete"`
	Degraded     bool   `json:"degraded"`
}

func (r *Registry) queue(scope incident.Scope, call incident.ToolCall, e incident.Evidence) (incident.Evidence, error) {
	var args struct{}
	if err := jsonutil.DecodeObject(call.Arguments, &args); err != nil {
		return e, err
	}
	s, ok := Latest(r.Sources.Queue, scope, r.Now)
	complete := ok && fresh(s, r.Now)
	degraded := policy.QueueDegraded(s.QueueDepth, s.OldestJobAge, s.WorkerStatus)
	e.Source = "queue"
	e.Available = complete
	data, err := json.Marshal(QueueHealth{
		Depth:        s.QueueDepth,
		OldestJobAge: s.OldestJobAge,
		WorkerStatus: s.WorkerStatus,
		DataComplete: complete,
		Degraded:     degraded,
	})
	if err != nil {
		return e, err
	}
	e.Data = data
	e.Summary = fmt.Sprintf("Queue depth=%d, oldest=%ds, worker=%s; complete=%t", s.QueueDepth, s.OldestJobAge, s.WorkerStatus, complete)
	if complete {
		if degraded {
			e.Claims = []string{incident.ClaimQueueDegraded}
		} else {
			e.Claims = []string{incident.ClaimQueueHealthy}
		}
	}
	return e, nil
}
