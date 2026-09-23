package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestInvalidArgumentsFailBeforeDatabaseAccess(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		message string
	}{
		{[]string{"replay", "queue-delay", "--incident", "bad-id"}, "invalid incident ID"},
		{[]string{"eval", "--ai-mode", "unexpected"}, "--ai-mode must be mock or live"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			t.Setenv("DATABASE_URL", "not a PostgreSQL URL")
			cmd := NewCommand()
			cmd.SetArgs(tc.args)
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatal(err)
			}
		})
	}
}
