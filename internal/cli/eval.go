package cli

import (
	"fmt"

	"apm-investigator/internal/config"
	"apm-investigator/internal/eval"
	"apm-investigator/internal/render"
	"apm-investigator/internal/replay"
	"github.com/spf13/cobra"
)

func (o *options) evalCommand() *cobra.Command {
	var scenarioName string
	evaluate := &cobra.Command{Use: "eval", Short: "Evaluate fixtures with explicit synthetic reviewer decisions", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadAI(o.mode)
		if err != nil {
			return err
		}
		s, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer s.Close()
		names := []string{scenarioName}
		if scenarioName == "" {
			names, err = replay.Names(o.rootDir)
			if err != nil {
				return err
			}
		}
		runner := &replay.Runner{Store: s, Config: cfg, Root: o.rootDir, Logger: o.logger(cmd), WithoutMemory: o.withoutMemory}
		reports := []eval.Report{}
		for _, name := range names {
			r, err := eval.Run(cmd.Context(), runner, name)
			if err != nil {
				return err
			}
			reports = append(reports, r)
		}
		if o.asJSON {
			if err = render.JSON(cmd.OutOrStdout(), reports); err != nil {
				return err
			}
		} else {
			for _, r := range reports {
				status := "PASS"
				if !r.Passed {
					status = "FAIL"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", status, r.Scenario)
				for _, f := range r.Failures {
					fmt.Fprintln(cmd.OutOrStdout(), "  FAIL:", f)
				}
				for _, w := range r.Warnings {
					fmt.Fprintln(cmd.OutOrStdout(), "  WARN:", w)
				}
				if o.verbose {
					fmt.Fprintf(cmd.OutOrStdout(), "  tools=%d model_calls=%d API_attempts=%d latency=%dms tokens=%d/%d premature_closure=%d memory_rules=%d\n", r.ToolCalls, r.ModelCalls, r.APICalls, r.LatencyMS, r.InputTokens, r.OutputTokens, r.PrematureClosureCount, len(r.MemoryIDs))
				}
			}
		}
		if err = eval.Summary(reports); err != nil {
			return err
		}
		if !o.asJSON {
			fmt.Fprintln(cmd.OutOrStdout(), "ALL SCENARIOS PASSED (structured contracts and workflow; prose requires review)")
		}
		return nil
	}}
	evaluate.Flags().StringVar(&scenarioName, "scenario", "", "Evaluate one scenario")
	return evaluate
}
