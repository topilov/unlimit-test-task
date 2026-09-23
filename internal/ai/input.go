package ai

import (
	"apm-investigator/internal/incident"
	"github.com/google/uuid"
	"time"
)

type Input struct {
	Context            incident.RunContext     `json:"-"`
	AsOf               time.Time               `json:"as_of"`
	PreviousRunID      uuid.UUID               `json:"previous_run_id"`
	Memory             []incident.MemoryRule   `json:"memory"`
	RunID              uuid.UUID               `json:"run_id"`
	IncidentID         uuid.UUID               `json:"incident_id"`
	Revision           int64                   `json:"revision"`
	StepsRemaining     int                     `json:"steps_remaining"`
	Scope              incident.Scope          `json:"scope"`
	Impact             incident.Impact         `json:"impact"`
	Cohort             []string                `json:"original_cohort"`
	Evidence           []incident.Evidence     `json:"evidence"`
	PreviousHypotheses []incident.Hypothesis   `json:"previous_hypotheses"`
	OpenQuestions      []incident.OpenQuestion `json:"open_questions"`
	ToolHistory        []incident.ToolRun      `json:"tool_history"`
}
