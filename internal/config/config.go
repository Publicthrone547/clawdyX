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

	"github.com/Publicthrone547/clawdyX/internal/platform"
	"github.com/Publicthrone547/clawdyX/internal/vault"
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
	// OwnerUserID is the owner's numeric Telegram user ID. It is set locally
	// (clawdyx setup or the app) and must be positive; the bot fails closed
	// when no owner is configured. Usernames are never accepted.
	OwnerUserID int64 `json:"owner_user_id,omitempty"`
	// AllowedUsers lists every numeric ID the bot serves, with exactly one
	// owner. When empty it is derived from OwnerUserID.
	AllowedUsers       []AllowedUser `json:"allowed_users,omitempty"`
	PollTimeoutSeconds int           `json:"poll_timeout_seconds"`
}

// Roles of allowed Telegram users.
const (
	RoleOwner = "owner"
	RoleUser  = "user"
)

// MaxAllowedUsers bounds the allowlist.
const MaxAllowedUsers = 20

// AllowedUser is one numeric Telegram user ID with a local label and role.
// The label is only a reminder for the owner; it never authorizes anything.
type AllowedUser struct {
	ID    int64  `json:"id"`
	Label string `json:"label,omitempty"`
	Role  string `json:"role"`
}

var labelRe = regexp.MustCompile(`^[\p{L}\p{N} ._'@()-]{0,32}$`)

// NormalizeAllowlist validates the allowlist (and the legacy owner field)
// and returns it with the owner first. It fails closed.
func NormalizeAllowlist(owner int64, users []AllowedUser) ([]AllowedUser, error) {
	if len(users) == 0 {
		if owner <= 0 {
			return nil, errors.New("telegram: no owner configured; set your numeric Telegram user ID (run `clawdyx setup`)")
		}
		return []AllowedUser{{ID: owner, Role: RoleOwner}}, nil
	}
	if len(users) > MaxAllowedUsers {
		return nil, fmt.Errorf("telegram.allowed_users: at most %d entries", MaxAllowedUsers)
	}
	seen := map[int64]bool{}
	var out []AllowedUser
	var ownerEntry *AllowedUser
	for i := range users {
		u := users[i]
		u.Label = strings.TrimSpace(u.Label)
		if u.ID <= 0 {
			return nil, fmt.Errorf("telegram.allowed_users: %d is not a positive numeric user ID", u.ID)
		}
		if seen[u.ID] {
			return nil, fmt.Errorf("telegram.allowed_users: %d is listed twice", u.ID)
		}
		seen[u.ID] = true
		if !labelRe.MatchString(u.Label) {
			return nil, fmt.Errorf("telegram.allowed_users: label %q must be at most 32 letters, digits, spaces or ._'@()-", u.Label)
		}
		switch u.Role {
		case RoleOwner:
			if ownerEntry != nil {
				return nil, errors.New("telegram.allowed_users: exactly one owner is allowed")
			}
			ownerEntry = &u
		case RoleUser:
			out = append(out, u)
		default:
			return nil, fmt.Errorf("telegram.allowed_users: role of %d must be owner or user", u.ID)
		}
	}
	if ownerEntry == nil {
		return nil, errors.New("telegram.allowed_users: one entry must have role owner")
	}
	if owner > 0 && owner != ownerEntry.ID {
		return nil, errors.New("telegram: owner_user_id and the owner in allowed_users differ")
	}
	return append([]AllowedUser{*ownerEntry}, out...), nil
}

// Role returns the role of a Telegram user ID ("" if not allowed).
func (t Telegram) Role(id int64) string {
	for _, u := range t.AllowedUsers {
		if u.ID == id {
			return u.Role
		}
	}
	return ""
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
	// GitBashPath is the bash.exe of Git for Windows that Claude Code's
	// shell tool uses (CLAUDE_CODE_GIT_BASH_PATH). Empty: Claude Code finds
	// it. Windows only; ignored elsewhere.
	GitBashPath string `json:"git_bash_path,omitempty"`
}

