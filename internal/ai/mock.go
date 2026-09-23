package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"apm-investigator/internal/incident"
)

type Mock struct {
	scripts []mockScript
	turns   []incident.Turn
	step    int
}

type mockScript struct {
	At    time.Time       `json:"at"`
	Turns []incident.Turn `json:"turns"`
}

func LoadMock(path string) (*Mock, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var scripts []mockScript
	if err = json.Unmarshal(b, &scripts); err != nil {
		return nil, err
	}
	for n, script := range scripts {
		if script.At.IsZero() || len(script.Turns) == 0 || (n > 0 && !script.At.After(scripts[n-1].At)) {
			return nil, fmt.Errorf("invalid mock script timeline")
		}
	}
	return &Mock{scripts: scripts}, nil
}
func (m *Mock) Next(ctx context.Context, input Input) (incident.Turn, incident.Metadata, error) {
	meta := incident.Metadata{Model: "scripted"}
	if err := ctx.Err(); err != nil {
		return incident.Turn{}, meta, err
	}
	if m.turns == nil {
		for _, script := range m.scripts {
			if !script.At.After(input.AsOf) {
				m.turns = script.Turns
			}
		}
	}
	if m.step >= len(m.turns) {
		return incident.Turn{}, meta, fmt.Errorf("%w: mock script exhausted", incident.ErrModelUnavailable)
	}
	encoded, err := json.Marshal(m.turns[m.step])
	if err != nil {
		return incident.Turn{}, meta, err
	}
	raw := string(encoded)
	m.step++
	aliases := map[string]string{
		"base":      incident.BaseFacts,
		"callbacks": incident.ToolCallbacks,
		"queue":     incident.ToolQueue,
		"provider":  incident.ToolProvider,
	}
	for alias, kind := range aliases {
		for n := len(input.Context.Evidence) - 1; n >= 0; n-- {
			e := input.Context.Evidence[n]
			if e.Kind == kind {
				raw = strings.ReplaceAll(raw, `"@prior_`+alias+`"`, `"`+e.ID+`"`)
				break
			}
		}
		for n := len(input.Evidence) - 1; n >= 0; n-- {
			e := input.Evidence[n]
			if e.Kind == kind {
				raw = strings.ReplaceAll(raw, `"@`+alias+`"`, `"`+e.ID+`"`)
				break
			}
		}
	}
	var turn incident.Turn
	if err := json.Unmarshal([]byte(raw), &turn); err != nil {
		return turn, meta, err
	}
	return turn, meta, nil
}
