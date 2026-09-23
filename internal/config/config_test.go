package config

import "testing"

func TestAISettingsAreScopedToMode(t *testing.T) {
	t.Setenv("AI_MODE", "live")
	t.Setenv("MAX_INVESTIGATION_STEPS", "6")
	t.Setenv("OPENAI_TIMEOUT", "invalid")
	t.Setenv("OPENAI_REASONING_EFFORT", "invalid")
	cfg, err := LoadAI("mock")
	if err != nil || cfg.Mode != Mock || cfg.MaxSteps != 6 {
		t.Fatal("mock depends on live settings", cfg, err)
	}
	if _, err = LoadAI(""); err == nil {
		t.Fatal("invalid live settings accepted")
	}
	if _, err = LoadAI("unknown"); err == nil {
		t.Fatal("invalid mode accepted")
	}
	t.Setenv("MAX_INVESTIGATION_STEPS", "0")
	if _, err = LoadAI("mock"); err == nil {
		t.Fatal("mock still needs a bounded step budget")
	}
}
