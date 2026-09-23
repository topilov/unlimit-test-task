package ai

import (
	"context"
	"fmt"
	"os"

	"apm-investigator/internal/incident"
	"apm-investigator/internal/jsonutil"
	"github.com/google/uuid"
)

type ReflectionInput struct {
	Context  incident.RunContext   `json:"context"`
	RunID    uuid.UUID             `json:"run_id"`
	Scope    incident.Scope        `json:"scope"`
	Result   *incident.Result      `json:"result"`
	Turns    []incident.ModelTurn  `json:"turns"`
	Evidence []incident.Evidence   `json:"evidence"`
	Tools    []incident.ToolRun    `json:"tools"`
	Memory   []incident.MemoryRule `json:"memory"`
	Verdict  incident.Verdict      `json:"verdict"`
	Note     string                `json:"feedback"`
}

func (a *OpenAI) Reflect(ctx context.Context, input ReflectionInput) (result incident.Reflection, meta incident.Metadata, err error) {
	params, err := a.structuredRequest(input, a.resources.ReflectionPrompt, "investigation_reflection", a.resources.ReflectionSchema)
	if err != nil {
		return result, meta, err
	}
	response, meta, err := a.respond(ctx, params)
	if err != nil {
		return result, meta, err
	}
	calls, texts, err := responseShape(response)
	if err != nil {
		return result, meta, err
	}
	if calls != 0 || texts != 1 {
		return result, meta, fmt.Errorf("%w: reflection requires one text result", incident.ErrModelInvalidOutput)
	}
	if err := jsonutil.DecodeObject([]byte(response.OutputText()), &result); err != nil {
		return result, meta, fmt.Errorf("%w: reflection schema", incident.ErrModelInvalidOutput)
	}
	return result, meta, nil
}

type MockReflection struct{ Path string }

func (m MockReflection) Reflect(ctx context.Context, input ReflectionInput) (result incident.Reflection, meta incident.Metadata, err error) {
	meta.Model = "scripted-reflection"
	if err := ctx.Err(); err != nil {
		return result, meta, err
	}
	data, err := os.ReadFile(m.Path)
	if err != nil {
		return result, meta, err
	}
	if err := jsonutil.DecodeObject(data, &result); err != nil {
		return result, meta, err
	}
	if result.Lesson != nil {
		for n, ref := range result.Lesson.EvidenceIDs {
			for _, e := range input.Evidence {
				if ref == "@"+e.Kind {
					result.Lesson.EvidenceIDs[n] = e.ID
					break
				}
			}
		}
	}
	return result, meta, nil
}
