package eval

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"apm-investigator/internal/incident"
	"gopkg.in/yaml.v3"
)

func loadExpected(dir string) (Expected, error) {
	var expected Expected
	b, err := os.ReadFile(filepath.Join(dir, "expected.yaml"))
	if err != nil {
		return expected, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(b))
	decoder.KnownFields(true)
	if err = decoder.Decode(&expected); err != nil {
		return expected, err
	}
	if !slices.Contains(incident.Actions(), expected.Action) || (expected.InitialStatus != incident.AwaitingReview && expected.InitialStatus != incident.ManualTriage) {
		return expected, fmt.Errorf("invalid expected action/status")
	}
	for n, c := range expected.Investigations {
		if c.At.IsZero() || n > 0 && !c.At.After(expected.Investigations[n-1].At) || c.MinRevisedHypotheses < 0 {
			return expected, fmt.Errorf("invalid investigation checkpoints")
		}
		if !slices.Contains(incident.Actions(), c.Action) || (c.InitialStatus != incident.AwaitingReview && c.InitialStatus != incident.ManualTriage) {
			return expected, fmt.Errorf("invalid intermediate action/status")
		}
	}
	for n, c := range expected.Recovery {
		if c.At.IsZero() || n > 0 && !c.At.After(expected.Recovery[n-1].At) {
			return expected, fmt.Errorf("recovery checkpoints must have strictly increasing timestamps")
		}
	}
	return expected, nil
}
