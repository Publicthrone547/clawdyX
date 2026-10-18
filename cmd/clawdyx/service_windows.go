package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/registry"

	"github.com/Publicthrone547/clawdyX/internal/config"
)

// On Windows the background worker runs under the supervisor
// (`clawdyx service supervise`), which restarts it after a crash. It is
// started now by `service start` and at every login from the per-user Run
// key through clawdyxw.exe, a GUI program, so no console window flashes
// (no administrator rights needed). It replaces the LaunchAgent.
const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

func runValue() (string, bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(serviceLabel)
	return v, err == nil
}

func setRunValue(v string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(serviceLabel, v)
}

func deleteRunValue() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	if err := k.DeleteValue(serviceLabel); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
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
		_, registered := runValue()
		_, supervised := supervisorAlive(cfg.StateDir)
		if lockHeld(cfg) || supervised {
			if !registered && !supervised {
				fmt.Fprintln(os.Stderr, "another clawdyx is already running in the foreground (it holds", filepath.Join(cfg.StateDir, "bot.lock")+").")
				fmt.Fprintln(os.Stderr, "Stop it with Ctrl-C in its terminal, then run this again.")
				return exitLocked
			}
			if err := stopAll(cfg); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return exitError
			}
		}
		// The worker is stopped: move 0.4.x settings to %APPDATA% now, so
		// the login entry below already points at the new place.
		cfgPath = migrateLegacy(cfgPath)
		exe, err := os.Executable()
		if err == nil {
			exe, err = filepath.EvalSymlinks(exe)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "cannot resolve executable:", err)
			return exitError
		}
		abs, _ := filepath.Abs(cfgPath)
		if err := os.MkdirAll(cfg.LogDir, 0o700); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitError
		}
		_, lerr := os.Stat(filepath.Join(filepath.Dir(exe), launcherName))
		if err := setRunValue(RunCommand(exe, abs, lerr == nil)); err != nil {
			fmt.Fprintln(os.Stderr, "could not register the worker to start at login:", err)
			fmt.Fprintf(os.Stderr, "Start it at login another way: put a shortcut to\n  %s\nin the Startup folder (Win+R, shell:startup).\n", RunCommand(exe, abs, lerr == nil))
			return exitError
		}
		if err := spawnDetached(exe, "service", "supervise", "-config", abs); err != nil {
			fmt.Fprintln(os.Stderr, "could not start the worker:", err)
			return exitError
		}
		fmt.Printf("service %s → %s (starts at login)\n", serviceLabel, exe)
		if lerr != nil {
			fmt.Printf("note: %s is not next to it; a console window may appear for a moment at login\n", launcherName)
		}
		verb := "started"
		if registered && sub != "install" {
			verb = "restarted"
		}
		return report(cfg, verb)
	case "stop":
		cfg, err := loadConfigOnly(cfgPath)
		if err != nil {
			fmt.Println("config:", err)
			return exitConfig
		}
		_, registered := runValue()
		_, supervised := supervisorAlive(cfg.StateDir)
		if !registered && !supervised {
			fmt.Println("not installed as a background service; nothing to stop. (A foreground `clawdyx run` is stopped with Ctrl-C.)")
			return exitOK
		}
		if err := deleteRunValue(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitError
		}
		if err := stopAll(cfg); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitError
		}
		fmt.Println("stopped. It stays off until `clawdyx service start`.")
		return exitOK
	case "uninstall":
		if err := deleteRunValue(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitError
		}
		cfg, err := loadConfigOnly(cfgPath)
		if err == nil {
			if _, supervised := supervisorAlive(cfg.StateDir); supervised || lockHeld(cfg) {
				if err := stopAll(cfg); err != nil {
					fmt.Fprintln(os.Stderr, err)
					return exitError
				}
			}
		}
		fmt.Println("removed the login entry", `HKCU\`+runKeyPath+`\`+serviceLabel)
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
		// Prints the worker executable the login entry runs (for the desktop app).
		v, ok := runValue()
		if p := runProgram(v); ok && p != "" {
			fmt.Println(p)
			return exitOK
		}
		fmt.Println("")
		return exitError
	case "status":
		cfg, err := loadConfigOnly(cfgPath)
		if err != nil {
			fmt.Println("config:", err)
			return exitConfig
		}
		_, registered := runValue()
		running := lockHeld(cfg)
		spid, supervised := supervisorAlive(cfg.StateDir)
		switch {
		case running && supervised:
			fmt.Printf("running in the background (supervisor pid %d, worker pid %d)", spid, readPID(cfg.StateDir))
			if registered {
				fmt.Print(", starts at login")
			}
			fmt.Println()
		case supervised:
			fmt.Printf("the supervisor (pid %d) is starting the worker again; check %s\n", spid, cfg.LogDir)
		case running:
			fmt.Println("running in the foreground (not as a background service)")
		case registered:
			fmt.Println("registered to start at login, but no process is running (check the logs)")
		default:
			fmt.Println("stopped")
		}
		if bs := workerBotState(cfg.ControlSocket()); bs != "" {
			fmt.Printf("telegram polling: %s", bs)
			if bs == "stopped" {
				fmt.Print(" (start it from the clawdyX app or: clawdyx ctl bot.start)")
			}
			fmt.Println()
		}
		fmt.Printf("logs: %s\n", cfg.LogDir)
		if registered && running {
			return exitOK
		}
		return exitError
	}
	fmt.Fprintln(os.Stderr, errNoSub)
	return exitConfig
}

// report waits briefly for the worker to take the state lock.
func report(cfg *config.Config, verb string) int {
	pid := 0
	for i := 0; i < 30; i++ {
		time.Sleep(300 * time.Millisecond)
		if lockHeld(cfg) {
			if pid = readPID(cfg.StateDir); pid > 0 {
				break
			}
		}
	}
	if pid > 0 {
		// A worker that cannot start (no network, a rejected token) exits
		// within a second; the supervisor keeps trying.
		time.Sleep(1500 * time.Millisecond)
		if !lockHeld(cfg) {
			fmt.Printf("%s, but the worker stopped again at once; the supervisor keeps trying. Check %s (bot.log, supervisor.log)\n", verb, cfg.LogDir)
			return exitError
		}
	}
	if pid == 0 {
		fmt.Printf("%s, but no process is running yet. Check %s (bot.log, supervisor.log, stderr.log)\n", verb, cfg.LogDir)
		return exitError
	}
	fmt.Printf("%s (pid %d). logs: %s · stop: clawdyx service stop · status: clawdyx service status\n", verb, pid, cfg.LogDir)
	return exitOK
}
