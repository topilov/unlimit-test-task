package replay

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"apm-investigator/internal/incident"
	"apm-investigator/internal/tools"
	"gopkg.in/yaml.v3"
)

type Scenario struct {
	Name             string           `yaml:"name"`
	Description      string           `yaml:"description"`
	Scope            incident.Scope   `yaml:"scope"`
	StartedAt        time.Time        `yaml:"started_at"`
	CriticalMerchant bool             `yaml:"critical_merchant"`
	Sources          *tools.Sources   `yaml:"-"`
	Events           []incident.Event `yaml:"-"`
	Directory        string           `yaml:"-"`
}

func Names(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, strings.ReplaceAll(e.Name(), "_", "-"))
		}
	}
	slices.Sort(names)
	return names, nil
}
func Load(root, name string) (*Scenario, error) {
	if name == "" || strings.ContainsAny(name, "/\\.") {
		return nil, fmt.Errorf("invalid scenario name")
	}
	dir := filepath.Join(root, strings.ReplaceAll(name, "-", "_"))
	b, err := os.ReadFile(filepath.Join(dir, "scenario.yaml"))
	if err != nil {
		return nil, err
	}
	var s Scenario
	if err = yaml.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	s.Directory = dir
	if s.Name != name || s.Scope.WindowEnd.Before(s.Scope.WindowStart) || s.StartedAt.Before(s.Scope.WindowEnd) {
		return nil, fmt.Errorf("invalid scenario metadata")
	}
	s.Sources, err = tools.Load(filepath.Join(dir, "sources"))
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e incident.Event
		if err = json.Unmarshal(scanner.Bytes(), &e); err != nil {
			return nil, err
		}
		s.Events = append(s.Events, e)
	}
	if err = scanner.Err(); err != nil {
		return nil, err
	}
	if len(s.Events) == 0 {
		return nil, fmt.Errorf("scenario has no events")
	}
	for k, e := range s.Events {
		if e.AvailableAt.Before(e.OccurredAt) || (k > 0 && e.AvailableAt.Before(s.Events[k-1].AvailableAt)) {
			return nil, fmt.Errorf("events are not ordered by availability")
		}
	}
	return &s, nil
}
