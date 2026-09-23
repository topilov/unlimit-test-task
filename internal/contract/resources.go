package contract

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed prompts/*.md schemas/*.json
var resources embed.FS

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}
type Resources struct {
	Prompt           string
	ResultSchema     map[string]any
	Tools            []Tool
	ReflectionPrompt string
	ReflectionSchema map[string]any
}

func Load() (Resources, error) {
	var out Resources
	prompt, err := resources.ReadFile("prompts/investigator.md")
	if err != nil {
		return out, err
	}
	out.Prompt = strings.TrimSpace(string(prompt))
	if out.Prompt == "" {
		return out, fmt.Errorf("empty investigator prompt")
	}
	var definitions map[string]any
	if err = readJSON("definitions.json", &definitions); err != nil {
		return out, err
	}
	if err = readJSON("result.json", &out.ResultSchema); err != nil {
		return out, err
	}
	out.ResultSchema["$defs"] = definitions
	if err = readJSON("tools.json", &out.Tools); err != nil {
		return out, err
	}
	for index := range out.Tools {
		var envelope map[string]any
		if err = readJSON("tool-call.json", &envelope); err != nil {
			return out, err
		}
		props, ok := envelope["properties"].(map[string]any)
		if !ok {
			return out, fmt.Errorf("tool-call schema missing properties")
		}
		props["arguments"] = out.Tools[index].Parameters
		envelope["$defs"] = definitions
		out.Tools[index].Parameters = envelope
	}
	reflectionPrompt, err := resources.ReadFile("prompts/reflection.md")
	if err != nil {
		return out, err
	}
	out.ReflectionPrompt = strings.TrimSpace(string(reflectionPrompt))
	if out.ReflectionPrompt == "" {
		return out, fmt.Errorf("empty reflection prompt")
	}
	if err := readJSON("reflection.json", &out.ReflectionSchema); err != nil {
		return out, err
	}
	return out, nil
}
func readJSON(name string, target any) error {
	data, err := resources.ReadFile("schemas/" + name)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("schema %s: %w", name, err)
	}
	return nil
}
