package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/Publicthrone547/clawdyX/desktop/host"
	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/observability"
	"github.com/Publicthrone547/clawdyX/internal/platform"
)

// serviceLabel names the login entry (CLAWDYX_SERVICE_LABEL lets a test
// instance use its own).
var serviceLabel = config.ServiceLabel()

// ServiceInfo describes the background worker's LaunchAgent.
type ServiceInfo struct {
	Installed     bool   `json:"installed"`
	Loaded        bool   `json:"loaded"`
	PID           int    `json:"pid"`
	Binary        string `json:"binary"`        // program the LaunchAgent runs (home-relative)
	AppWorker     string `json:"appWorker"`     // worker shipped with this app ("" in dev builds)
	UsesAppWorker bool   `json:"usesAppWorker"` // the LaunchAgent runs the app's worker
	InBundle      bool   `json:"inBundle"`
}

// AppInfo is static information for the About and Settings screens.
type AppInfo struct {
	Version       string `json:"version"`
	ConfigPath    string `json:"configPath"`
	LogDir        string `json:"logDir"`
	StateDir      string `json:"stateDir"`
	LaunchAtLogin bool   `json:"launchAtLogin"`
	InBundle      bool   `json:"inBundle"`
	// TrayAvailable is false on a Linux desktop without a tray host; closing
	// the window then quits the app (the worker keeps running).
	TrayAvailable bool `json:"trayAvailable"`
}

// PlatformInfo is the OS the app runs on and its vocabulary; the frontend
// reads every OS-specific word from here.
type PlatformInfo struct {
	platform.Info
	Words platform.Words `json:"words"`
}

// SystemService manages the background worker's service, launch at login,
// and opens the few OS places the UI links to. Commands are fixed; the UI passes keys,
// never paths or command text.
type SystemService struct {
	app *application.App
	w   *WorkerService
}

func (s *SystemService) ServiceStartup(context.Context, application.ServiceOptions) error {
	s.app = application.Get()
	return nil
}

// Service reports the background worker's state (macOS: the LaunchAgent).
func (s *SystemService) Service() ServiceInfo {
	si := ServiceInfo{InBundle: appWorker() != ""}
	prog := ""
	si.Installed, si.Loaded, si.PID, prog = serviceStatus()
	si.Binary = homeRel(prog)
	if aw := appWorker(); aw != "" {
		si.AppWorker = homeRel(aw)
		si.UsesAppWorker = prog != "" && prog == aw
	}
	return si
}

func runWorker(timeout time.Duration, args ...string) (string, error) {
	bin, err := workerBinary()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	msg := strings.TrimSpace(observability.Redact(string(out)))
	if err != nil {
		if msg == "" {
			msg = err.Error()
		}
		return msg, errors.New(msg)
	}
	return msg, nil
}

// InstallService (re)installs the LaunchAgent for this app's worker and starts it.
func (s *SystemService) InstallService() (string, error) {
	return runWorker(40*time.Second, "service", "install")
}

// StartService starts (or restarts) the LaunchAgent.
func (s *SystemService) StartService() (string, error) {
	return runWorker(40*time.Second, "service", "start")
}

// StopService stops the LaunchAgent until it is started again.
func (s *SystemService) StopService() (string, error) {
	return runWorker(40*time.Second, "service", "stop")
}

// RestartService restarts the worker process.
func (s *SystemService) RestartService() (string, error) {
	return runWorker(40*time.Second, "service", "restart")
}

// UninstallService removes the LaunchAgent; purge also deletes state and logs.
func (s *SystemService) UninstallService(purge bool) (string, error) {
	args := []string{"service", "uninstall"}
	if purge {
		args = append(args, "-purge-state")
	}
	return runWorker(40*time.Second, args...)
}

// Info returns paths and app settings.
func (s *SystemService) Info() AppInfo {
	ai := AppInfo{Version: version, ConfigPath: homeRel(config.DefaultPath()), InBundle: appWorker() != ""}
	if cfg, err := loadConfig(); err == nil {
		ai.LogDir, ai.StateDir = homeRel(cfg.LogDir), homeRel(cfg.StateDir)
	}
	ai.TrayAvailable = trayAvailable
	if s.app != nil {
		ai.LaunchAtLogin, _ = s.app.Autostart.IsEnabled()
	}
	return ai
}

