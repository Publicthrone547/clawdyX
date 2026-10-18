// Package settings is the one implementation of reading and changing the
// configuration from the desktop app: the worker's settings.* control
// methods use it, and so does the app itself while the worker is down. It
// never validates on its own: every change goes through config.Rewrite,
// which parses the result exactly like the worker does at startup, so the
// app can never write a file the worker would refuse.
package settings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/camera"
	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/control"
	"github.com/Publicthrone547/clawdyX/internal/policy"
	"github.com/Publicthrone547/clawdyX/internal/setup"
)

// Info describes c for the app. def is the default workspace the worker
// uses now (it can differ from the file until a restart).
func Info(c *config.Config, path, def string, restartPending bool) control.SettingsInfo {
	if def == "" {
		def = c.DefaultWorkspace
	}
	cl := c.Claude
	si := control.SettingsInfo{
		ConfigPath: path, OwnerUserID: c.Telegram.OwnerUserID, DefaultWorkspace: def, Workspaces: []control.WorkspaceInfo{},
		PollTimeoutSeconds: c.Telegram.PollTimeoutSeconds,
		ClaudeBinary:       cl.Binary, SettingSources: splitSources(cl.SettingSources), StrictMCPConfig: cl.StrictMCPConfig,
		GitBashPath: cl.GitBashPath, DefaultModel: cl.DefaultModel, DefaultEffort: cl.DefaultEffort,
		MaxConcurrentJobs: cl.MaxConcurrentJobs, JobTimeoutMinutes: int(cl.JobTimeout.Duration / time.Minute), MaxTurns: cl.MaxTurns,
		MaxBudgetUSD: cl.MaxBudgetUSD, MaxQueuePerSession: cl.MaxQueuePerSession, MaxOutputBytes: cl.MaxOutputBytes,
		MaxChildRSSMB: cl.MaxChildRSSMB, BgWaitCeilingMS: cl.BgWaitCeilingMS,
		ChildPath: splitPath(cl.ChildPath), ChildPathDefault: splitPath(defaultChildPath(cl.Binary)),
		ExtraDisallowedTools: append([]string{}, cl.ExtraDisallowedTools...), BuiltinDenied: policy.BuiltinDenied(),
		AutomationEnabled: c.Modes.AutomationEnabled, ScreenControl: c.Desktop.ScreenControl,
		Camera: c.Desktop.Camera, CameraSupported: camera.Supported, OpenURLs: c.Desktop.OpenEnabled(),
		BrowserApp: c.Desktop.BrowserApp, ShutdownCommand: c.ShutdownCommandEnabled,
		StateDir: c.StateDir, LogDir: c.LogDir, LogLevel: c.Logging.Level,
		JobsDays: c.Retention.JobsDays, JobEventsDays: c.Retention.JobEventsDays, AuditDays: c.Retention.AuditDays,
		ProcessedUpdatesDays: c.Retention.ProcessedUpdatesDays,
		RestartPending:       restartPending,
	}
	for _, w := range c.Workspaces {
		si.Workspaces = append(si.Workspaces, control.WorkspaceInfo{Name: w.Name, Path: w.Path, DefaultMode: w.DefaultMode,
			AllowedCommands: append([]string{}, w.AllowedCommands...), DeniedCommands: append([]string{}, w.DeniedCommands...),
			IKnowWhatIAmDoing: w.IKnowWhatIAmDoing, IsDefault: w.Name == def})
	}
	return si
}

