package incident

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrUnknownTool        = errors.New("unknown tool")
	ErrToolScopeViolation = errors.New("tool scope violation")
	ErrStaleRevision      = errors.New("stale incident revision")
	ErrModelUnavailable   = errors.New("model unavailable")
	ErrModelInvalidOutput = errors.New("invalid model output")
	ErrStepLimitExceeded  = errors.New("investigation step limit exceeded")
	ErrProposalStale      = errors.New("proposal is stale")
)

type Status string

const (
	Investigating      Status = "investigating"
	AwaitingReview     Status = "awaiting_review"
	MonitoringRecovery Status = "monitoring_recovery"
	ClosureProposed    Status = "closure_proposed"
	Closed             Status = "closed"
	ManualTriage       Status = "manual_triage"
)

type Scope struct {
	MerchantID  string    `json:"merchant_id" yaml:"merchant_id"`
	Method      string    `json:"method" yaml:"method"`
	Environment string    `json:"environment" yaml:"environment"`
	WindowStart time.Time `json:"window_start" yaml:"window_start"`
	WindowEnd   time.Time `json:"window_end" yaml:"window_end"`
}

func (s Scope) SameIdentity(other Scope) bool {
	return s.MerchantID == other.MerchantID && s.Method == other.Method && s.Environment == other.Environment
}

type Impact struct {
	CohortComplete   bool      `json:"cohort_complete"`
	UniquePayments   int       `json:"unique_payments"`
	AffectedPayments int       `json:"affected_payments"`
	CustomerVisible  bool      `json:"customer_visible"`
	CriticalMerchant bool      `json:"critical_merchant"`
	WindowStart      time.Time `json:"window_start"`
	WindowEnd        time.Time `json:"window_end"`
}
type Incident struct {
	ID        uuid.UUID `json:"id"`
	Status    Status    `json:"status"`
	Revision  int64     `json:"revision"`
	Scope     Scope     `json:"scope"`
	Scenario  string    `json:"scenario"`
	Clock     time.Time `json:"clock"`
	Cursor    int       `json:"cursor"`
	Impact    Impact    `json:"impact"`
	Cohort    []string  `json:"original_cohort"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Event struct {
	ID          uuid.UUID       `json:"id"`
	IncidentID  uuid.UUID       `json:"incident_id"`
	Source      string          `json:"source"`
	ExternalID  string          `json:"external_id"`
	Kind        string          `json:"kind"`
	OccurredAt  time.Time       `json:"occurred_at"`
	AvailableAt time.Time       `json:"available_at"`
	Payload     json.RawMessage `json:"payload"`
}
type Evidence struct {
	EventID     uuid.UUID       `json:"event_id,omitempty"`
	ID          string          `json:"id"`
	IncidentID  uuid.UUID       `json:"incident_id"`
	RunID       uuid.UUID       `json:"run_id"`
	Source      string          `json:"source"`
	Kind        string          `json:"kind"`
	ObservedAt  time.Time       `json:"observed_at"`
	WindowStart time.Time       `json:"window_start"`
	WindowEnd   time.Time       `json:"window_end"`
	Available   bool            `json:"available"`
	Summary     string          `json:"summary"`
	Data        json.RawMessage `json:"data"`
	Claims      []string        `json:"claims"`
}
type Hypothesis struct {
	ID                    string           `json:"id"`
	Statement             string           `json:"statement"`
	Status                HypothesisStatus `json:"status"`
	SupportingEvidence    []string         `json:"supporting_evidence"`
	ContradictingEvidence []string         `json:"contradicting_evidence"`
	MissingEvidence       []string         `json:"missing_evidence"`
}
type OpenQuestion struct {
	ID           string `json:"id"`
	Question     string `json:"question"`
	WhyItMatters string `json:"why_it_matters"`
}
type Result struct {
	Summary           string          `json:"summary"`
	FailureDomain     string          `json:"failure_domain"`
	RootCauseStatus   RootCauseStatus `json:"root_cause_status"`
	Hypotheses        []Hypothesis    `json:"hypotheses"`
	OpenQuestions     []OpenQuestion  `json:"open_questions"`
	EvidenceIDs       []string        `json:"evidence_ids"`
	ExecutedTools     []string        `json:"executed_tools"`
	RecommendedOwner  Owner           `json:"recommended_owner"`
	RecommendedAction Action          `json:"recommended_action"`
	Claims            []string        `json:"claims"`
}
type ToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}
type Turn struct {
	Kind          TurnKind       `json:"kind"`
	ToolCall      *ToolCall      `json:"tool_call,omitempty"`
	Final         *Result        `json:"final,omitempty"`
	Hypotheses    []Hypothesis   `json:"hypotheses"`
	OpenQuestions []OpenQuestion `json:"open_questions"`
}
type ToolRun struct {
	ID          uuid.UUID       `json:"id"`
	RunID       uuid.UUID       `json:"run_id"`
	Step        int             `json:"step"`
	Name        string          `json:"name"`
	Arguments   json.RawMessage `json:"arguments"`
	Status      ToolStatus      `json:"status"`
	EvidenceRef string          `json:"evidence_ref"`
	Error       string          `json:"error,omitempty"`
	StartedAt   time.Time       `json:"started_at"`
	FinishedAt  time.Time       `json:"finished_at"`
}
type Metadata struct {
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort"`
	InputTokens     int64  `json:"input_tokens"`
	OutputTokens    int64  `json:"output_tokens"`
	CachedTokens    int64  `json:"cached_tokens"`
	DurationMS      int64  `json:"duration_ms"`
	ResponseID      string `json:"response_id"`
	Attempts        int    `json:"attempts"`
}
type ModelTurn struct {
	Step     int      `json:"step"`
	Turn     Turn     `json:"turn"`
	Metadata Metadata `json:"metadata"`
	Error    string   `json:"error,omitempty"`
}
type RunContext struct {
	AsOf          time.Time      `json:"as_of"`
	Scope         Scope          `json:"scope"`
	Impact        Impact         `json:"impact"`
	Cohort        []string       `json:"original_cohort"`
	PreviousRunID uuid.UUID      `json:"previous_run_id"`
	Evidence      []Evidence     `json:"evidence"`
	Hypotheses    []Hypothesis   `json:"hypotheses"`
	OpenQuestions []OpenQuestion `json:"open_questions"`
	ToolHistory   []ToolRun      `json:"tool_history"`
}

