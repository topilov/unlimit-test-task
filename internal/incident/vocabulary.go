package incident

type Action string

const (
	ActionContinue           Action = "continue_investigation"
	ActionEscalate           Action = "escalate_internal"
	ActionRequestInformation Action = "request_external_information"
	ActionMonitor            Action = "monitor_recovery"
	ActionProposeClosure     Action = "propose_closure"
)

type Owner string

const (
	OwnerTechOps   Owner = "tech_ops"
	OwnerCallbacks Owner = "callback_platform"
	OwnerPayments  Owner = "payment_processing"
	OwnerMerchant  Owner = "merchant_integration"
	OwnerProvider  Owner = "provider_operations"
)

type ProposalType string

const (
	ProposalEscalation ProposalType = "escalation"
	ProposalClosure    ProposalType = "closure"
)

type ProposalStatus string

const (
	ProposalPending  ProposalStatus = "pending"
	ProposalApproved ProposalStatus = "approved"
	ProposalRejected ProposalStatus = "rejected"
)

type RunStatus string

const (
	RunRunning    RunStatus = "running"
	RunCompleted  RunStatus = "completed"
	RunIncomplete RunStatus = "incomplete"
	RunSuperseded RunStatus = "superseded"
)

type ToolStatus string

const (
	ToolSucceeded ToolStatus = "succeeded"
	ToolFailed    ToolStatus = "failed"
)

type TurnKind string

const (
	TurnToolCall TurnKind = "tool_call"
	TurnFinal    TurnKind = "final"
)

type HypothesisStatus string

const (
	HypothesisOpen      HypothesisStatus = "open"
	HypothesisSupported HypothesisStatus = "supported"
	HypothesisWeakened  HypothesisStatus = "weakened"
	HypothesisRejected  HypothesisStatus = "rejected"
)

type RootCauseStatus string

const (
	CauseUnknown   RootCauseStatus = "unknown"
	CauseLocalized RootCauseStatus = "localized"
	CauseConfirmed RootCauseStatus = "confirmed"
)

const (
	ToolPayments   = "get_payment_summary"
	ToolCallbacks  = "get_callback_attempts"
	ToolQueue      = "get_queue_health"
	ToolProvider   = "get_provider_status"
	MaxCallbackIDs = 200
)

const (
	ClaimCallbacksMissing    = "callback_attempts_missing"
	ClaimReceiver503         = "receiver_503_observed"
	ClaimCallbacksIncomplete = "callback_source_incomplete"
	ClaimQueueDegraded       = "queue_degraded"
	ClaimQueueHealthy        = "queue_healthy"
	ClaimProviderStale       = "provider_status_stale"
	BaseFacts                = "base_facts"
)

const (
	EventFiring          = "grafana.alert.firing"
	EventResolved        = "grafana.alert.resolved"
	EventSlack           = "slack.message"
	EventSupport         = "support.ticket"
	EventNote            = "manual.note"
	RecoveryCheckPurpose = "recovery_check"
)

func Owners() []Owner {
	return []Owner{OwnerTechOps, OwnerCallbacks, OwnerPayments, OwnerMerchant, OwnerProvider}
}
func Actions() []Action {
	return []Action{ActionContinue, ActionEscalate, ActionRequestInformation, ActionMonitor, ActionProposeClosure}
}
func Claims() []string {
	return []string{ClaimCallbacksMissing, ClaimReceiver503, ClaimCallbacksIncomplete, ClaimQueueDegraded, ClaimQueueHealthy, ClaimProviderStale}
}
func ToolNames() []string { return []string{ToolPayments, ToolCallbacks, ToolQueue, ToolProvider} }
