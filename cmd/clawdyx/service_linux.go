package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/config"
)

// On Linux the background worker is a systemd user service
// (~/.config/systemd/user/clawdyx.service, Restart=on-failure). Where no
// systemd user manager runs, an XDG autostart entry starts the supervisor
// at desktop login instead. A server without a desktop login needs
// "lingering" so the user manager runs from boot (loginctl enable-linger).

// unitName is the systemd unit for the service label: "clawdyx" for the
// default label, "clawdyx-dev" for local.clawdyx.dev.
func unitName() string {
	n := strings.ReplaceAll(strings.TrimPrefix(serviceLabel, "local."), ".", "-")
	if n == "" {
		n = "clawdyx"
	}
	return n
}

func xdgConfigHome() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" && filepath.IsAbs(d) {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}

func unitPath() string {
	return filepath.Join(xdgConfigHome(), "systemd", "user", unitName()+".service")
}

func autostartPath() string {
	return filepath.Join(xdgConfigHome(), "autostart", unitName()+".desktop")
}

// systemdQuote quotes one word of an Exec line: double quotes, with
// backslash, quote, "$" (variable expansion) and "%" (specifiers) escaped.
func systemdQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "$", "$$", "%", "%%")
	return `"` + r.Replace(s) + `"`
}

// Unit renders the systemd user unit. The token is never written here.
func Unit(exe, cfgPath, path string) string {
	return fmt.Sprintf(`[Unit]
Description=clawdyX Telegram bot for the local Claude Code CLI
After=network-online.target graphical-session.target
StartLimitIntervalSec=0

[Service]
Type=simple
ExecStart=%s run -config %s
Restart=on-failure
RestartSec=10
TimeoutStopSec=30
KillMode=mixed
Environment=%s

[Install]
WantedBy=default.target
`, systemdQuote(exe), systemdQuote(cfgPath), systemdQuote("PATH="+path))
}

// desktopEntry is the XDG autostart fallback. Exec arguments use the same
// double-quote rules (the Desktop Entry spec escapes \, ", ` and $).
func desktopEntry(exe, cfgPath string) string {
	q := func(s string) string {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", "$", `\$`).Replace(s) + `"`
	}
	return fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=clawdyX worker
Comment=Starts the clawdyX Telegram bot worker
Exec=%s service supervise -config %s
Terminal=false
NoDisplay=true
X-GNOME-Autostart-enabled=true
`, q(exe), q(cfgPath))
}

func systemctl(args ...string) (string, error) {
	out, err := exec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// userManager reports whether a systemd user manager answers.
func userManager() bool {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	_, err := systemctl("show-environment")
	return err == nil
}

// parseShow reads `systemctl show` output (KEY=value lines).
func parseShow(out string) map[string]string {
	m := map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(l), "="); ok {
			m[k] = v
		}
	}
	return m
}

func unitState() map[string]string {
	out, err := systemctl("show", unitName()+".service", "-p", "LoadState,ActiveState,SubState,MainPID,UnitFileState,ExecMainStartTimestamp")
	if err != nil {
		return map[string]string{}
	}
	return parseShow(out)
}

// linger reports whether the user manager runs without a login ("" when
// loginctl is missing).
func linger() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	out, err := exec.Command("loginctl", "show-user", u.Username, "-p", "Linger").Output()
	if err != nil {
		return ""
	}
	return parseShow(string(out))["Linger"]
}

func lingerHint() string {
	name := "$USER"
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	return "on a server without a desktop login, keep it running from boot with: loginctl enable-linger " + name
}

func resolvedExe() (string, error) {
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	return exe, err
}

