package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// CLI wraps read-only helper invocations of the claude binary.
type CLI struct {
	Binary string
	Env    []string
	Dir    string // neutral cwd for helper commands (the bot's state dir)
}

var versionRe = regexp.MustCompile(`^(\d+\.\d+\.\d+)`)

// Version returns the CLI version (e.g. "2.1.292").
func (c CLI) Version(ctx context.Context) (string, error) {
	out, _, code, err := RunCapture(ctx, 15*time.Second, c.Dir, c.Env, c.Binary, "--version")
	if err != nil {
		return "", err
	}
	m := versionRe.FindStringSubmatch(strings.TrimSpace(out))
	if code != 0 || m == nil {
		return "", fmt.Errorf("unexpected version output")
	}
	return m[1], nil
}

// AuthInfo is the non-identifying subset of `claude auth status`. Email,
// organization ID and name are deliberately not decoded.
type AuthInfo struct {
	LoggedIn         bool   `json:"loggedIn"`
	AuthMethod       string `json:"authMethod"`
	APIProvider      string `json:"apiProvider"`
	SubscriptionType string `json:"subscriptionType"`
}

// AuthStatus runs `claude auth status` (slow: 20-40 s observed).
func (c CLI) AuthStatus(ctx context.Context) (AuthInfo, error) {
	out, _, code, err := RunCapture(ctx, 90*time.Second, c.Dir, c.Env, c.Binary, "auth", "status")
	if err != nil {
		return AuthInfo{}, err
	}
	var a AuthInfo
	if err := json.Unmarshal([]byte(out), &a); err != nil {
		return AuthInfo{}, fmt.Errorf("unparseable auth status (exit %d)", code)
	}
	return a, nil
}

// AgentEntry is one row of `claude agents --json`.
type AgentEntry struct {
	PID       int    `json:"pid"`
	CWD       string `json:"cwd"`
	Kind      string `json:"kind"`
	StartedAt int64  `json:"startedAt"`
	SessionID string `json:"sessionId"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	State     string `json:"state"`
}

// Agents runs `claude agents --json` (slow: 10-15 s observed).
func (c CLI) Agents(ctx context.Context) ([]AgentEntry, error) {
	out, _, code, err := RunCapture(ctx, 60*time.Second, c.Dir, c.Env, c.Binary, "agents", "--json")
	if err != nil {
		return nil, err
	}
	var a []AgentEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &a); err != nil {
		return nil, fmt.Errorf("unparseable agents list (exit %d)", code)
	}
	return a, nil
}

// Doctor runs `claude doctor` (read-only) and returns its text.
func (c CLI) Doctor(ctx context.Context) (string, error) {
	out, errOut, _, err := RunCapture(ctx, 60*time.Second, c.Dir, c.Env, c.Binary, "doctor")
	if err != nil {
		return "", err
	}
	return stripANSI(out + errOut), nil
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// ProbeResult is the outcome of a model validation probe.
type ProbeResult struct {
	OK        bool
	Canonical string // resolved model ID, e.g. claude-fable-5-1
	Reason    string // user-facing reason when !OK
}

// ErrProbeInfra means the probe could not run (CLI/auth problem), as opposed
// to the model being rejected.
var ErrProbeInfra = errors.New("probe failed to run")

// ProbeModel validates a model name with one minimal, non-persisted turn.
func (c CLI) ProbeModel(ctx context.Context, model string, settingSources string) (ProbeResult, error) {
	args := []string{"-p", "--model", model, "--max-turns", "1", "--no-session-persistence",
		"--output-format", "json", "--permission-mode", "dontAsk", "--permission-prompts", "none", "--strict-mcp-config"}
	if settingSources != "" {
		args = append(args, "--setting-sources", settingSources)
	}
	args = append(args, "--tools", "")
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	out, errOut, code, err := runWithStdin(ctx, c, "Reply with exactly: OK", args...)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("%w: %v", ErrProbeInfra, err)
	}
	var r Result
	if jerr := json.Unmarshal([]byte(strings.TrimSpace(lastLine(out))), &r); jerr != nil {
		return ProbeResult{}, fmt.Errorf("%w: exit %d, %s", ErrProbeInfra, code, firstLine(errOut))
	}
	if strings.Contains(errOut, "unrecognized_model") || (r.APIErrorStatus != nil && *r.APIErrorStatus == 404) {
		return ProbeResult{Reason: "the CLI or account rejected this model (not found or no access)"}, nil
	}
	if r.IsError {
		reason := r.Text()
		if reason == "" {
			reason = r.TerminalReason
		}
		if r.APIErrorStatus != nil && (*r.APIErrorStatus == 401 || *r.APIErrorStatus == 403) {
			return ProbeResult{}, fmt.Errorf("%w: authentication error", ErrProbeInfra)
		}
		return ProbeResult{Reason: reason}, nil
	}
	if len(r.ModelUsage) != 1 {
		return ProbeResult{Reason: "could not determine which model answered"}, nil
	}
	for k := range r.ModelUsage {
		return ProbeResult{OK: true, Canonical: k}, nil
	}
	return ProbeResult{}, nil
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
