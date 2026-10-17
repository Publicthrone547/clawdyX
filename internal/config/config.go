// Package config loads and validates the bot configuration. Validation fails
// closed: any missing, malformed, or unexpectedly broad value aborts startup.
package config

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Modes of a session.
const (
	ModeObserve    = "observe"
	ModeAssisted   = "assisted"
	ModeAutomation = "automation"
)

// EffortLevels accepted by the installed CLI (verified on 2.1.292) plus "auto".
var EffortLevels = []string{"low", "medium", "high", "xhigh", "max", "auto"}

// Duration is a time.Duration that unmarshals from strings like "15m".
type Duration struct{ time.Duration }

// UnmarshalJSON implements json.Unmarshaler.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("duration must be a string like \"15m\": %w", err)
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

// MarshalJSON implements json.Marshaler.
func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// Telegram settings.
type Telegram struct {
	// OwnerUserID is the only Telegram user the bot serves. It is set locally
	// (clawdyx setup) and must be a positive numeric user ID; the bot fails
	// closed when it is missing. Usernames and lists are never accepted.
	OwnerUserID        int64 `json:"owner_user_id"`
	PollTimeoutSeconds int   `json:"poll_timeout_seconds"`
}

// Claude CLI settings.
type Claude struct {
	Binary               string   `json:"binary"`
	DefaultModel         string   `json:"default_model"`
	DefaultEffort        string   `json:"default_effort"`
	SettingSources       string   `json:"setting_sources"`
	StrictMCPConfig      bool     `json:"strict_mcp_config"`
	JobTimeout           Duration `json:"job_timeout"`
	MaxTurns             int      `json:"max_turns"`
	MaxBudgetUSD         float64  `json:"max_budget_usd"`
	MaxConcurrentJobs    int      `json:"max_concurrent_jobs"`
	MaxQueuePerSession   int      `json:"max_queue_per_session"`
	MaxOutputBytes       int      `json:"max_output_bytes"`
	MaxChildRSSMB        int      `json:"max_child_rss_mb"`
	BgWaitCeilingMS      int      `json:"bg_wait_ceiling_ms"`
	ChildPath            string   `json:"child_path"`
	ExtraDisallowedTools []string `json:"extra_disallowed_tools"`
}

// Workspace is an allowed project root.
type Workspace struct {
	Name            string   `json:"name"`
	Path            string   `json:"path"`
	DefaultMode     string   `json:"default_mode"`
	AllowedCommands []string `json:"allowed_commands"`
	DeniedCommands  []string `json:"denied_commands"`
	// IKnowWhatIAmDoing disables the minimum deny-list check for this workspace.
	IKnowWhatIAmDoing bool `json:"i_know_what_i_am_doing,omitempty"`

	// Canonical is the symlink-resolved absolute path, filled by Validate.
	Canonical string `json:"-"`
}

// Modes gates the permission tiers beyond observe/assisted.
type Modes struct {
	// AutomationEnabled allows /mode automation (Claude Code auto mode: a
	// safety classifier approves commands instead of a fixed allowlist).
	AutomationEnabled bool `json:"automation_enabled"`
}

// Desktop controls owner-typed desktop actions (not available to Claude).
type Desktop struct {
	// OpenURLs enables /open; nil means the default (enabled).
	OpenURLs *bool `json:"open_urls"`
	// BrowserApp is the macOS application used by /open.
	BrowserApp string `json:"browser_app"`
	// ScreenControl lets automation-tier sessions see the main display and
	// use the mouse and keyboard (MCP tools mcp__screen__*), and enables
	// /screenshot. Requires macOS Screen Recording and Accessibility grants.
	ScreenControl bool `json:"screen_control"`
}

// OpenEnabled reports whether /open is enabled.
func (d Desktop) OpenEnabled() bool { return d.OpenURLs == nil || *d.OpenURLs }

// Logging settings.
type Logging struct {
	Level string `json:"level"`
}