type Run struct {
	Context      RunContext   `json:"context"`
	ID           uuid.UUID    `json:"id"`
	IncidentID   uuid.UUID    `json:"incident_id"`
	BaseRevision int64        `json:"base_revision"`
	Mode         string       `json:"mode"`
	Status       RunStatus    `json:"status"`
	Result       *Result      `json:"result,omitempty"`
	Error        string       `json:"error,omitempty"`
	Turns        []ModelTurn  `json:"turns"`
	Memory       []MemoryRule `json:"memory"`
}
type Proposal struct {
	ID               uuid.UUID      `json:"id"`
	IncidentID       uuid.UUID      `json:"incident_id"`
	IncidentRevision int64          `json:"incident_revision"`
	Type             ProposalType   `json:"type"`
	Owner            Owner          `json:"owner"`
	Title            string         `json:"title"`
	Body             string         `json:"body"`
	Status           ProposalStatus `json:"status"`
	CreatedAt        time.Time      `json:"created_at"`
	DecidedAt        *time.Time     `json:"decided_at,omitempty"`
	DecisionNote     string         `json:"decision_note,omitempty"`
}
type CheckStatus string

const (
	Pass    CheckStatus = "pass"
	Fail    CheckStatus = "fail"
	Unknown CheckStatus = "unknown"
)

type RecoveryResult struct {
	NewTrafficHealthy       CheckStatus `json:"new_traffic_healthy"`
	OriginalCohortRecovered CheckStatus `json:"original_cohort_recovered"`
	UnresolvedOriginalItems int         `json:"unresolved_original_items"`
	ClosureAllowed          bool        `json:"closure_allowed"`
}
type TimelineEntry struct {
	At   time.Time       `json:"at"`
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}