func service(sub, cfgPath string, purge bool) int {
	switch sub {
	case "supervise":
		return supervise(cfgPath)
	case "start", "install", "restart":
		cfg, err := loadConfigOnly(cfgPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "config:", err)
			fmt.Fprintln(os.Stderr, "Run `clawdyx setup` first.")
			return exitConfig
		}
		exe, err := resolvedExe()
		if err != nil {
			fmt.Fprintln(os.Stderr, "cannot resolve executable:", err)
			return exitError
		}
		abs, _ := filepath.Abs(cfgPath)
		if err := os.MkdirAll(cfg.LogDir, 0o700); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitError
		}
		if !userManager() {
			return startAutostart(cfg, exe, abs)
		}
		active := unitState()["ActiveState"] == "active"
		if lockHeld(cfg) && !active {
			fmt.Fprintln(os.Stderr, "another clawdyx is already running in the foreground (it holds", filepath.Join(cfg.StateDir, "bot.lock")+").")
			fmt.Fprintln(os.Stderr, "Stop it with Ctrl-C in its terminal, then run this again.")
			return exitLocked
		}
		p := unitPath()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitError
		}
		if err := os.WriteFile(p, []byte(Unit(exe, abs, config.DefaultChildPath())), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitError
		}
		if out, err := systemctl("daemon-reload"); err != nil {
			fmt.Fprintf(os.Stderr, "systemctl --user daemon-reload failed: %v\n%s\n", err, out)
			return exitError
		}
		args := []string{"enable", "--now", unitName() + ".service"}
		if active {
			// A running worker restarts so it runs this executable and config.
			if out, err := systemctl("enable", unitName()+".service"); err != nil {
				fmt.Fprintf(os.Stderr, "systemctl --user enable failed: %v\n%s\n", err, out)
				return exitError
			}
			args = []string{"restart", unitName() + ".service"}
		}
		if out, err := systemctl(args...); err != nil {
			fmt.Fprintf(os.Stderr, "systemctl --user %s failed: %v\n%s\n", args[0], err, out)
			return exitError
		}
		fmt.Printf("service %s → %s (systemd user unit %s)\n", unitName(), exe, p)
		if l := linger(); l == "no" {
			fmt.Println("note: " + lingerHint())
		}
		verb := "started"
		if active {
			verb = "restarted"
		}
		return reportSystemd(cfg, verb)
	case "stop":
		cfg, err := loadConfigOnly(cfgPath)
		if err != nil {
			fmt.Println("config:", err)
			return exitConfig
		}
		stopped := false
		if userManager() {
			if st := unitState(); st["LoadState"] == "loaded" {
				if out, err := systemctl("disable", "--now", unitName()+".service"); err != nil {
					fmt.Fprintf(os.Stderr, "systemctl --user disable --now failed: %v\n%s\n", err, out)
					return exitError
				}
				stopped = true
			}
		}
		if _, err := os.Stat(autostartPath()); err == nil {
			_ = os.Remove(autostartPath())
			stopped = true
		}
		if _, ok := supervisorAlive(cfg.StateDir); ok {
			if err := stopAll(cfg); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return exitError
			}
			stopped = true
		}
		if !stopped {
			fmt.Println("not installed as a background service; nothing to stop. (A foreground `clawdyx run` is stopped with Ctrl-C.)")
			return exitOK
		}
		fmt.Println("stopped. It stays off until `clawdyx service start`.")
		return exitOK
	case "uninstall":
		if userManager() {
			_, _ = systemctl("disable", "--now", unitName()+".service")
		}
		p := unitPath()
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, err)
			return exitError
		}
		if userManager() {
			_, _ = systemctl("daemon-reload")
		}
		_ = os.Remove(autostartPath())
		fmt.Println("removed", p)
		cfg, err := loadConfigOnly(cfgPath)
		if err == nil {
			if _, ok := supervisorAlive(cfg.StateDir); ok {
				_ = stopAll(cfg)
			}
		}
		if purge {
			if err != nil {
				fmt.Fprintln(os.Stderr, "config (needed to locate state):", err)
				return exitConfig
			}
			for _, d := range []string{cfg.StateDir, cfg.LogDir} {
				if err := os.RemoveAll(d); err != nil {
					fmt.Fprintln(os.Stderr, err)
				} else {
					fmt.Println("deleted", d)
				}
			}
		}
		return exitOK
	case "where":
		// Prints the executable the unit runs (for the desktop app).
		b, err := os.ReadFile(unitPath())
		if err != nil {
			fmt.Println("")
			return exitError
		}
		if p := unitProgram(string(b)); p != "" {
			fmt.Println(p)
			return exitOK
		}
		return exitError
	case "status":
		cfg, err := loadConfigOnly(cfgPath)
		if err != nil {
			fmt.Println("config:", err)
			return exitConfig
		}
		st := map[string]string{}
		if userManager() {
			st = unitState()
		}
		spid, supervised := supervisorAlive(cfg.StateDir)
		ok := false
		switch {
		case st["ActiveState"] == "active" && st["MainPID"] != "" && st["MainPID"] != "0":
			fmt.Printf("running under systemd (pid %s)", st["MainPID"])
			if st["UnitFileState"] == "enabled" {
				fmt.Print(", starts at login")
			}
			fmt.Println()
			ok = true
		case st["LoadState"] == "loaded" && st["UnitFileState"] == "enabled":
			fmt.Printf("systemd unit %s is %s/%s (check the logs: journalctl --user -u %s)\n", unitName(), st["ActiveState"], st["SubState"], unitName())
		case supervised && lockHeld(cfg):
			fmt.Printf("running in the background (supervisor pid %d, worker pid %d), starts at desktop login\n", spid, readPID(cfg.StateDir))
			ok = true
		case supervised:
			fmt.Printf("the supervisor (pid %d) is starting the worker again; check %s\n", spid, cfg.LogDir)
		case lockHeld(cfg):
			fmt.Println("running in the foreground (not as a background service)")
		default:
			fmt.Println("stopped")
		}
		if l := linger(); l != "" {
			if l == "yes" {
				fmt.Println("lingering: on (runs from boot, also without a login)")
			} else {
				fmt.Println("lingering: off — " + lingerHint())
			}
		}
		if bs := workerBotState(cfg.ControlSocket()); bs != "" {
			fmt.Printf("telegram polling: %s", bs)
			if bs == "stopped" {
				fmt.Print(" (start it from the clawdyX app or: clawdyx ctl bot.start)")
			}
			fmt.Println()
		}
		fmt.Printf("logs: %s (also: journalctl --user -u %s)\n", cfg.LogDir, unitName())
		if ok {
			return exitOK
		}
		return exitError
	}
	fmt.Fprintln(os.Stderr, errNoSub)
	return exitConfig
}

