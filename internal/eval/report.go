package eval

import (
	"time"

	"apm-investigator/internal/incident"
	"github.com/google/uuid"
)

type AssessmentExpected struct {
	MustUseTools    []string                 `yaml:"must_use_tools"`
	ShouldUseTools  []string                 `yaml:"should_use_tools"`
	MustDiscover    []string                 `yaml:"must_discover"`
	MustNotClaim    []string                 `yaml:"must_not_claim"`
	Action          incident.Action          `yaml:"action"`
	InitialStatus   incident.Status          `yaml:"initial_status"`
	Owner           incident.Owner           `yaml:"owner"`
	RootCauseStatus incident.RootCauseStatus `yaml:"root_cause_status"`
}
type Expected struct {
	AssessmentExpected    `yaml:",inline"`
	AdvanceBeforeApproval bool                      `yaml:"advance_before_approval"`
	Investigations        []InvestigationCheckpoint `yaml:"investigations"`
	Recovery              []Checkpoint              `yaml:"recovery"`
}
type InvestigationCheckpoint struct {
	At                   time.Time `yaml:"at"`
	AssessmentExpected   `yaml:",inline"`
	MinRevisedHypotheses int `yaml:"min_revised_hypotheses"`
}
type Report struct {
	MemoryIDs             []uuid.UUID     `json:"memory_ids"`
	Scenario              string          `json:"scenario"`
	IncidentID            uuid.UUID       `json:"incident_id"`
	Passed                bool            `json:"passed"`
	Failures              []string        `json:"failures"`
	Warnings              []string        `json:"warnings"`
	RequiredTools         map[string]bool `json:"required_tools"`
	ForbiddenClaims       []string        `json:"forbidden_claims"`
	EvaluationScope       string          `json:"evaluation_scope"`
	ProseEvaluated        bool            `json:"prose_evaluated"`
	PrematureClosureCount int             `json:"premature_closure_count"`
	ToolCalls             int             `json:"tool_call_count"`
	ModelCalls            int             `json:"model_call_count"`
	APICalls              int             `json:"api_attempt_count"`
	LatencyMS             int64           `json:"latency_ms"`
	InputTokens           int64           `json:"input_tokens"`
	OutputTokens          int64           `json:"output_tokens"`
}

type Checkpoint struct {
	At             time.Time `yaml:"at"`
	ClosureAllowed bool      `yaml:"closure_allowed"`
}
