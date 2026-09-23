package cli

import (
	"fmt"

	"apm-investigator/internal/incident"
	"apm-investigator/internal/render"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func (o *options) incidentCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "incident"}
	cmd.AddCommand(
		&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: o.listIncidents},
		&cobra.Command{Use: "show <id>", Args: cobra.ExactArgs(1), RunE: o.showIncident},
		&cobra.Command{Use: "timeline <id>", Args: cobra.ExactArgs(1), RunE: o.showTimeline},
	)
	return cmd
}

func (o *options) listIncidents(cmd *cobra.Command, _ []string) error {
	s, err := openStore(cmd)
	if err != nil {
		return err
	}
	defer s.Close()
	items, err := s.List(cmd.Context())
	if err != nil {
		return err
	}
	if o.asJSON {
		return render.JSON(cmd.OutOrStdout(), items)
	}
	for _, i := range items {
		fmt.Fprintf(cmd.OutOrStdout(), "%s  %-20s %s\n", i.ID, i.Status, i.Scenario)
	}
	return nil
}

func (o *options) showIncident(cmd *cobra.Command, args []string) error {
	id, err := uuid.Parse(args[0])
	if err != nil {
		return fmt.Errorf("invalid incident ID")
	}
	s, err := openStore(cmd)
	if err != nil {
		return err
	}
	defer s.Close()
	i, err := s.Get(cmd.Context(), id)
	if err != nil {
		return err
	}
	proposals, err := s.Proposals(cmd.Context(), id)
	if err != nil {
		return err
	}
	runs, err := s.Runs(cmd.Context(), id)
	if err != nil {
		return err
	}
	if o.asJSON {
		return render.JSON(cmd.OutOrStdout(), incidentView{Incident: i, Runs: runs, Proposals: proposals})
	}
	render.Incident(cmd.OutOrStdout(), i)
	for _, run := range runs {
		if run.Result != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "\nRun %s  %s (memory=%d)\n%s\n", run.ID, run.Status, len(run.Memory), run.Result.Summary)
		}
	}
	for _, p := range proposals {
		render.Proposal(cmd.OutOrStdout(), p, o.commandContext(runs))
	}
	return nil
}

type incidentView struct {
	Incident  incident.Incident   `json:"incident"`
	Runs      []incident.Run      `json:"runs"`
	Proposals []incident.Proposal `json:"proposals"`
}

func (o *options) showTimeline(cmd *cobra.Command, args []string) error {
	id, err := uuid.Parse(args[0])
	if err != nil {
		return fmt.Errorf("invalid incident ID")
	}
	s, err := openStore(cmd)
	if err != nil {
		return err
	}
	defer s.Close()

	if _, err = s.Get(cmd.Context(), id); err != nil {
		return err
	}
	entries, err := s.Timeline(cmd.Context(), id)
	if err != nil {
		return err
	}
	if o.asJSON {
		return render.JSON(cmd.OutOrStdout(), entries)
	}
	return render.Timeline(cmd.OutOrStdout(), entries)
}
