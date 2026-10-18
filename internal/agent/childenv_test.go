package agent

import (
	"strings"
	"testing"
)

func TestChildEnvExtraWins(t *testing.T) {
	t.Setenv("CLAUDE_CODE_GIT_BASH_PATH", "inherited")
	t.Setenv("TELEGRAM_BOT_TOKEN", "secret")
	env := ChildEnv("/x", 0, "CLAUDE_CODE_GIT_BASH_PATH=from-settings")
	last := ""
	for _, kv := range env {
		if strings.HasPrefix(kv, "TELEGRAM_BOT_TOKEN=") {
			t.Fatal("the bot token must never reach claude")
		}
		if v, ok := strings.CutPrefix(kv, "CLAUDE_CODE_GIT_BASH_PATH="); ok {
			last = v
		}
	}
	if last != "from-settings" {
		t.Fatalf("the setting must come last, got %q", last)
	}
}
