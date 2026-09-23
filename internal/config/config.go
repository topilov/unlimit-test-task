package config

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"time"
)

type Mode string

const (
	Mock Mode = "mock"
	Live Mode = "live"
)

func (m Mode) Validate() error {
	if m != Mock && m != Live {
		return fmt.Errorf("--ai-mode must be mock or live")
	}
	return nil
}

type AI struct {
	Mode            Mode
	APIKey          string
	Model           string
	ReasoningEffort string
	Timeout         time.Duration
	MaxSteps        int
}

func DatabaseURL() string {
	return value("DATABASE_URL", "postgres://apm:apm@localhost:55432/apm?sslmode=disable")
}

func LoadAI(modeOverride string) (AI, error) {
	mode := modeOverride
	if mode == "" {
		mode = value("AI_MODE", string(Mock))
	}
	cfg := AI{Mode: Mode(mode)}
	if err := cfg.Mode.Validate(); err != nil {
		return cfg, err
	}
	var err error
	cfg.MaxSteps, err = strconv.Atoi(value("MAX_INVESTIGATION_STEPS", "6"))
	if err != nil || cfg.MaxSteps < 1 || cfg.MaxSteps > 30 {
		return cfg, fmt.Errorf("MAX_INVESTIGATION_STEPS must be 1..30")
	}
	if cfg.Mode == Mock {
		return cfg, nil
	}
	cfg.APIKey = os.Getenv("OPENAI_API_KEY")
	cfg.Model = value("OPENAI_MODEL", "gpt-6-luna")
	cfg.ReasoningEffort = value("OPENAI_REASONING_EFFORT", "high")
	cfg.Timeout, err = time.ParseDuration(value("OPENAI_TIMEOUT", "45s"))
	if err != nil || cfg.Timeout <= 0 {
		return cfg, fmt.Errorf("OPENAI_TIMEOUT must be a positive duration")
	}
	if !slices.Contains([]string{"none", "low", "medium", "high", "xhigh", "max"}, cfg.ReasoningEffort) {
		return cfg, fmt.Errorf("unsupported OPENAI_REASONING_EFFORT")
	}
	return cfg, nil
}

func value(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