// Platform describes the OS (re-detected, so a Linux session change shows)
// and returns its words.
func (s *SystemService) Platform() PlatformInfo {
	return PlatformInfo{Info: platform.Detect(), Words: platform.W}
}

// SetLaunchAtLogin registers or removes the app as a login item.
func (s *SystemService) SetLaunchAtLogin(on bool) (bool, error) {
	var err error
	if on {
		err = s.app.Autostart.Enable()
	} else {
		err = s.app.Autostart.Disable()
	}
	if err != nil {
		return false, err
	}
	return s.app.Autostart.IsEnabled()
}

// links are the only places the UI can open: the project's pages and the
// OS settings pages of this OS (host.SettingsLinks).
var links = func() map[string]string {
	m := map[string]string{
		"botfather": "https://t.me/BotFather",
		"docs":      "https://github.com/Publicthrone547/clawdyX#readme",
		"license":   "https://polyformproject.org/licenses/noncommercial/1.0.0",
		"releases":  releasesURL,
	}
	for k, v := range host.SettingsLinks {
		m[k] = v
	}
	return m
}()

// HasLink reports whether this OS has a settings page for a key (the UI
// shows an "Open … Settings" button only then).
func (s *SystemService) HasLink(key string) bool {
	_, ok := links[key]
	return ok
}

// Open opens one of the known links.
func (s *SystemService) Open(key string) error {
	u, ok := links[key]
	if !ok {
		return fmt.Errorf("unknown link %q", key)
	}
	return openURL(u)
}

// RevealLogs opens the log folder (Finder on macOS, Explorer on Windows).
func (s *SystemService) RevealLogs() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	return openFolder(cfg.LogDir)
}

// RequestScreenAccess asks macOS for Screen Recording on behalf of the
// worker's capture helper (the prompt appears once; afterwards the user
// enables it in System Settings).
func (s *SystemService) RequestScreenAccess() error {
	if !screenSupported {
		return errScreenUnsupported
	}
	if b := bundleDir(); b != "" {
		h := filepath.Join(b, "Helpers", "clawdyx-screenshot")
		if isExec(h) {
			_ = exec.Command(h, "request").Run()
			return nil
		}
	}
	if _, ok := links["screen-recording"]; !ok {
		return nil // this OS asks for no screen permission
	}
	return s.Open("screen-recording")
}