func splitSources(s string) []string {
	out := []string{}
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

func splitPath(s string) []string {
	out := []string{}
	for _, p := range filepath.SplitList(s) {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// defaultChildPath is the child PATH a new config gets: the OS default
// with the folder of the claude binary in front (as setup writes it).
func defaultChildPath(claudeBin string) string {
	p := config.DefaultChildPath()
	sep := string(os.PathListSeparator)
	if claudeBin != "" {
		if dir := filepath.Dir(claudeBin); !strings.Contains(sep+p+sep, sep+dir+sep) {
			p = dir + sep + p
		}
	}
	return p
}

var (
	ruleRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*\([^\r\n]*\)$`)
	toolRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
)

// checkRules accepts permission rules of the form Tool(...), and with
// bareTools also plain tool names (for extra_disallowed_tools).
func checkRules(what string, rules []string, bareTools bool) error {
	for _, r := range rules {
		if ruleRe.MatchString(r) || (bareTools && toolRe.MatchString(r)) {
			continue
		}
		return fmt.Errorf("%s: %q is not a rule like Bash(npm test *)", what, r)
	}
	return nil
}

func set[T any](dst *T, v *T) {
	if v != nil {
		*dst = *v
	}
}

// Apply changes c according to p. Range checks are config.Parse's
// (through config.Rewrite); Apply only refuses what Parse cannot see.
func Apply(c *config.Config, p control.SettingsPatch, inUse func(ws string) bool) error {
	cl := &c.Claude
	set(&c.Telegram.PollTimeoutSeconds, p.PollTimeoutSeconds)
	set(&cl.Binary, p.ClaudeBinary)
	if p.SettingSources != nil {
		cl.SettingSources = strings.Join(*p.SettingSources, ",")
	}
	set(&cl.StrictMCPConfig, p.StrictMCPConfig)
	if p.GitBashPath != nil {
		cl.GitBashPath = strings.TrimSpace(*p.GitBashPath)
	}
	set(&cl.MaxConcurrentJobs, p.MaxConcurrentJobs)
	if p.JobTimeoutMinutes != nil {
		cl.JobTimeout = config.Duration{Duration: time.Duration(*p.JobTimeoutMinutes) * time.Minute}
	}
	set(&cl.MaxTurns, p.MaxTurns)
	set(&cl.MaxBudgetUSD, p.MaxBudgetUSD)
	set(&cl.MaxQueuePerSession, p.MaxQueuePerSession)
	set(&cl.MaxOutputBytes, p.MaxOutputBytes)
	set(&cl.MaxChildRSSMB, p.MaxChildRSSMB)
	set(&cl.BgWaitCeilingMS, p.BgWaitCeilingMS)
	if p.ChildPath != nil {
		if len(*p.ChildPath) == 0 {
			return errors.New("the child PATH needs at least one folder")
		}
		cl.ChildPath = strings.Join(*p.ChildPath, string(os.PathListSeparator))
	}
	if p.ExtraDisallowedTools != nil {
		if err := checkRules("extra denied tools", *p.ExtraDisallowedTools, true); err != nil {
			return err
		}
		cl.ExtraDisallowedTools = append([]string{}, *p.ExtraDisallowedTools...)
	}
	set(&c.Modes.AutomationEnabled, p.AutomationEnabled)
	set(&c.Desktop.ScreenControl, p.ScreenControl)
	if p.Camera != nil {
		if *p.Camera && !camera.Supported {
			return camera.ErrUnsupported
		}
		c.Desktop.Camera = *p.Camera
	}
	if p.OpenURLs != nil {
		v := *p.OpenURLs
		c.Desktop.OpenURLs = &v
	}
	set(&c.Desktop.BrowserApp, p.BrowserApp)
	set(&c.ShutdownCommandEnabled, p.ShutdownCommand)
	set(&c.StateDir, p.StateDir)
	set(&c.LogDir, p.LogDir)
	set(&c.Logging.Level, p.LogLevel)
	set(&c.Retention.JobsDays, p.JobsDays)
	set(&c.Retention.JobEventsDays, p.JobEventsDays)
	set(&c.Retention.AuditDays, p.AuditDays)
	set(&c.Retention.ProcessedUpdatesDays, p.ProcessedUpdatesDays)

	for name, mode := range p.WorkspaceModes {
		w, ok := c.Workspace(name)
		if !ok {
			return fmt.Errorf("unknown workspace %q", name)
		}
		w.DefaultMode = mode
	}
	for name, r := range p.WorkspaceRules {
		w, ok := c.Workspace(name)
		if !ok {
			return fmt.Errorf("unknown workspace %q", name)
		}
		if r.Allowed != nil {
			if err := checkRules("allowed commands", *r.Allowed, false); err != nil {
				return err
			}
			w.AllowedCommands = append([]string{}, *r.Allowed...)
		}
		if r.Denied != nil {
			if err := checkRules("denied commands", *r.Denied, false); err != nil {
				return err
			}
			w.DeniedCommands = append([]string{}, *r.Denied...)
		}
		set(&w.IKnowWhatIAmDoing, r.IKnowWhatIAmDoing)
	}
	if p.AddWorkspace != nil {
		path := filepath.Clean(setup.ExpandHome(p.AddWorkspace.Path))
		if err := setup.WorkspaceOK(path); err != nil {
			return err
		}
		name := p.AddWorkspace.Name
		if name == "" {
			name = setup.WorkspaceName(path)
		}
		w := setup.NewWorkspace(name, path)
		if p.AddWorkspace.DefaultMode != "" {
			w.DefaultMode = p.AddWorkspace.DefaultMode
		}
		c.Workspaces = append(c.Workspaces, w)
	}
	if p.RenameWorkspace != nil {
		from, to := p.RenameWorkspace.From, strings.TrimSpace(p.RenameWorkspace.To)
		w, ok := c.Workspace(from)
		if !ok {
			return fmt.Errorf("unknown workspace %q", from)
		}
		if inUse(from) {
			return fmt.Errorf("%s is used by sessions; archive or forget them first", from)
		}
		w.Name = to
		if c.DefaultWorkspace == from {
			c.DefaultWorkspace = to
		}
	}
	if p.RemoveWorkspace != "" {
		if p.RemoveWorkspace == c.DefaultWorkspace {
			return fmt.Errorf("%s is the default workspace; make another one the default first", p.RemoveWorkspace)
		}
		if inUse(p.RemoveWorkspace) {
			return fmt.Errorf("%s is used by sessions; archive or forget them first", p.RemoveWorkspace)
		}
		kept := c.Workspaces[:0]
		for _, w := range c.Workspaces {
			if w.Name != p.RemoveWorkspace {
				kept = append(kept, w)
			}
		}
		if len(kept) == len(c.Workspaces) {
			return fmt.Errorf("unknown workspace %q", p.RemoveWorkspace)
		}
		c.Workspaces = kept
	}
	return nil
}

// Reset restores the defaults of a section (control.Section*). The
// Telegram settings and the workspaces' names and folders are never reset.
func Reset(c *config.Config, section string) error {
	d := config.Defaults()
	all := section == control.SectionAll
	known := all
	pick := func(s string) bool {
		if section == s {
			known = true
		}
		return all || section == s
	}
	if pick(control.SectionClaudeCode) {
		if bin := setup.DetectClaude(); bin != "" {
			c.Claude.Binary = bin
		}
		// The values `clawdyx setup` writes, not the empty-document defaults.
		c.Claude.SettingSources, c.Claude.StrictMCPConfig, c.Claude.GitBashPath = "user", true, ""
	}
	if pick(control.SectionDefaults) {
		c.Claude.DefaultModel, c.Claude.DefaultEffort = d.Claude.DefaultModel, d.Claude.DefaultEffort
	}
	if pick(control.SectionLimits) {
		c.Claude.JobTimeout, c.Claude.MaxTurns, c.Claude.MaxBudgetUSD = d.Claude.JobTimeout, d.Claude.MaxTurns, 0
		c.Claude.MaxConcurrentJobs, c.Claude.MaxQueuePerSession = d.Claude.MaxConcurrentJobs, d.Claude.MaxQueuePerSession
	}
	if pick(control.SectionFeatures) {
		on := true
		c.Modes.AutomationEnabled, c.Desktop.ScreenControl, c.Desktop.Camera = false, false, false
		c.Desktop.OpenURLs, c.Desktop.BrowserApp, c.ShutdownCommandEnabled = &on, setup.DetectBrowser(), false
	}
	if pick(control.SectionPermissions) {
		c.Claude.ExtraDisallowedTools = []string{}
	}
	if pick(control.SectionWorkspaces) {
		for i := range c.Workspaces {
			w := &c.Workspaces[i]
			w.DefaultMode, w.IKnowWhatIAmDoing = config.ModeObserve, false
			w.AllowedCommands = append([]string{}, setup.DefaultAllowed...)
			w.DeniedCommands = append([]string{}, setup.DefaultDenied...)
		}
	}
	if pick(control.SectionLogs) {
		c.Logging.Level = d.Logging.Level
		c.Retention.JobsDays, c.Retention.JobEventsDays, c.Retention.AuditDays = d.Retention.JobsDays, d.Retention.JobEventsDays, d.Retention.AuditDays
	}
	if pick(control.SectionAdvanced) {
		c.Telegram.PollTimeoutSeconds = d.Telegram.PollTimeoutSeconds
		c.Claude.MaxOutputBytes, c.Claude.MaxChildRSSMB, c.Claude.BgWaitCeilingMS = d.Claude.MaxOutputBytes, d.Claude.MaxChildRSSMB, d.Claude.BgWaitCeilingMS
		c.Claude.ChildPath = defaultChildPath(c.Claude.Binary)
		c.StateDir, c.LogDir = d.StateDir, d.LogDir
		c.Retention.ProcessedUpdatesDays = d.Retention.ProcessedUpdatesDays
	}
	if !known {
		return fmt.Errorf("unknown settings section %q", section)
	}
	return nil
}

// Save applies p to the file at path.
func Save(path string, p control.SettingsPatch, inUse func(string) bool) (*config.Config, error) {
	return config.Rewrite(path, func(c *config.Config) error { return Apply(c, p, inUse) })
}

// ResetFile resets a section of the file at path.
func ResetFile(path, section string) (*config.Config, error) {
	return config.Rewrite(path, func(c *config.Config) error { return Reset(c, section) })
}

// Load reads the file at path without validating it (see
// config.ParseLenient): invalid is why the worker would refuse it.
func Load(path string) (c *config.Config, invalid error, err error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	if !config.Private(st) {
		return nil, nil, fmt.Errorf("%s must not be readable by other users (chmod 600)", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	return config.ParseLenient(b)
}
