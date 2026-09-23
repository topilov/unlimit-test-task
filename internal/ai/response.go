package ai

import (
	"encoding/json"
	"fmt"
	"strings"

	"apm-investigator/internal/incident"
	"apm-investigator/internal/jsonutil"
	"github.com/openai/openai-go/v3/responses"
)

func parseResponse(response *responses.Response) (turn incident.Turn, err error) {
	calls, textParts, err := responseShape(response)
	if err != nil {
		return turn, err
	}
	if calls == 1 && textParts == 0 {
		for _, item := range response.Output {
			if item.Type != "function_call" {
				continue
			}
			f := item.AsFunctionCall()
			var payload struct {
				Arguments     json.RawMessage         `json:"arguments"`
				Hypotheses    []incident.Hypothesis   `json:"hypotheses"`
				OpenQuestions []incident.OpenQuestion `json:"open_questions"`
			}
			if e := jsonutil.DecodeObject([]byte(f.Arguments), &payload); e != nil || !json.Valid(payload.Arguments) || strings.TrimSpace(string(payload.Arguments)) == "null" {
				return turn, fmt.Errorf("%w: tool arguments", incident.ErrModelInvalidOutput)
			}
			return incident.Turn{Kind: incident.TurnToolCall, ToolCall: &incident.ToolCall{Name: f.Name, Arguments: payload.Arguments}, Hypotheses: payload.Hypotheses, OpenQuestions: payload.OpenQuestions}, nil
		}
	}
	if calls != 0 || textParts != 1 {
		return turn, fmt.Errorf("%w: expected exactly one action", incident.ErrModelInvalidOutput)
	}
	var result incident.Result
	if err := jsonutil.DecodeObject([]byte(response.OutputText()), &result); err != nil {
		return turn, fmt.Errorf("%w: final schema", incident.ErrModelInvalidOutput)
	}
	return incident.Turn{
		Kind:          incident.TurnFinal,
		Final:         &result,
		Hypotheses:    result.Hypotheses,
		OpenQuestions: result.OpenQuestions,
	}, nil
}

func responseShape(response *responses.Response) (calls, texts int, err error) {
	if response.Status != "completed" {
		return 0, 0, fmt.Errorf("%w: response status %s", incident.ErrModelInvalidOutput, response.Status)
	}
	for _, item := range response.Output {
		switch item.Type {
		case "function_call":
			calls++
		case "message":
			for _, content := range item.AsMessage().Content {
				if content.Type == "refusal" {
					return 0, 0, fmt.Errorf("%w: model refusal", incident.ErrModelInvalidOutput)
				}
				if content.Type == "output_text" {
					texts++
				}
			}
		case "reasoning":
		default:
			return 0, 0, fmt.Errorf("%w: unexpected response item", incident.ErrModelInvalidOutput)
		}
	}
	return calls, texts, nil
}
