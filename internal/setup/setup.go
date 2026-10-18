// Package setup holds the first-run logic shared by `clawdyx setup` and the
// desktop app's wizard: token checks, owner-ID detection, workspace checks,
// config generation and private file writes.
package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-telegram/bot"

	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/observability"
)

// APIBase is the Bot API base URL (tests replace it).
var APIBase = "https://api.telegram.org"

// GetMe checks a token with Telegram and returns the bot's username.
func GetMe(ctx context.Context, token string) (string, error) {
	if !config.ValidToken(token) {
		return "", errors.New("that does not look like a Telegram bot token (123456789:AAAA…, 35 characters after the colon)")
	}
	b, err := bot.New(token, bot.WithSkipGetMe(), bot.WithServerURL(APIBase))
	if err != nil {
		return "", errors.New(observability.Redact(err.Error()))
	}
	me, err := b.GetMe(ctx)
	if err != nil {
		return "", errors.New(observability.Redact(err.Error()))
	}
	return me.Username, nil
}

type tgUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

type tgUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		From *tgUser `json:"from"`
		Chat struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		} `json:"chat"`
	} `json:"message"`
}

// Sender is someone who messaged the bot in a private chat.
type Sender struct {
	ID        int64  `json:"id"`
	FirstName string `json:"firstName"`
	Username  string `json:"username,omitempty"`
}

