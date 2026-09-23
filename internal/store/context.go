package store

import (
	"context"

	db "apm-investigator/internal/db/generated"
	"apm-investigator/internal/incident"
)

func previousContext(ctx context.Context, q *db.Queries, i incident.Incident, mode string) (incident.RunContext, error) {
	out := incident.RunContext{AsOf: i.Clock, Scope: i.Scope, Impact: i.Impact, Cohort: i.Cohort, Evidence: []incident.Evidence{}, Hypotheses: []incident.Hypothesis{}, OpenQuestions: []incident.OpenQuestion{}, ToolHistory: []incident.ToolRun{}}
	rows, err := q.ListRuns(ctx, i.ID)
	if err != nil {
		return out, err
	}
	for n := len(rows) - 1; n >= 0; n-- {
		row := rows[n]
		if row.Mode != mode || row.BaseRevision >= i.Revision || (row.Status != string(incident.RunCompleted) && row.Status != string(incident.RunIncomplete)) {
			continue
		}
		prior, err := decodeRun(row)
		if err != nil {
			return out, err
		}
		if prior.Result == nil || prior.Context.AsOf.After(i.Clock) {
			continue
		}
		evidence, err := runEvidence(ctx, q, prior.ID)
		if err != nil {
			return out, err
		}
		for _, e := range append(prior.Context.Evidence, evidence...) {
			if e.IncidentID == i.ID && !e.ObservedAt.After(i.Clock) {
				out.Evidence = append(out.Evidence, e)
			}
		}
		tools, err := toolRuns(ctx, q, prior.ID)
		if err != nil {
			return out, err
		}
		out.PreviousRunID = prior.ID
		out.Hypotheses = prior.Result.Hypotheses
		out.OpenQuestions = prior.Result.OpenQuestions
		out.ToolHistory = append(prior.Context.ToolHistory, tools...)
		break
	}
	return out, nil
}
