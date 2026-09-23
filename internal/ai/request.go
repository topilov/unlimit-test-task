package ai

import (
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

func (a *OpenAI) request(input Input) (responses.ResponseNewParams, error) {
	params, err := a.structuredRequest(input, a.resources.Prompt, "investigation_result", a.resources.ResultSchema)
	if err != nil {
		return params, err
	}
	if input.StepsRemaining == 1 {
		return params, nil
	}
	for _, d := range a.resources.Tools {
		params.Tools = append(params.Tools, responses.ToolUnionParam{OfFunction: &responses.FunctionToolParam{
			Name: d.Name, Description: openai.String(d.Description), Parameters: d.Parameters, Strict: openai.Bool(true),
		}})
	}
	return params, nil
}

func (a *OpenAI) structuredRequest(input any, prompt, name string, schema map[string]any) (responses.ResponseNewParams, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return responses.ResponseNewParams{}, fmt.Errorf("encode model input: %w", err)
	}
	params := responses.ResponseNewParams{
		Model:             shared.ResponsesModel(a.model),
		Instructions:      openai.String(prompt),
		Store:             openai.Bool(false),
		ParallelToolCalls: openai.Bool(false),
		MaxOutputTokens:   openai.Int(maxOutputTokens),
		Input:             responses.ResponseNewParamsInputUnion{OfString: openai.String(string(raw))},
		Reasoning:         shared.ReasoningParam{Effort: shared.ReasoningEffort(a.effort)},
		Text: responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigUnionParam{
				OfJSONSchema: &responses.ResponseFormatTextJSONSchemaConfigParam{
					Name:   name,
					Schema: schema,
					Strict: openai.Bool(true),
				},
			},
		},
	}
	return params, nil
}
