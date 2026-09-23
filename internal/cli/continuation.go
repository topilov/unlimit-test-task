package cli

import (
	"os"

	"apm-investigator/internal/incident"
	"apm-investigator/internal/render"
)

// Printed commands carry execution flags through review commands, which must
// remain usable without model configuration or an API key. For a standalone
// review, recover the mode from the most recent persisted investigation.
func (o *options) commandContext(runs []incident.Run) render.CommandContext {
	mode := o.mode
	if mode == "" {
		mode = "mock"
		var revision int64 = -1
		for _, run := range runs {
			if run.BaseRevision >= revision && (run.Mode == "mock" || run.Mode == "live") {
				mode, revision = run.Mode, run.BaseRevision
			}
		}
	}
	return render.CommandContext{Executable: os.Args[0], Mode: mode, ScenariosDir: o.rootDir, WithoutMemory: o.withoutMemory, Verbose: o.verbose}
}
