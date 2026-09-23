package render

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"

	"apm-investigator/internal/incident"
	"apm-investigator/internal/policy"
	"apm-investigator/internal/replay"
)

func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
}
func JSON(w io.Writer, v any) error {
	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	return e.Encode(v)
}
func Incident(w io.Writer, i incident.Incident) {
	fmt.Fprintf(w, "%s  %s  revision=%d\n%s / %s / %s\nUnique payments: %d; potential affected: %d; severity: %s\nReplay time: %s\n", i.ID, i.Status, i.Revision, safe(i.Scope.MerchantID), safe(i.Scope.Method), safe(i.Scope.Environment), i.Impact.UniquePayments, i.Impact.AffectedPayments, policy.Severity(i.Impact), i.Clock.Format("15:04:05"))
}

type CommandContext struct {
	Executable, Mode, ScenariosDir string
	WithoutMemory, Verbose         bool
}

func (c CommandContext) prefix() string {
	parts := []string{shellWord(c.Executable)}
	if c.Mode != "" {
		parts = append(parts, "--ai-mode", shellWord(c.Mode))
	}
	if c.ScenariosDir != "" && c.ScenariosDir != "testdata/scenarios" {
		parts = append(parts, "--scenarios-dir", shellWord(c.ScenariosDir))
	}
	if c.WithoutMemory {
		parts = append(parts, "--without-memory")
	}
	if c.Verbose {
		parts = append(parts, "--verbose")
	}
	return strings.Join(parts, " ")
}

func Proposal(w io.Writer, p incident.Proposal, command CommandContext) {
	fmt.Fprintf(w, "Proposal %s  %s  %s\nOwner: %s\n%s\n%s\n", p.ID, p.Type, p.Status, safe(string(p.Owner)), safe(p.Title), safe(p.Body))
	if p.Status == incident.ProposalPending {
		fmt.Fprintln(w, "approval required")
		fmt.Fprintf(w, "%s proposal approve %s --note \"Reviewed evidence\"\n", command.prefix(), p.ID)
		fmt.Fprintf(w, "%s proposal reject %s --note \"Reason for rejection\"\n", command.prefix(), p.ID)
	}
}
func Outcome(w io.Writer, out replay.Outcome, command CommandContext) {
	i := out.Incident
	fmt.Fprintf(w, "Incident %s  %s\n", i.ID, i.Status)
	if out.Recovery != nil {
		r := out.Recovery
		fmt.Fprintf(w, "Recovery: new traffic=%s; original cohort=%s; unresolved=%d\n", r.NewTrafficHealthy, r.OriginalCohortRecovered, r.UnresolvedOriginalItems)
		if !r.ClosureAllowed {
			fmt.Fprintln(w, "Closure blocked")
		}
	} else if out.Proposal != nil {
		fmt.Fprintf(w, "Potentially affected payments: %d/%d\n", i.Impact.AffectedPayments, i.Impact.UniquePayments)
		fmt.Fprintf(w, "%s (owner: %s)\n", safe(out.Proposal.Title), out.Proposal.Owner)
	}
	switch {
	case out.Proposal != nil && out.Proposal.Status == incident.ProposalPending:
		fmt.Fprintf(w, "Next: %s proposal show %s\n", command.prefix(), out.Proposal.ID)
	case i.Status == incident.MonitoringRecovery || i.Status == incident.Investigating:
		fmt.Fprintf(w, "Next: %s replay %s --incident %s\n", command.prefix(), shellWord(i.Scenario), i.ID)
	case i.Status == incident.ManualTriage:
		fmt.Fprintln(w, "Manual follow-up required")
		fmt.Fprintf(w, "Next: %s replay %s --incident %s --retry\n", command.prefix(), shellWord(i.Scenario), i.ID)
	case i.Status == incident.Closed:
		fmt.Fprintln(w, "Incident closed")
	case i.Status != incident.Closed:
		fmt.Fprintf(w, "Next: %s incident show %s\n", command.prefix(), i.ID)
	}
}

func shellWord(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("_./-", r)
	}) == -1 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func Timeline(w io.Writer, entries []incident.TimelineEntry) error {
	for _, entry := range entries {
		fmt.Fprintf(w, "%s  %s\n", entry.At.Format("15:04:05"), entry.Kind)
		switch entry.Kind {
		case "evidence":
			var e incident.Evidence
			if err := json.Unmarshal(entry.Data, &e); err != nil {
				return fmt.Errorf("decode timeline %s at %s: %w", entry.Kind, entry.At.Format("15:04:05"), err)
			}
			fmt.Fprintf(w, "  %s %s\n", e.ID, safe(e.Summary))
		case "model_turn":
			var t incident.ModelTurn
			if err := json.Unmarshal(entry.Data, &t); err != nil {
				return fmt.Errorf("decode timeline %s at %s: %w", entry.Kind, entry.At.Format("15:04:05"), err)
			}
			if t.Turn.ToolCall != nil {
				fmt.Fprintf(w, "  agent -> %s\n", safe(t.Turn.ToolCall.Name))
			}
			for _, h := range t.Turn.Hypotheses {
				fmt.Fprintf(w, "  %s %s: %s [support: %s; contradict: %s]\n", safe(h.ID), safe(string(h.Status)), safe(h.Statement), strings.Join(h.SupportingEvidence, ","), strings.Join(h.ContradictingEvidence, ","))
			}
			if t.Turn.Final != nil {
				fmt.Fprintf(w, "  %s (root cause: %s)\n", safe(t.Turn.Final.Summary), t.Turn.Final.RootCauseStatus)
			}
			if t.Error != "" {
				fmt.Fprintf(w, "  %s\n", safe(t.Error))
			}
		case incident.RecoveryCheckPurpose:
			var r incident.RecoveryResult
			if err := json.Unmarshal(entry.Data, &r); err != nil {
				return fmt.Errorf("decode timeline %s at %s: %w", entry.Kind, entry.At.Format("15:04:05"), err)
			}
			fmt.Fprintf(w, "  new traffic=%s original cohort=%s unresolved=%d closure allowed=%t\n", r.NewTrafficHealthy, r.OriginalCohortRecovered, r.UnresolvedOriginalItems, r.ClosureAllowed)
		case "proposal_created", "proposal_approved", "proposal_rejected":
			var p incident.Proposal
			if err := json.Unmarshal(entry.Data, &p); err != nil {
				return fmt.Errorf("decode timeline %s at %s: %w", entry.Kind, entry.At.Format("15:04:05"), err)
			}
			fmt.Fprintf(w, "  %s %s %s -> %s\n", p.ID, p.Type, p.Status, safe(string(p.Owner)))
		case "event_received":
			var e incident.Event
			if err := json.Unmarshal(entry.Data, &e); err != nil {
				return fmt.Errorf("decode timeline %s at %s: %w", entry.Kind, entry.At.Format("15:04:05"), err)
			}
			fmt.Fprintf(w, "  %s occurred=%s available=%s\n", safe(e.Kind), e.OccurredAt.Format("15:04:05"), e.AvailableAt.Format("15:04:05"))
		}
	}
	return nil
}
