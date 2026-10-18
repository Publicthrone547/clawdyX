package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/registry"

	"github.com/Publicthrone547/clawdyX/internal/agent"
	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/control"
)

// On Windows the background worker is a detached `clawdyx run` process,
// started now by `service start` and at every login from the per-user Run
// key (no administrator rights needed). It replaces the LaunchAgent.
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

// stopWorker asks the worker to exit cleanly (worker.stop over the control
// socket) and waits for the state lock; after 40 s it ends the worker's
// process tree.
func stopWorker(cfg *config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if c, err := control.Dial(ctx, cfg.ControlSocket()); err == nil {
		_ = c.Call(ctx, "worker.stop", nil, nil)
		c.Close()
	}
	cancel()
	for i := 0; i < 160; i++ {
		if !lockHeld(cfg) {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	if pid := readPID(cfg.StateDir); pid > 0 && agent.ProcessComm(pid) == "clawdyx" {
		_ = agent.KillGroup(pid, true)
		for i := 0; i < 20 && lockHeld(cfg); i++ {
			time.Sleep(250 * time.Millisecond)
		}
	}
	if lockHeld(cfg) {
		return errors.New("the worker did not stop; end clawdyx.exe in Task Manager")
	}
	return nil
}

func service(sub, cfgPath string, purge bool) int {
	switch sub {
	case "start", "install", "restart":
		cfg, err := loadConfigOnly(cfgPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "config:", err)
			fmt.Fprintln(os.Stderr, "Run `clawdyx setup` first.")
			return exitConfig
		}
		_, registered := runValue()
		if lockHeld(cfg) {
			if !registered {
				fmt.Fprintln(os.Stderr, "another clawdyx is already running in the foreground (it holds", filepath.Join(cfg.StateDir, "bot.lock")+").")
				fmt.Fprintln(os.Stderr, "Stop it with Ctrl-C in its terminal, then run this again.")
				return exitLocked
			}
			if err := stopWorker(cfg); err != nil {
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
		if err := setRunValue(RunCommand(exe, abs)); err != nil {
			fmt.Fprintln(os.Stderr, "could not register the worker to start at login:", err)
			return exitError
		}
		if err := spawnDetached(exe, "run", "-config", abs); err != nil {
			fmt.Fprintln(os.Stderr, "could not start the worker:", err)
			return exitError
		}
		fmt.Printf("service %s → %s (starts at login)\n", serviceLabel, exe)
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
		if !registered {
			fmt.Println("not installed as a background service; nothing to stop. (A foreground `clawdyx run` is stopped with Ctrl-C.)")
			return exitOK
		}
		if err := deleteRunValue(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitError
		}
		if err := stopWorker(cfg); err != nil {
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
		if err == nil && lockHeld(cfg) {
			if err := stopWorker(cfg); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return exitError
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
		// Prints the executable the login entry runs (for the desktop app).
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
		switch {
		case running && registered:
			fmt.Printf("running in the background (pid %d), starts at login\n", readPID(cfg.StateDir))
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
	for i := 0; i < 20; i++ {
		time.Sleep(300 * time.Millisecond)
		if lockHeld(cfg) {
			if pid = readPID(cfg.StateDir); pid > 0 {
				break
			}
		}
	}
	if pid == 0 {
		fmt.Printf("%s, but no process is running yet. Check %s (bot.log, stderr.log)\n", verb, cfg.LogDir)
		return exitError
	}
	fmt.Printf("%s (pid %d). logs: %s · stop: clawdyx service stop · status: clawdyx service status\n", verb, pid, cfg.LogDir)
	return exitOK
}
