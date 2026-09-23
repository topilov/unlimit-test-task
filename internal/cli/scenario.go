package cli

import (
	"fmt"

	"apm-investigator/internal/render"
	"apm-investigator/internal/replay"
	"github.com/spf13/cobra"
)

func (o *options) scenarioCommand() *cobra.Command {
	scenario := &cobra.Command{Use: "scenario"}
	scenario.AddCommand(&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		names, err := replay.Names(o.rootDir)
		if err != nil {
			return err
		}
		if o.asJSON {
			return render.JSON(cmd.OutOrStdout(), names)
		}
		for _, n := range names {
			fmt.Fprintln(cmd.OutOrStdout(), n)
		}
		return nil
	}})
	return scenario
}