func getUpdates(ctx context.Context, token string, offset int64) ([]tgUpdate, error) {
	params := map[string]any{"timeout": 0, "allowed_updates": []string{"message"}}
	if offset > 0 {
		params["offset"] = offset
	}
	body, _ := json.Marshal(params)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, APIBase+"/bot"+token+"/getUpdates", bytes.NewReader(body))
	if err != nil {
		return nil, errors.New(observability.Redact(err.Error()))
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.New(observability.Redact(err.Error()))
	}
	defer resp.Body.Close()
	var out struct {
		OK          bool       `json:"ok"`
		Result      []tgUpdate `json:"result"`
		Description string     `json:"description"`
		ErrorCode   int        `json:"error_code"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("telegram: unreadable response (HTTP %d)", resp.StatusCode)
	}
	if !out.OK {
		if out.ErrorCode == 409 {
			return nil, errors.New("another program is reading this bot's messages (is the clawdyX worker running?); stop it first")
		}
		return nil, fmt.Errorf("telegram: %s (error %d)", observability.Redact(out.Description), out.ErrorCode)
	}
	return out.Result, nil
}

// DetectSenders returns the private-chat senders of pending updates and
// acknowledges those updates so they are not replayed as prompts later.
// It must not run while a worker polls the same bot.
func DetectSenders(ctx context.Context, token string) ([]Sender, int, error) {
	ups, err := getUpdates(ctx, token, 0)
	if err != nil {
		return nil, 0, err
	}
	seen := map[int64]Sender{}
	var maxID int64
	for _, u := range ups {
		if u.UpdateID > maxID {
			maxID = u.UpdateID
		}
		m := u.Message
		if m == nil || m.From == nil || m.From.IsBot || m.Chat.Type != "private" || m.Chat.ID != m.From.ID {
			continue
		}
		if _, ok := seen[m.From.ID]; !ok {
			seen[m.From.ID] = Sender{ID: m.From.ID, FirstName: observability.Truncate(m.From.FirstName, 40), Username: m.From.Username}
		}
	}
	out := make([]Sender, 0, len(seen))
	for _, s := range seen {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if maxID > 0 {
		_, _ = getUpdates(ctx, token, maxID+1)
	}
	return out, len(ups), nil
}

// WorkspaceOK checks a candidate project folder.
func WorkspaceOK(p string) error {
	if p == "" {
		return errors.New("a project folder is required")
	}
	if !filepath.IsAbs(p) {
		return fmt.Errorf("%s is not an absolute path", p)
	}
	st, err := os.Stat(p)
	if err != nil {
		return fmt.Errorf("%v", err)
	}
	if !st.IsDir() {
		return fmt.Errorf("%s is not a directory", p)
	}
	canon, err := filepath.EvalSymlinks(p)
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	if filepath.Dir(canon) == canon || canon == filepath.Clean(home) {
		return errors.New("the home folder and the filesystem root cannot be workspaces; pick a project folder such as ~/projects/<name>")
	}
	return nil
}

// PrivacyWarning flags folders that the OS can hide from a background
// worker (macOS privacy protection); "" when there is nothing to say.
func PrivacyWarning(p string) string {
	home, _ := os.UserHomeDir()
	return privacyWarning(home, p)
}

// DetectClaude finds the claude CLI (PATH first, then the usual places).
func DetectClaude() string {
	if p, err := exec.LookPath("claude"); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
		return p
	}
	home, _ := os.UserHomeDir()
	return firstFile(claudeCandidates(home))
}

func firstFile(candidates []string) string {
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

var wsNameClean = regexp.MustCompile(`[^a-z0-9_-]+`)

// WorkspaceName derives a config-safe workspace name from a folder path.
func WorkspaceName(path string) string {
	n := strings.ToLower(filepath.Base(filepath.Clean(path)))
	n = wsNameClean.ReplaceAllString(n, "-")
	n = strings.Trim(n, "-_")
	if len(n) > 32 {
		n = strings.TrimRight(n[:32], "-_")
	}
	if n == "" {
		return "workspace"
	}
	return n
}

// DefaultAllowed and DefaultDenied are the command rules for new workspaces.
var (
	DefaultAllowed = []string{"Bash(git status *)", "Bash(git diff *)", "Bash(git log *)", "Bash(make test *)", "Bash(go test *)", "Bash(go build *)", "Bash(go vet *)"}
	DefaultDenied  = []string{"Bash(rm *)", "Bash(rmdir *)", "Bash(sudo *)", "Bash(git push *)", "Bash(git remote *)", "Bash(curl *)", "Bash(wget *)", "Bash(brew *)", "Bash(claude *)"}
)

// NewWorkspace returns a workspace with the default rules.
func NewWorkspace(name, path string) config.Workspace {
	return config.Workspace{Name: name, Path: path, DefaultMode: config.ModeObserve,
		AllowedCommands: append([]string(nil), DefaultAllowed...), DeniedCommands: append([]string(nil), DefaultDenied...)}
}

// GenerateConfig builds the default configuration for one workspace.
func GenerateConfig(owner int64, wsPath, browser, claudeBin string) *config.Config {
	open := true
	name := WorkspaceName(wsPath)
	childPath := config.DefaultChildPath()
	sep := string(os.PathListSeparator)
	if dir := filepath.Dir(claudeBin); !strings.Contains(sep+childPath+sep, sep+dir+sep) {
		childPath = dir + sep + childPath
	}
	return &config.Config{
		Telegram: config.Telegram{OwnerUserID: owner, PollTimeoutSeconds: 30},
		Claude: config.Claude{
			Binary: claudeBin, DefaultModel: "fable", DefaultEffort: "auto", SettingSources: "user", StrictMCPConfig: true,
			JobTimeout: config.Duration{Duration: 15 * time.Minute}, MaxTurns: 60, MaxConcurrentJobs: 2, MaxQueuePerSession: 5,
			MaxOutputBytes: 1 << 20, MaxChildRSSMB: 2048, BgWaitCeilingMS: 60000, ChildPath: childPath, ExtraDisallowedTools: []string{},
		},
		Workspaces:       []config.Workspace{NewWorkspace(name, wsPath)},
		DefaultWorkspace: name,
		Desktop:          config.Desktop{OpenURLs: &open, BrowserApp: browser},
		Logging:          config.Logging{Level: "info"},
		Retention:        config.Retention{JobsDays: 90, JobEventsDays: 14, ProcessedUpdatesDays: 7, AuditDays: 180},
	}
}

// WriteConfig validates c and writes it privately (0600 in a 0700 folder).
func WriteConfig(path string, c *config.Config) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if _, err := config.Parse(data); err != nil {
		return err
	}
	return WritePrivate(path, data)
}

// WritePrivate writes data to path atomically with mode 0600 inside a 0700 directory.
func WritePrivate(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	_ = os.Chmod(dir, 0o700)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if err := tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(data)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Rename(name, path)
}

// ExpandHome expands a leading ~/.
func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
