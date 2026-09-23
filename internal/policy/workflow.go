package policy

import (
	"encoding/json"
	"fmt"

	"apm-investigator/internal/incident"
)

type ProposalDraft struct {
	Type        incident.ProposalType
	Owner       incident.Owner
	Title, Body string
}
type Decision struct {
	Status   incident.Status
	Proposal *ProposalDraft
}

func AfterInvestigation(i incident.Incident, run incident.Run, evidence []incident.Evidence) Decision {
	if run.Status != incident.RunCompleted || run.Result == nil || run.Result.RecommendedAction != incident.ActionEscalate {
		return Decision{Status: incident.ManualTriage}
	}
	r := run.Result
	if !CanRoute(r, evidence, i.Clock) {
		return Decision{Status: incident.ManualTriage}
	}
	return Decision{Status: incident.AwaitingReview, Proposal: &ProposalDraft{
		Type: incident.ProposalEscalation, Owner: Owner(r.RecommendedOwner), Title: "Investigate " + r.FailureDomain, Body: escalationBody(i, run, evidence),
	}}
}

func AfterRecovery(result incident.RecoveryResult) (Decision, error) {
	out := Decision{Status: incident.MonitoringRecovery}
	if !result.ClosureAllowed {
		return out, nil
	}

	if result.NewTrafficHealthy != incident.Pass || result.OriginalCohortRecovered != incident.Pass || result.UnresolvedOriginalItems != 0 {
		return out, incident.ErrInvalidInput
	}
	data, err := json.Marshal(result)
	if err != nil {
		return out, err
	}
	out.Status = incident.ClosureProposed
	out.Proposal = &ProposalDraft{
		Type:  incident.ProposalClosure,
		Owner: incident.OwnerTechOps,
		Title: "Original impact recovered",
		Body:  string(data),
	}
	return out, nil
}

func AfterDecision(kind incident.ProposalType, approve bool, impact incident.Impact) (incident.Status, error) {
	switch kind {
	case incident.ProposalEscalation:
		if approve {
			return incident.MonitoringRecovery, nil
		}
		return incident.Investigating, nil
	case incident.ProposalClosure:
		if approve && !impact.CohortComplete {
			return "", fmt.Errorf("%w: original cohort completeness is unknown", incident.ErrInvalidInput)
		}
		if approve {
			return incident.Closed, nil
		}
		return incident.MonitoringRecovery, nil
	default:
		return "", fmt.Errorf("%w: proposal type", incident.ErrInvalidInput)
	}
}

func AfterEvent(status incident.Status, event incident.Event) incident.Status {
	if status == incident.AwaitingReview {
		return incident.Investigating
	}
	var payload struct {
		Purpose string `json:"purpose"`
	}
	_ = json.Unmarshal(event.Payload, &payload)
	switch event.Kind {
	case incident.EventFiring, incident.EventSupport, incident.EventSlack:
		return incident.Investigating
	case incident.EventNote:
		if payload.Purpose != incident.RecoveryCheckPurpose {
			return incident.Investigating
		}
	}
	if status == incident.ClosureProposed {
		return incident.MonitoringRecovery
	}
	return status
}
