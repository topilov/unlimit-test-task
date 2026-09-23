package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"apm-investigator/internal/incident"
	"apm-investigator/internal/replay"
)

func TestHumanOutputExplainsApprovalAndBlockedRecovery(t *testing.T) {
	var b bytes.Buffer
	Outcome(&b, replay.Outcome{Incident: incident.Incident{Status: incident.MonitoringRecovery}, Recovery: &incident.RecoveryResult{NewTrafficHealthy: incident.Pass, OriginalCohortRecovered: incident.Fail, UnresolvedOriginalItems: 1}, Message: "Closure blocked"}, CommandContext{Executable: "./bin/apm", Mode: "mock"})
	for _, v := range []string{"Closure blocked", "original cohort=fail", "unresolved=1"} {
		if !strings.Contains(b.String(), v) {
			t.Fatal(b.String())
		}
	}
	b.Reset()
	Proposal(&b, incident.Proposal{Status: "pending", Title: "\x1b[31munsafe"}, CommandContext{Executable: "./bin/apm", Mode: "mock"})
	if !strings.Contains(b.String(), "approval required") || strings.Contains(b.String(), "\x1b") {
		t.Fatal(b.String())
	}
}

func TestTimelineRejectsMalformedPersistedData(t *testing.T) {
	var output bytes.Buffer
	if err := Timeline(&output, []incident.TimelineEntry{{Kind: "event_received", Data: json.RawMessage(`{}`)}}); err != nil {
		t.Fatal("positive control", err)
	}
	output.Reset()
	if err := Timeline(&output, []incident.TimelineEntry{{Kind: "event_received", Data: json.RawMessage(`{"broken"`)}}); err == nil {
		t.Fatal("corrupt timeline rendered as zero values")
	}
}

func TestGuidedCommandsPreserveContext(t *testing.T) {
	command := CommandContext{Executable: "./a path/apm's", Mode: "live", ScenariosDir: "fixtures with spaces", WithoutMemory: true, Verbose: true}
	var b bytes.Buffer
	Proposal(&b, incident.Proposal{Status: incident.ProposalPending}, command)
	for _, status := range []incident.Status{incident.Investigating, incident.MonitoringRecovery, incident.ManualTriage} {
		Outcome(&b, replay.Outcome{Incident: incident.Incident{Status: status, Scenario: "queue-delay"}}, command)
	}
	for _, line := range strings.Split(b.String(), "\n") {
		if strings.Contains(line, "--note") || strings.HasPrefix(line, "Next:") {
			for _, want := range []string{"'./a path/apm'\\''s'", "--ai-mode live", "--scenarios-dir 'fixtures with spaces'", "--without-memory", "--verbose"} {
				if !strings.Contains(line, want) {
					t.Fatal("lost execution context", line, want)
				}
			}
		}
	}
	if !strings.Contains(b.String(), "proposal reject") || !strings.Contains(b.String(), "--retry") {
		t.Fatal(b.String())
	}
	b.Reset()
	Outcome(&b, replay.Outcome{Incident: incident.Incident{Status: incident.Closed}}, command)
	if strings.Contains(b.String(), "Next:") || !strings.Contains(b.String(), "Incident closed") {
		t.Fatal(b.String())
	}
}
