package cli

import (
	"apm-investigator/internal/incident"
	"testing"
)

func TestReviewRecoversModeWithoutAIConfiguration(t *testing.T) {
	t.Setenv("AI_MODE", "invalid")
	o := options{rootDir: "testdata/scenarios"}
	got := o.commandContext([]incident.Run{{Mode: "live", BaseRevision: 7}, {Mode: "mock", BaseRevision: 2}})
	if got.Mode != "live" {
		t.Fatal("review silently switched live to mock", got)
	}
	o.mode = "mock"
	if o.commandContext([]incident.Run{{Mode: "live", BaseRevision: 7}}).Mode != "mock" {
		t.Fatal("explicit override lost")
	}
}