// DiagnosticBundle builds a sanitized text report for preview: worker
// status, diagnostics, settings and the log tail. Tokens are redacted, home
// paths shortened, and Telegram IDs removed unless includeIDs.
func (s *SystemService) DiagnosticBundle(includeIDs bool) string {
	w := s.w
	var b strings.Builder
	fmt.Fprintf(&b, "# clawdyX diagnostic report\n\nCreated %s · app %s\n\n", time.Now().Format(time.RFC1123), version)
	fmt.Fprintf(&b, "## System\n\n%s\n\n", systemLine(platform.Current()))
	si := s.Service()
	fmt.Fprintf(&b, "## Service\n\ninstalled=%v loaded=%v pid=%d binary=%s usesAppWorker=%v\n\n", si.Installed, si.Loaded, si.PID, si.Binary, si.UsesAppWorker)
	if st, err := w.Status(); err == nil {
		fmt.Fprintf(&b, "## Worker\n\nversion %s · bot %s · jobs %d running / %d queued · Claude %s logged in=%v · screen enabled=%v\n", st.Version, st.Bot.State, st.Jobs.Running, st.Jobs.Queued, st.Claude.Version, st.Claude.LoggedIn != nil && *st.Claude.LoggedIn, st.Screen.Enabled)
		fmt.Fprintf(&b, "Telegram: poll errors %d · send errors %d · 409 conflicts %d · unauthorized dropped %d · allowed users %d · token in %s\n", st.Telegram.PollErrors, st.Telegram.SendErrors, st.Telegram.Conflicts409, st.Telegram.Unauthorized, st.Telegram.AllowedUsers, st.Telegram.TokenStore)
		if st.LastError != "" {
			fmt.Fprintf(&b, "Last error: %s\n", st.LastError)
		}
		b.WriteString("\n")
	} else {
		fmt.Fprintf(&b, "## Worker\n\nnot reachable: %v\n\n", err)
	}
	if checks, err := w.Diagnostics(); err == nil {
		b.WriteString("## Checks\n\n")
		for _, c := range checks {
			mark := "OK  "
			if !c.OK {
				mark = "FAIL"
			}
			if c.Warn {
				mark = "WARN"
			}
			fmt.Fprintf(&b, "- [%s] %s: %s\n", mark, c.Label, c.Detail)
		}
		b.WriteString("\n")
	}
	if set, err := w.Settings(); err == nil {
		fmt.Fprintf(&b, "## Settings\n\nworkspaces %d · default %s · concurrency %d · timeout %d min · automation %v · screen control %v · camera %v · log level %s\n\n", len(set.Workspaces), set.DefaultWorkspace, set.MaxConcurrentJobs, set.JobTimeoutMinutes, set.AutomationEnabled, set.ScreenControl, set.Camera, set.LogLevel)
	}
	if includeIDs {
		if users, err := w.Allowlist(); err == nil {
			b.WriteString("## Allowed Telegram users\n\n")
			for _, u := range users {
				fmt.Fprintf(&b, "- %d (%s) %s\n", u.ID, u.Role, u.Label)
			}
			b.WriteString("\n")
		}
	}
	if lines, err := w.LogTail(200, "info"); err == nil {
		b.WriteString("## Log (last 200 lines, info and above)\n\n```\n")
		for _, l := range lines {
			fmt.Fprintf(&b, "%s %s %s", l.Time, l.Level, l.Msg)
			for k, v := range l.Attrs {
				fmt.Fprintf(&b, " %s=%s", k, v)
			}
			b.WriteString("\n")
		}
		b.WriteString("```\n")
	}
	return sanitize(b.String(), includeIDs, w)
}

func sanitize(s string, includeIDs bool, w *WorkerService) string {
	s = observability.Redact(s)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		s = strings.ReplaceAll(s, home, "~")
	}
	if !includeIDs {
		if users, err := w.Allowlist(); err == nil {
			for _, u := range users {
				s = strings.ReplaceAll(s, strconv.FormatInt(u.ID, 10), "<telegram id>")
			}
		}
	}
	return s
}

// SaveDiagnostics asks where to save the previewed report and writes it (0600).
func (s *SystemService) SaveDiagnostics(text string) (string, error) {
	home, _ := os.UserHomeDir()
	path, err := s.app.Dialog.SaveFile().SetFilename("clawdyX-diagnostics-" + time.Now().Format("2006-01-02") + ".md").
		SetDirectory(filepath.Join(home, "Desktop")).PromptForSingleSelection()
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		return "", err
	}
	return homeRel(path), nil
}

// systemLine describes the computer: "Windows 11 23H2 · amd64",
// "Ubuntu 24.04 LTS · amd64 · Wayland (GNOME)".
func systemLine(in platform.Info) string {
	s := in.Pretty
	if s == "" {
		s = in.OSName
	}
	s += " · " + in.Arch
	if in.OS == "linux" {
		l := platform.SessionLabel(in.Session)
		if l == "" {
			l = platform.SessionLabel(platform.SessionHeadless)
		}
		s += " · " + l
		if in.Desktop != "" {
			s += " (" + in.Desktop + ")"
		}
	}
	return s
}

// CopyDiagnostics is the report without Telegram IDs, for the clipboard.
func (s *SystemService) CopyDiagnostics() string { return s.DiagnosticBundle(false) }

// RevealFolder shows one of clawdyX's folders (config, state, logs) in the
// file manager.
func (s *SystemService) RevealFolder(which string) error {
	cfgPath := config.DefaultPath()
	if abs, err := filepath.Abs(cfgPath); err == nil {
		cfgPath = abs
	}
	state, logs := config.Folders(cfgPath)
	switch which {
	case "config":
		return openFolder(filepath.Dir(cfgPath))
	case "state":
		return openFolder(state)
	case "logs":
		return openFolder(logs)
	}
	return fmt.Errorf("unknown folder %q", which)
}
