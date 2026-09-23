package cli

import (
	"fmt"

	"apm-investigator/internal/render"
	"apm-investigator/internal/replay"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func (o *options) proposalCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "proposal"}
	cmd.AddCommand(
		&cobra.Command{Use: "show <id>", Args: cobra.ExactArgs(1), RunE: o.showProposal},
		o.decisionCommand("approve", true),
		o.decisionCommand("reject", false),
	)
	return cmd
}

func (o *options) showProposal(cmd *cobra.Command, args []string) error {
	id, err := uuid.Parse(args[0])
	if err != nil {
		return fmt.Errorf("invalid proposal ID")
	}
	s, err := openStore(cmd)
	if err != nil {
		return err
	}
	defer s.Close()
	proposal, err := s.Proposal(cmd.Context(), id)
	if err != nil {
		return err
	}
	if o.asJSON {
		return render.JSON(cmd.OutOrStdout(), proposal)
	}
	runs, err := s.Runs(cmd.Context(), proposal.IncidentID)
	if err != nil {
		return err
	}
	render.Proposal(cmd.OutOrStdout(), proposal, o.commandContext(runs))
	return nil
}

func (o *options) decisionCommand(verb string, approve bool) *cobra.Command {
	var note string
	cmd := &cobra.Command{
		Use:  verb + " <id>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid proposal ID")
			}
			s, err := openStore(cmd)
			if err != nil {
				return err
			}
			defer s.Close()
			proposal, err := s.Decide(cmd.Context(), id, approve, note)
			if err != nil {
				return err
			}
			if o.asJSON {
				return render.JSON(cmd.OutOrStdout(), proposal)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s: %s\n", proposal.Type, proposal.Status, proposal.ID)
			i, err := s.Get(cmd.Context(), proposal.IncidentID)
			if err != nil {
				return err
			}
			runs, err := s.Runs(cmd.Context(), proposal.IncidentID)
			if err != nil {
				return err
			}
			render.Outcome(cmd.OutOrStdout(), replay.Outcome{Incident: i}, o.commandContext(runs))
			return nil
		},
	}
	cmd.Flags().StringVar(&note, "note", "", "Human decision note")
	return cmd
}