// unitProgram returns the executable of a unit file's ExecStart line.
func unitProgram(unit string) string {
	for _, l := range strings.Split(unit, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(l), "ExecStart=")
		if !ok {
			continue
		}
		if strings.HasPrefix(rest, `"`) {
			var b strings.Builder
			for i := 1; i < len(rest); i++ {
				switch c := rest[i]; {
				case c == '\\' && i+1 < len(rest):
					i++
					b.WriteByte(rest[i])
				case c == '"':
					return strings.ReplaceAll(strings.ReplaceAll(b.String(), "$$", "$"), "%%", "%")
				default:
					b.WriteByte(c)
				}
			}
			return ""
		}
		return strings.Fields(rest)[0]
	}
	return ""
}

// startAutostart is the fallback without a systemd user manager: an XDG
// autostart entry for the next desktop login, and the supervisor now.
func startAutostart(cfg *config.Config, exe, abs string) int {
	if _, ok := supervisorAlive(cfg.StateDir); ok || lockHeld(cfg) {
		if err := stopAll(cfg); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitError
		}
	}
	p := autostartPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitError
	}
	if err := os.WriteFile(p, []byte(desktopEntry(exe, abs)), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitError
	}
	if err := spawnDetached(exe, "service", "supervise", "-config", abs); err != nil {
		fmt.Fprintln(os.Stderr, "could not start the worker:", err)
		return exitError
	}
	fmt.Printf("no systemd user manager found; the worker starts at desktop login from %s\n", p)
	for i := 0; i < 30; i++ {
		time.Sleep(300 * time.Millisecond)
		if pid := readPID(cfg.StateDir); pid > 0 && lockHeld(cfg) {
			fmt.Printf("started (pid %d). logs: %s · stop: clawdyx service stop · status: clawdyx service status\n", pid, cfg.LogDir)
			return exitOK
		}
	}
	fmt.Printf("started, but no worker is running yet. Check %s (bot.log, supervisor.log)\n", cfg.LogDir)
	return exitError
}

// reportSystemd waits briefly for systemd to start the worker.
func reportSystemd(cfg *config.Config, verb string) int {
	var pid string
	for i := 0; i < 20; i++ {
		time.Sleep(300 * time.Millisecond)
		if st := unitState(); st["ActiveState"] == "active" {
			if pid = st["MainPID"]; pid != "" && pid != "0" {
				break
			}
		}
	}
	if n, _ := strconv.Atoi(pid); n == 0 {
		fmt.Printf("%s, but no process is running yet. Check journalctl --user -u %s and %s\n", verb, unitName(), cfg.LogDir)
		return exitError
	}
	fmt.Printf("%s (pid %s). logs: %s · stop: clawdyx service stop · status: clawdyx service status\n", verb, pid, cfg.LogDir)
	return exitOK
}