// ChildEnvExtra are the variables the config adds to every claude child.
func (cl Claude) ChildEnvExtra() []string {
	if cl.GitBashPath == "" || !gitBashUsed {
		return nil
	}
	return []string{"CLAUDE_CODE_GIT_BASH_PATH=" + cl.GitBashPath}
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
	// Camera enables the owner's /cameras and /camera commands (photos and
	// clips sent to the owner's private chat). Claude never gets a camera
	// tool. /screen off switches the camera off too. macOS only; needs the
	// Camera permission for clawdyX.
	Camera bool `json:"camera,omitempty"`
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
	// TokenSource says where Token came from: "env", "file" or "keychain"
	// (the system token store of any OS).
	TokenSource string `json:"-"`
	// NoKeyring is set when the OS has no usable token store (a Linux
	// server without a Secret Service) and the token file was used.
	NoKeyring bool `json:"-"`
}

// MinimumDenied are deny rules every workspace must contain unless it opts out.
var MinimumDenied = []string{"Bash(rm *)", "Bash(sudo *)", "Bash(git push *)"}

var (
	tokenRe = regexp.MustCompile(`^[0-9]{8,10}:[A-Za-z0-9_-]{35}$`)
	nameRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	// appNameRe allows names like "Firefox" or "Google Chrome"; no paths, no leading dash.
	appNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._()]{0,63}$`)
)

// DefaultPath returns the default config path: CLAWDYX_CONFIG when set,
// else config.json in the OS's settings folder (~/.config/clawdyx on macOS,
// $XDG_CONFIG_HOME/clawdyx on Linux, %APPDATA%\clawdyx on Windows). Until
// MigrateDefault has moved an earlier release's file there, that file is
// used where it is.
func DefaultPath() string {
	if p := os.Getenv("CLAWDYX_CONFIG"); p != "" {
		return p
	}
	std := StandardPath()
	if old := LegacyPath(); old != "" && !exists(std) && exists(old) {
		return old
	}
	return std
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// StandardPath is the OS's default config path, ignoring CLAWDYX_CONFIG.
func StandardPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(configDir(home), "config.json")
}

// DefaultEnvPath returns the default secrets file path: .env next to the
// default config file.
func DefaultEnvPath() string { return EnvPathFor(DefaultPath()) }

// EnvPathFor is the .env file that belongs to a config file.
func EnvPathFor(configPath string) string { return filepath.Join(filepath.Dir(configPath), ".env") }

// LegacyPath is where an earlier release kept the config on this OS, or ""
// when it never moved.
func LegacyPath() string {
	home, _ := os.UserHomeDir()
	return legacyConfigPath(home)
}

func applyDefaults(c *Config) {
	home, _ := os.UserHomeDir()
	if c.Telegram.PollTimeoutSeconds == 0 {
		c.Telegram.PollTimeoutSeconds = 30
	}
	cl := &c.Claude
	if cl.Binary == "" {
		cl.Binary = defaultClaudeBinary(home)
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
		cl.ChildPath = DefaultChildPath()
	}
	for i := range c.Workspaces {
		if c.Workspaces[i].DefaultMode == "" {
			c.Workspaces[i].DefaultMode = ModeObserve
		}
	}
	if c.StateDir == "" {
		c.StateDir = defaultStateDir(home)
	}
	if c.LogDir == "" {
		c.LogDir = defaultLogDir(home)
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

// TokenVault is where the bot token is kept (the system token store by
// default: Keychain, Credential Manager or Secret Service). Tests replace it. CLAWDYX_TOKEN_STORE=file skips it.
var TokenVault vault.Store = vault.Keychain{}

func vaultEnabled() bool { return TokenVault != nil && os.Getenv("CLAWDYX_TOKEN_STORE") != "file" }

// StoredToken returns the token kept in the vault for a config file.
func StoredToken(configPath string) (string, error) {
	if !vaultEnabled() {
		return "", vault.ErrNotFound
	}
	return TokenVault.Get(vault.TokenAccount(configPath))
}

// StoreToken validates tok and saves it in the vault for a config file.
func StoreToken(configPath, tok string) error {
	tok = strings.TrimSpace(tok)
	if !ValidToken(tok) {
		return errors.New("config: that does not look like a Telegram bot token")
	}
	if !vaultEnabled() {
		return errors.New("config: the system token store is disabled (CLAWDYX_TOKEN_STORE=file)")
	}
	return TokenVault.Set(vault.TokenAccount(configPath), tok)
}

// DeleteStoredToken removes the vault item for a config file.
func DeleteStoredToken(configPath string) error {
	if !vaultEnabled() {
		return nil
	}
	err := TokenVault.Delete(vault.TokenAccount(configPath))
	if errors.Is(err, vault.ErrNotFound) {
		return nil
	}
	return err
}

// Load reads the config file and the token: the TELEGRAM_BOT_TOKEN
// environment variable (development), else the system token store, else
// the private .env file. Only a missing token store (vault.ErrUnavailable)
// falls through to the file; any other token-store error fails closed. The
// config file must be private (0600).
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
	if env := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")); env != "" {
		if !tokenRe.MatchString(env) {
			return nil, errors.New("config: TELEGRAM_BOT_TOKEN has an invalid format")
		}
		c.Token, c.TokenSource = env, "env"
		return c, nil
	}
	if tok, err := StoredToken(configPath); err == nil {
		if !tokenRe.MatchString(tok) {
			return nil, errors.New("config: the stored token has an invalid format; run `clawdyx setup` to replace it")
		}
		c.Token, c.TokenSource = tok, "keychain"
		return c, nil
	} else if errors.Is(err, vault.ErrUnavailable) {
		c.NoKeyring = true
	} else if !errors.Is(err, vault.ErrNotFound) {
		return nil, fmt.Errorf("config: reading the bot token from the token store failed: %v (is %s locked?)", err, platform.W.TokenStore)
	}
	tok, err := LoadToken(envPath)
	if err != nil {
		return nil, errors.New(err.Error() + " — run `clawdyx setup` to store one")
	}
	c.Token, c.TokenSource = tok, "file"
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
	if !Private(st) {
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

	if users, err := NormalizeAllowlist(c.Telegram.OwnerUserID, c.Telegram.AllowedUsers); err != nil {
		add("%s", err.Error())
	} else {
		c.Telegram.AllowedUsers = users
		c.Telegram.OwnerUserID = users[0].ID
	}
	if c.Telegram.PollTimeoutSeconds < 5 || c.Telegram.PollTimeoutSeconds > 50 {
		add("telegram.poll_timeout_seconds must be 5..50")
	}

	cl := c.Claude
	if !filepath.IsAbs(cl.Binary) {
		add("claude.binary must be an absolute path")
	} else if st, err := os.Stat(cl.Binary); err != nil {
		add("claude.binary %s: %v", cl.Binary, err)
	} else if !Executable(st) {
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
	for _, p := range filepath.SplitList(cl.ChildPath) {
		if !filepath.IsAbs(p) {
			add("claude.child_path entries must be absolute (%q)", p)
		}
	}
	if cl.GitBashPath != "" && gitBashUsed {
		if err := checkGitBash(cl.GitBashPath); err != nil {
			add("claude.git_bash_path %v", err)
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
		if filepath.Dir(canon) == canon {
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

// ControlSocket is the worker's local control socket.
func (c *Config) ControlSocket() string { return filepath.Join(c.StateDir, "control.sock") }

// Rewrite applies mutate to the config file at path, validates the result
// like Parse, and replaces the file atomically (mode 0600). Defaults that
// were implicit are written explicitly.
func Rewrite(path string, mutate func(*Config) error) (*Config, error) {
	if err := checkPrivate(path); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c, err := Parse(raw)
	if err != nil {
		return nil, err
	}
	if err := mutate(c); err != nil {
		return nil, err
	}
	out, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	out = append(out, '\n')
	nc, err := Parse(out)
	if err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return nil, err
	}
	name := tmp.Name()
	if err := tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(out)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(name)
		return nil, err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return nil, err
	}
	return nc, nil
}
