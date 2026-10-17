package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/Publicthrone547/clawdyX/internal/config"
)

const childPath = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", serviceLabel+".plist")
}

func xmlEsc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// Plist renders the LaunchAgent definition. The token is never written here;
// the bot reads it from the 0600 .env file at startup.
func Plist(binary, cfgPath, logDir, path string) string {
	home, _ := os.UserHomeDir()
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key><array>
    <string>%s</string><string>run</string><string>-config</string><string>%s</string>
  </array>
  <key>EnvironmentVariables</key><dict>
    <key>PATH</key><string>%s</string>
    <key>HOME</key><string>%s</string>
  </dict>
  <key>WorkingDirectory</key><string>%s</string>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>
  <key>ThrottleInterval</key><integer>10</integer>
  <key>ExitTimeOut</key><integer>30</integer>
  <key>ProcessType</key><string>Interactive</string>
  <key>LimitLoadToSessionType</key><string>Aqua</string>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
</dict></plist>
`, serviceLabel, xmlEsc(binary), xmlEsc(cfgPath), xmlEsc(path), xmlEsc(home), xmlEsc(home),
		xmlEsc(filepath.Join(logDir, "stdout.log")), xmlEsc(filepath.Join(logDir, "stderr.log")))
}

func domain() string { return "gui/" + strconv.Itoa(os.Getuid()) }

func target() string { return domain() + "/" + serviceLabel }

func launchctl(args ...string) (string, error) {
	out, err := exec.Command("/bin/launchctl", args...).CombinedOutput()
	return string(out), err
}

// serviceState reports whether launchd knows the service and the pid it runs.
func serviceState() (loaded bool, pid string) {
	out, err := launchctl("print", target())
	if err != nil {
		return false, ""
	}
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); strings.HasPrefix(l, "pid = ") {
			return true, strings.TrimPrefix(l, "pid = ")
		}
	}
	return true, ""
}

// lockHeld reports whether some clawdyx instance (foreground or launchd) is running.
func lockHeld(cfg *config.Config) bool {
	_ = os.MkdirAll(cfg.StateDir, 0o700)
	l := flock.New(filepath.Join(cfg.StateDir, "bot.lock"))
	ok, err := l.TryLock()
	if err != nil || !ok {
		return true
	}
	_ = l.Unlock()
	return false
}

// writePlist (re)generates the LaunchAgent plist for the running executable.
func writePlist(cfgPath string, cfg *config.Config) (plist, exe string, err error) {
	exe, err = os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		return "", "", fmt.Errorf("cannot resolve executable: %w", err)
	}
	abs, _ := filepath.Abs(cfgPath)
	if err := os.MkdirAll(cfg.LogDir, 0o700); err != nil {
		return "", "", err
	}
	p := plistPath()
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(Plist(exe, abs, cfg.LogDir, childPath)), 0o644); err != nil {
		return "", "", err
	}
	return p, exe, nil
}

func service(sub, cfgPath string, purge bool) int {
	switch sub {
	case "start", "install", "restart":
		cfg, err := loadConfigOnly(cfgPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "config:", err)
			fmt.Fprintln(os.Stderr, "Run `clawdyx setup` (or `make setup`) first.")
			return exitConfig
		}
		if loaded, _ := serviceState(); loaded {
			if out, err := launchctl("kickstart", "-k", target()); err != nil {
				fmt.Fprintf(os.Stderr, "launchctl kickstart failed: %v\n%s", err, out)
				return exitError
			}
			return report(cfg, "restarted")
		}
		if lockHeld(cfg) {
			fmt.Fprintln(os.Stderr, "another clawdyx is already running in the foreground (it holds", filepath.Join(cfg.StateDir, "bot.lock")+").")
			fmt.Fprintln(os.Stderr, "Stop it with Ctrl-C in its terminal, then run this again.")
			return exitLocked
		}
		p, exe, err := writePlist(cfgPath, cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitError
		}
		_, _ = launchctl("enable", target())
		if out, err := launchctl("bootstrap", domain(), p); err != nil {
			fmt.Fprintf(os.Stderr, "launchctl bootstrap failed: %v\n%s", err, out)
			return exitError
		}
		fmt.Printf("service %s → %s\n", serviceLabel, exe)
		if cfg.Desktop.ScreenControl {
			fmt.Printf("screen control is on: macOS must grant Screen Recording and Accessibility to\n  %s\n(System Settings → Privacy & Security). Re-grant after every rebuild.\n", exe)
		}
		return report(cfg, "started")
	case "stop":
		loaded, _ := serviceState()
		if !loaded {
			fmt.Println("not running under launchd; nothing to stop. (A foreground `clawdyx run` is stopped with Ctrl-C.)")
			return exitOK
		}
		if out, err := launchctl("bootout", target()); err != nil {
			fmt.Fprintf(os.Stderr, "launchctl bootout failed: %v\n%s", err, out)
			return exitError
		}
		_, _ = launchctl("disable", target())
		fmt.Println("stopped. It stays off until `clawdyx service start` (or `make start`).")
		return exitOK
	case "uninstall":
		_, _ = launchctl("bootout", target())
		_, _ = launchctl("enable", target())
		p := plistPath()
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, err)
			return exitError
		}
		fmt.Println("removed", p)
		if purge {
			cfg, err := loadConfigOnly(cfgPath)
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
	case "status":
		cfg, err := loadConfigOnly(cfgPath)
		if err != nil {
			fmt.Println("config:", err)
			return exitConfig
		}
		loaded, pid := serviceState()
		switch {
		case loaded && pid != "":
			fmt.Printf("running under launchd (pid %s), starts at login\n", pid)
		case loaded:
			fmt.Println("loaded in launchd but no process is running (check the logs)")
		case lockHeld(cfg):
			fmt.Println("running in the foreground (not under launchd)")
		default:
			fmt.Println("stopped")
		}
		fmt.Printf("logs: %s\ndetails: launchctl print %s\n", cfg.LogDir, target())
		if loaded {
			return exitOK
		}
		return exitError
	}
	fmt.Fprintln(os.Stderr, errNoSub)
	return exitConfig
}

// report waits briefly for launchd to spawn the process and prints the outcome.
func report(cfg *config.Config, verb string) int {
	var pid string
	for i := 0; i < 10; i++ {
		time.Sleep(300 * time.Millisecond)
		if _, pid = serviceState(); pid != "" {
			break
		}
	}
	if pid == "" {
		fmt.Printf("%s, but no process is running yet. Check %s/stderr.log\n", verb, cfg.LogDir)
		return exitError
	}
	fmt.Printf("%s (pid %s). logs: %s · stop: make stop · status: make status\n", verb, pid, cfg.LogDir)
	return exitOK
}
