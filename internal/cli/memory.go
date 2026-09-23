package cli

import (
	"fmt"

	"apm-investigator/internal/incident"
	"apm-investigator/internal/render"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func (o *options) memoryCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "memory", Short: "Inspect, approve or disable diagnostic lessons"}
	cmd.AddCommand(&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		s, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer s.Close()
		lessons, err := s.Lessons(cmd.Context())
		if err != nil {
			return err
		}
		if o.asJSON {
			return render.JSON(cmd.OutOrStdout(), lessons)
		}
		render.LessonList(cmd.OutOrStdout(), lessons)
		return nil
	}}, &cobra.Command{Use: "show <id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		id, err := uuid.Parse(args[0])
		if err != nil {
			return fmt.Errorf("invalid lesson ID")
		}
		s, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer s.Close()
		l, err := s.Lesson(cmd.Context(), id)
		if err != nil {
			return err
		}
		if o.asJSON {
			return render.JSON(cmd.OutOrStdout(), l)
		}
		r, err := s.Run(cmd.Context(), l.RunID)
		if err != nil {
			return err
		}
		render.Lesson(cmd.OutOrStdout(), l)
		fmt.Fprintf(cmd.OutOrStdout(), "Review: incident show %s\n", r.IncidentID)
		return nil
	}}, o.memoryDecision("approve", incident.LessonActive), o.memoryDecision("disable", incident.LessonDisabled))
	return cmd
}

func (o *options) memoryDecision(verb string, status incident.LessonStatus) *cobra.Command {
	return &cobra.Command{Use: verb + " <id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		id, err := uuid.Parse(args[0])
		if err != nil {
			return fmt.Errorf("invalid lesson ID")
		}
		s, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer s.Close()
		l, err := s.SetLessonStatus(cmd.Context(), id, status)
		if err != nil {
			return err
		}
		if o.asJSON {
			return render.JSON(cmd.OutOrStdout(), l)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Lesson %s  %s\n", l.ID, l.Status)
		return nil
	}}
}
