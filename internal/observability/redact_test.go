package observability

import (
	"bytes"
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	tok := "123456789:" + strings.Repeat("a", 35)
	cases := []struct{ in, mustNot, mustHave string }{
		{"token " + tok + " end", tok, "<redacted>"},
		{"key sk-ant-api03-abcdefghijklmnop", "abcdefghijklmnop", "sk-ant-<redacted>"},
		{"Authorization: Bearer abc.def.ghijklmnop", "ghijklmnop", "<redacted>"},
		{`{"api_key": "supersecretvalue"}`, "supersecretvalue", "api_key"},
		{"password=hunter2hunter2", "hunter2hunter2", "password=<redacted>"},
		{"mail me at someone@example.com", "someone@example.com", "<email>"},
		{"AKIAABCDEFGHIJKLMNOP", "ABCDEFGHIJKLMNOP", "AKIA<redacted>"},
	}
	for _, c := range cases {
		got := Redact(c.in)
		if strings.Contains(got, c.mustNot) || !strings.Contains(got, c.mustHave) {
			t.Errorf("Redact(%q) = %q", c.in, got)
		}
	}
	if Redact("plain text 12:30") != "plain text 12:30" {
		t.Error("benign text changed")
	}
}

func TestLoggerRedacts(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(&buf, "info")
	l.Info("x", "detail", "TELEGRAM_BOT_TOKEN=123456789:"+strings.Repeat("Z", 35))
	if strings.Contains(buf.String(), strings.Repeat("Z", 35)) {
		t.Fatalf("token leaked: %s", buf.String())
	}
}

func TestTruncate(t *testing.T) {
	if Truncate("héllo", 3) != "hé…" || Truncate("ab", 5) != "ab" {
		t.Fatal("truncate wrong")
	}
}
