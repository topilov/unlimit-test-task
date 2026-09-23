package cli

import (
	"log/slog"

	"apm-investigator/internal/config"
	"apm-investigator/internal/store"
	"github.com/spf13/cobra"
)

type options struct {
	asJSON        bool
	verbose       bool
	withoutMemory bool
	rootDir, mode string
}

func NewCommand() *cobra.Command {
	o := &options{}
	root := &cobra.Command{
		Use:               "apm",
		Short:             "Evidence-driven investigation of synthetic payment incidents",
		SilenceUsage:      true,
		SilenceErrors:     true,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}
	root.PersistentFlags().BoolVar(&o.withoutMemory, "without-memory", false, "Investigate without learned rules (baseline comparison)")
	root.PersistentFlags().BoolVar(&o.asJSON, "json", false, "Print machine-readable JSON")
	root.PersistentFlags().BoolVarP(&o.verbose, "verbose", "v", false, "Show model logs and evaluation metrics")
	root.PersistentFlags().StringVar(&o.rootDir, "scenarios-dir", "testdata/scenarios", "Synthetic scenario directory")
	root.PersistentFlags().StringVar(&o.mode, "ai-mode", "", "mock or live (defaults to AI_MODE or mock)")
	root.AddCommand(o.scenarioCommand(), o.replayCommand(), o.incidentCommand(), o.proposalCommand(), o.evalCommand(), o.feedbackCommand(), o.memoryCommand(), migrateCommand())
	return root
}

func (o *options) logger(cmd *cobra.Command) *slog.Logger {
	if !o.verbose {
		return nil
	}
	return slog.New(slog.NewJSONHandler(cmd.ErrOrStderr(), nil))
}

func openStore(cmd *cobra.Command) (*store.Store, error) {
	return store.Open(cmd.Context(), config.DatabaseURL())
}
