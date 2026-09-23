package cli

import (
	"fmt"

	"apm-investigator/internal/ai"
	"apm-investigator/internal/config"
	"apm-investigator/internal/incident"
	"apm-investigator/internal/learning"
	"apm-investigator/internal/render"
	"apm-investigator/internal/store"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func (o *options) feedbackCommand() *cobra.Command {
	var verdict, note, mockResponse string
	cmd := &cobra.Command{Use: "feedback", Short: "Review a completed investigation and draft a lesson"}
	cmd.PersistentFlags().StringVar(&mockResponse, "mock-response", "testdata/learning/reflection.json", "Scripted reflection fixture (mock mode only)")
	add := &cobra.Command{Use: "add <run-id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		id, err := uuid.Parse(args[0])
		if err != nil {
			return fmt.Errorf("invalid run ID")
		}
		s, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer s.Close()
		f, err := s.AddFeedback(cmd.Context(), id, incident.Verdict(verdict), note)
		if err != nil {
			return err
		}
		return o.reflectFeedback(cmd, s, f, mockResponse)
	}}
	add.Flags().StringVar(&verdict, "verdict", "", "helpful, incorrect or incomplete")
	add.Flags().StringVar(&note, "note", "", "Explain what was useful, wrong or missing")
	cmd.AddCommand(add, &cobra.Command{Use: "reflect <feedback-id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		id, err := uuid.Parse(args[0])
		if err != nil {
			return fmt.Errorf("invalid feedback ID")
		}
		s, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer s.Close()
		f, err := s.Feedback(cmd.Context(), id)
		if err != nil {
			return err
		}
		return o.reflectFeedback(cmd, s, f, mockResponse)
	}}, &cobra.Command{Use: "show <feedback-id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		id, err := uuid.Parse(args[0])
		if err != nil {
			return fmt.Errorf("invalid feedback ID")
		}
		s, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer s.Close()
		f, err := s.Feedback(cmd.Context(), id)
		if err != nil {
			return err
		}
		l, err := s.LessonForFeedback(cmd.Context(), id)
		if err != nil {
			return err
		}
		result := learning.Result{Feedback: f, Lesson: l}
		if o.asJSON {
			return render.JSON(cmd.OutOrStdout(), result)
		}
		render.FeedbackDetails(cmd.OutOrStdout(), result)
		return nil
	}})
	return cmd
}

func (o *options) reflectFeedback(cmd *cobra.Command, s *store.Store, f incident.Feedback, mockResponse string) error {
	result := learning.Result{Feedback: f}
	var operationErr error
	if f.Reflection != nil {
		result.Lesson, operationErr = s.LessonForFeedback(cmd.Context(), f.ID)
	} else {
		var agent learning.Reflector
		var mode string
		agent, mode, operationErr = o.reflector(cmd, s, f.RunID, mockResponse)
		if operationErr == nil {
			service := learning.Service{Store: s, Agent: agent, Mode: mode}
			result, operationErr = service.Reflect(cmd.Context(), f.ID)
		}
	}
	if err := o.printFeedback(cmd, result); err != nil {
		return err
	}
	if operationErr != nil {
		return fmt.Errorf("feedback %s saved; reflection failed: %w", f.ID, operationErr)
	}
	return nil
}

func (o *options) reflector(cmd *cobra.Command, s *store.Store, runID uuid.UUID, mockResponse string) (learning.Reflector, string, error) {
	r, err := s.Run(cmd.Context(), runID)
	if err != nil {
		return nil, "", err
	}
	if o.mode != "" && o.mode != r.Mode {
		return nil, "", fmt.Errorf("reflection must use the source run's %s mode", r.Mode)
	}
	cfg, err := config.LoadAI(r.Mode)
	if err != nil {
		return nil, "", err
	}
	if cfg.Mode == config.Mock {
		return ai.MockReflection{Path: mockResponse}, r.Mode, nil
	}
	if cmd.Flags().Changed("mock-response") {
		return nil, "", fmt.Errorf("mock-response is not allowed in live mode")
	}
	agent, err := ai.NewOpenAI(cfg.APIKey, cfg.Model, cfg.ReasoningEffort, cfg.Timeout)
	return agent, r.Mode, err
}

func (o *options) printFeedback(cmd *cobra.Command, result learning.Result) error {
	if o.asJSON {
		return render.JSON(cmd.OutOrStdout(), result)
	}
	render.Feedback(cmd.OutOrStdout(), result)
	return nil
}