// Retention settings in days.
type Retention struct {
	JobsDays             int `json:"jobs_days"`
	JobEventsDays        int `json:"job_events_days"`
	ProcessedUpdatesDays int `json:"processed_updates_days"`
	AuditDays            int `json:"audit_days"`
}

// Config is the full configuration.
type Config struct {
	Telegram               Telegram    `json:"telegram"`
	Claude                 Claude      `json:"claude"`
	Workspaces             []Workspace `json:"workspaces"`
	DefaultWorkspace       string      `json:"default_workspace"`
	ShutdownCommandEnabled bool        `json:"shutdown_command_enabled"`
	Desktop                Desktop     `json:"desktop"`
	Modes                  Modes       `json:"modes"`
	StateDir               string      `json:"state_dir"`
	LogDir                 string      `json:"log_dir"`
	Logging                Logging     `json:"logging"`
	Retention              Retention   `json:"retention"`

	// Token is loaded separately from the environment or the .env file.
	Token string `json:"-"`
}

// MinimumDenied are deny rules every workspace must contain unless it opts out.
var MinimumDenied = []string{"Bash(rm *)", "Bash(sudo *)", "Bash(git push *)"}

var (
	tokenRe = regexp.MustCompile(`^[0-9]{8,10}:[A-Za-z0-9_-]{35}$`)
	nameRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	// appNameRe allows names like "Firefox" or "Google Chrome"; no paths, no leading dash.
	appNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._()]{0,63}$`)
)

// DefaultPath returns the default config path.
func DefaultPath() string {
	if p := os.Getenv("CLAWDYX_CONFIG"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "clawdyx", "config.json")
}

// DefaultEnvPath returns the default secrets file path.
func DefaultEnvPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "clawdyx", ".env")
}

func applyDefaults(c *Config) {
	home, _ := os.UserHomeDir()
	if c.Telegram.PollTimeoutSeconds == 0 {
		c.Telegram.PollTimeoutSeconds = 30
	}
	cl := &c.Claude
	if cl.Binary == "" {
		cl.Binary = "/opt/homebrew/bin/claude"
	}
	if cl.DefaultModel == "" {
		cl.DefaultModel = "fable"
	}
	if cl.DefaultEffort == "" {
		cl.DefaultEffort = "auto"
	}
	if cl.JobTimeout.Duration == 0 {
		cl.JobTimeout.Duration = 15 * time.Minute
	}
	if cl.MaxTurns == 0 {
		cl.MaxTurns = 60
	}
	if cl.MaxConcurrentJobs == 0 {
		cl.MaxConcurrentJobs = 2
	}
	if cl.MaxQueuePerSession == 0 {
		cl.MaxQueuePerSession = 5
	}
	if cl.MaxOutputBytes == 0 {
		cl.MaxOutputBytes = 1 << 20
	}
	if cl.MaxChildRSSMB == 0 {
		cl.MaxChildRSSMB = 2048
	}
	if cl.BgWaitCeilingMS == 0 {
		cl.BgWaitCeilingMS = 60000
	}
	if cl.ChildPath == "" {
		cl.ChildPath = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
	}
	for i := range c.Workspaces {
		if c.Workspaces[i].DefaultMode == "" {
			c.Workspaces[i].DefaultMode = ModeObserve
		}
	}
	if c.StateDir == "" {
		c.StateDir = filepath.Join(home, "Library", "Application Support", "clawdyx")
	}
	if c.LogDir == "" {
		c.LogDir = filepath.Join(home, "Library", "Logs", "clawdyx")
	}
	if c.Desktop.BrowserApp == "" {
		c.Desktop.BrowserApp = "Firefox"
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	r := &c.Retention
	if r.JobsDays == 0 {
		r.JobsDays = 90
	}
	if r.JobEventsDays == 0 {
		r.JobEventsDays = 14
	}
	if r.ProcessedUpdatesDays == 0 {
		r.ProcessedUpdatesDays = 7
	}
	if r.AuditDays == 0 {
		r.AuditDays = 180
	}
}

// Parse decodes and validates a config document (without the token).
func Parse(data []byte) (*Config, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var c Config
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	applyDefaults(&c)
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Load reads the config file and the token. Both files must be private (0600).
func Load(configPath, envPath string) (*Config, error) {
	if err := checkPrivate(configPath); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", configPath, err)
	}
	c, err := Parse(data)
	if err != nil {
		return nil, err
	}
	tok, err := LoadToken(envPath)
	if err != nil {
		return nil, err
	}
	c.Token = tok
	return c, nil
}

// LoadToken reads TELEGRAM_BOT_TOKEN from the process environment, falling back
// to the given .env file. The value is validated but never echoed.
func LoadToken(envPath string) (string, error) {
	tok := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if tok == "" && envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			if err := checkPrivate(envPath); err != nil {
				return "", err
			}
			vals, err := readEnvFile(envPath)
			if err != nil {
				return "", err
			}
			tok = vals["TELEGRAM_BOT_TOKEN"]
		}
	}
	if tok == "" {
		return "", errors.New("config: TELEGRAM_BOT_TOKEN is not set (environment or .env file)")
	}
	if !tokenRe.MatchString(tok) {
		return "", errors.New("config: TELEGRAM_BOT_TOKEN has an invalid format")
	}
	return tok, nil
}

func readEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("config: open env file: %w", err)
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if uq, err := strconv.Unquote(v); err == nil {
			v = uq
		} else {
			v = strings.Trim(v, "'")
		}
		out[strings.TrimSpace(k)] = v
	}
	return out, sc.Err()
}

func checkPrivate(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if st.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("config: %s must not be readable by group or others (chmod 600)", path)
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (c *Config) validate() error {
	var errs []string
	add := func(f string, a ...any) { errs = append(errs, fmt.Sprintf(f, a...)) }

	if c.Telegram.OwnerUserID <= 0 {
		add("telegram.owner_user_id must be your positive numeric Telegram user ID (run `clawdyx setup`)")
	}
	if c.Telegram.PollTimeoutSeconds < 5 || c.Telegram.PollTimeoutSeconds > 50 {
		add("telegram.poll_timeout_seconds must be 5..50")
	}

	cl := c.Claude
	if !filepath.IsAbs(cl.Binary) {
		add("claude.binary must be an absolute path")
	} else if st, err := os.Stat(cl.Binary); err != nil {
		add("claude.binary %s: %v", cl.Binary, err)
	} else if st.IsDir() || st.Mode().Perm()&0o111 == 0 {
		add("claude.binary %s is not an executable file", cl.Binary)
	}
	if strings.TrimSpace(cl.DefaultModel) == "" {
		add("claude.default_model must not be empty")
	}
	if !contains(EffortLevels, cl.DefaultEffort) {
		add("claude.default_effort must be one of %v", EffortLevels)
	}
	if cl.SettingSources != "" {
		for _, s := range strings.Split(cl.SettingSources, ",") {
			if !contains([]string{"user", "project", "local"}, strings.TrimSpace(s)) {
				add("claude.setting_sources contains invalid source %q", s)
			}
		}
	}
	if cl.JobTimeout.Duration < 30*time.Second || cl.JobTimeout.Duration > 2*time.Hour {
		add("claude.job_timeout must be between 30s and 2h")
	}
	if cl.MaxTurns < 1 || cl.MaxTurns > 500 {
		add("claude.max_turns must be 1..500")
	}
	if cl.MaxBudgetUSD != 0 && cl.MaxBudgetUSD < 2 {
		add("claude.max_budget_usd must be 0 (off) or >= 2")
	}
	if cl.MaxConcurrentJobs < 1 || cl.MaxConcurrentJobs > 4 {
		add("claude.max_concurrent_jobs must be 1..4")
	}
	if cl.MaxQueuePerSession < 1 || cl.MaxQueuePerSession > 50 {
		add("claude.max_queue_per_session must be 1..50")
	}
	if cl.MaxOutputBytes < 4096 || cl.MaxOutputBytes > 16<<20 {
		add("claude.max_output_bytes must be 4096..16777216")
	}
	if cl.MaxChildRSSMB < 256 {
		add("claude.max_child_rss_mb must be >= 256")
	}
	for _, p := range strings.Split(cl.ChildPath, ":") {
		if !filepath.IsAbs(p) {
			add("claude.child_path entries must be absolute (%q)", p)
		}
	}

	if len(c.Workspaces) == 0 {
		add("at least one workspace is required")
	}
	seenName, seenPath := map[string]bool{}, map[string]bool{}
	for i := range c.Workspaces {
		w := &c.Workspaces[i]
		if !nameRe.MatchString(w.Name) {
			add("workspace name %q must match %s", w.Name, nameRe)
		}
		if seenName[w.Name] {
			add("duplicate workspace name %q", w.Name)
		}
		seenName[w.Name] = true
		if !contains([]string{ModeObserve, ModeAssisted}, w.DefaultMode) {
			add("workspace %q default_mode must be observe or assisted", w.Name)
		}
		if !filepath.IsAbs(w.Path) {
			add("workspace %q path must be absolute", w.Name)
			continue
		}
		canon, err := filepath.EvalSymlinks(w.Path)
		if err != nil {
			add("workspace %q path: %v", w.Name, err)
			continue
		}
		canon = filepath.Clean(canon)
		st, err := os.Stat(canon)
		if err != nil || !st.IsDir() {
			add("workspace %q path is not a directory", w.Name)
			continue
		}
		if canon == "/" {
			add("workspace %q must not be the filesystem root", w.Name)
		}
		if home, _ := os.UserHomeDir(); canon == filepath.Clean(home) {
			add("workspace %q must not be the home directory itself", w.Name)
		}
		if seenPath[canon] {
			add("workspace %q duplicates another workspace path", w.Name)
		}
		seenPath[canon] = true
		w.Canonical = canon
		if !w.IKnowWhatIAmDoing {
			for _, m := range MinimumDenied {
				if !contains(w.DeniedCommands, m) {
					add("workspace %q denied_commands must include %s", w.Name, m)
				}
			}
		}
		for _, r := range append(append([]string{}, w.AllowedCommands...), w.DeniedCommands...) {
			if strings.ContainsAny(r, "\n\r") || !strings.HasSuffix(r, ")") || !strings.Contains(r, "(") {
				add("workspace %q has malformed permission rule %q", w.Name, r)
			}
		}
	}
	if c.DefaultWorkspace == "" || !seenName[c.DefaultWorkspace] {
		add("default_workspace must name a configured workspace")
	}
	if !filepath.IsAbs(c.StateDir) || !filepath.IsAbs(c.LogDir) {
		add("state_dir and log_dir must be absolute")
	}
	if !appNameRe.MatchString(c.Desktop.BrowserApp) {
		add("desktop.browser_app must be a plain application name like \"Firefox\"")
	}
	if !contains([]string{"debug", "info", "warn", "error"}, c.Logging.Level) {
		add("logging.level must be debug, info, warn or error")
	}

	if len(errs) > 0 {
		return errors.New("config invalid:\n  - " + strings.Join(errs, "\n  - "))
	}
	return nil
}

// Workspace returns the workspace with the given name.
func (c *Config) Workspace(name string) (*Workspace, bool) {
	for i := range c.Workspaces {
		if c.Workspaces[i].Name == name {
			return &c.Workspaces[i], true
		}
	}
	return nil, false
}

// IsEffort reports whether s is an accepted effort value.
func IsEffort(s string) bool { return contains(EffortLevels, s) }

// ValidToken reports whether s has the shape of a Telegram bot token. The
// value is never logged or echoed by callers.
func ValidToken(s string) bool { return tokenRe.MatchString(strings.TrimSpace(s)) }
