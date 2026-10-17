// Package observability provides redaction, structured logging, and counters.
package observability

import (
	"io"
	"regexp"
	"strings"
	"sync"
)

type rule struct {
	re   *regexp.Regexp
	repl func(m string) string
}

func keep2(prefix string) func(string) string {
	return func(m string) string { return prefix + "<redacted>" }
}

var rules = []rule{
	// Telegram bot tokens.
	{regexp.MustCompile(`[0-9]{8,10}:[A-Za-z0-9_-]{35}`), keep2("")},
	// Anthropic keys and OAuth tokens.
	{regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]{8,}`), keep2("sk-ant-")},
	// AWS access key IDs.
	{regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`), keep2("AKIA")},
	// Bearer / Authorization headers.
	{regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{8,}`), keep2("Bearer ")},
	// key=value or key: value secrets.
	{regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|auth[_-]?token|token|secret|password|passwd|authorization)(["']?\s*[:=]\s*["']?)([^\s"',;]{4,})`), nil},
	// Email addresses.
	{regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`), func(string) string { return "<email>" }},
}

var kvRule = rules[4].re

// Redact removes credential-like substrings and email addresses.
func Redact(s string) string {
	if s == "" {
		return s
	}
	for i, r := range rules {
		if i == 4 {
			s = kvRule.ReplaceAllString(s, "${1}${2}<redacted>")
			continue
		}
		s = r.re.ReplaceAllStringFunc(s, r.repl)
	}
	return s
}

// RedactingWriter wraps a writer and redacts every write. Writes are assumed to
// be whole log lines, which holds for slog handlers.
type RedactingWriter struct {
	mu sync.Mutex
	w  io.Writer
}

// NewRedactingWriter returns a redacting writer.
func NewRedactingWriter(w io.Writer) *RedactingWriter { return &RedactingWriter{w: w} }

func (r *RedactingWriter) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := Redact(string(p))
	if _, err := io.WriteString(r.w, out); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Truncate shortens s to at most n runes, adding an ellipsis.
func Truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

// OneLine collapses whitespace so values fit on one line.
func OneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
