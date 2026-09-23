package cli

import (
	"fmt"

	"apm-investigator/internal/config"
	"apm-investigator/internal/render"
	"apm-investigator/internal/replay"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func (o *options) replayCommand() *cobra.Command {
	var resume string
	var next, retry bool
	replayCmd := &cobra.Command{Use: "replay <scenario>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var id uuid.UUID
		var err error
		if resume != "" {
			id, err = uuid.Parse(resume)
			if err != nil {
				return fmt.Errorf("invalid incident ID")
			}
		}
		cfg, err := config.LoadAI(o.mode)
		if err != nil {
			return err
		}
		s, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer s.Close()
		runner := replay.Runner{
			Store:         s,
			Config:        cfg,
			Root:          o.rootDir,
			Logger:        o.logger(cmd),
			WithoutMemory: o.withoutMemory,
			AdvanceNext:   next,
			Retry:         retry,
		}
		out, err := runner.Run(cmd.Context(), args[0], id)
		if out.Incident.ID != uuid.Nil {
			if o.asJSON {
				if e := render.JSON(cmd.OutOrStdout(), out); e != nil {
					return e
				}
			} else {
				command := o.commandContext(nil)
				command.Mode = string(cfg.Mode)
				render.Outcome(cmd.OutOrStdout(), out, command)
			}
		}
		return err
	}}
	replayCmd.Flags().StringVar(&resume, "incident", "", "Continue an existing incident")
	replayCmd.Flags().BoolVar(&next, "next", false, "Ingest the next event batch, superseding an older proposal")
	replayCmd.Flags().BoolVar(&retry, "retry", false, "Retry manual triage at the same replay time")
	replayCmd.MarkFlagsMutuallyExclusive("next", "retry")
	return replayCmd
}
