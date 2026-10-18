// Command clawdyx is a private Telegram control plane for the locally
// installed and authenticated Claude Code CLI.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"github.com/go-telegram/bot"
	"github.com/gofrs/flock"

	"github.com/Publicthrone547/clawdyX/internal/agent"
	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/control"
	"github.com/Publicthrone547/clawdyX/internal/observability"
	"github.com/Publicthrone547/clawdyX/internal/platform"
	"github.com/Publicthrone547/clawdyX/internal/screen"
	"github.com/Publicthrone547/clawdyX/internal/store"
	"github.com/Publicthrone547/clawdyX/internal/telegram"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "0.4.1"

const (
	exitOK       = 0
	exitError    = 1
	exitConfig   = 2
	exitLocked   = 3
	exitRestart  = 75
	serviceLabel = "local.clawdyx"
)

func usage() {
	fmt.Fprintf(os.Stderr, `clawdyx %s — Telegram control plane for the local Claude Code CLI

Usage:
  clawdyx setup                     guided first-time setup: token, your Telegram ID, project folder
  clawdyx run                       run the bot in the foreground (Ctrl-C stops it)
  clawdyx service start|stop|restart|status
                                    background service that also starts at login
                                    (macOS: LaunchAgent; Windows: detached process + login entry)
  clawdyx check                     validate config, token, CLI, auth and Telegram (read-only)
  clawdyx service uninstall [--purge-state]
  clawdyx backup                    write a consistent copy of the state database
  clawdyx ctl METHOD [JSON]         call the running worker (e.g. ctl status.get, ctl bot.stop '{"cancelJobs":true}');
                                    ctl events streams events; ctl help lists methods
  clawdyx version

Flags (all commands): -config PATH (default %s), -env PATH (default %s)
Setup flags (optional, for scripts): -owner-id N -workspace DIR -browser NAME -claude-binary PATH
                                     -token-stdin -token-file -migrate-token -force -no-check -yes
`, version, config.DefaultPath(), config.DefaultEnvPath())
}

func main() {
	telegram.Version = version
	if len(os.Args) < 2 {
		usage()
		os.Exit(exitConfig)
	}
	cmd, rest := os.Args[1], os.Args[2:]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	cfgPath := fs.String("config", config.DefaultPath(), "config file")
	envPath := fs.String("env", config.DefaultEnvPath(), "secrets file with TELEGRAM_BOT_TOKEN")
	purge := fs.Bool("purge-state", false, "service uninstall: also delete state and logs")
	var so setupOptions
	fs.Int64Var(&so.OwnerID, "owner-id", 0, "setup: your numeric Telegram user ID")
	fs.StringVar(&so.Workspace, "workspace", "", "setup: project folder Claude may work in")
	fs.StringVar(&so.Browser, "browser", "", "setup: browser app for /open")
	fs.StringVar(&so.ClaudeBinary, "claude-binary", "", "setup: path of the claude CLI")
	fs.BoolVar(&so.TokenStdin, "token-stdin", false, "setup: read the bot token from stdin (non-interactive)")
	fs.BoolVar(&so.TokenFile, "token-file", false, "setup: keep the token in the 0600 .env file instead of the Keychain")
	fs.BoolVar(&so.MigrateToken, "migrate-token", false, "setup: move the token from the .env file into the Keychain")
	fs.BoolVar(&so.Force, "force", false, "setup: replace existing config and token files")
	fs.BoolVar(&so.NoCheck, "no-check", false, "setup: skip the read-only checks at the end")
	fs.BoolVar(&so.Yes, "yes", false, "setup: never prompt; use flags and defaults")
	sub := ""
	if cmd == "service" && len(rest) > 0 {
		sub, rest = rest[0], rest[1:]
	}
	_ = fs.Parse(rest)

	switch cmd {
	case "setup":
		os.Exit(runSetup(*cfgPath, *envPath, so, os.Stdin, os.Stdout))
	case "run":
		code, background := run(*cfgPath, *envPath)
		os.Exit(relaunch(code, background))
	case "check":
		os.Exit(check(*cfgPath, *envPath))
	case "service":
		os.Exit(service(sub, *cfgPath, *purge))
	case "backup":
		os.Exit(backup(*cfgPath))
	case "ctl":
		os.Exit(ctl(*cfgPath, fs.Args()))
	case "mcp-screen":
		// Started by claude (via --mcp-config) for automation sessions with
		// screen control. Speaks MCP on stdin/stdout.
		mac := screen.Mac{TmpDir: os.TempDir()}
		if exe, err := os.Executable(); err == nil {
			if exe, err = filepath.EvalSymlinks(exe); err == nil {
				mac.Helper = screen.FindHelper(exe)
			}
		}
		srv := &screen.Server{D: mac, Version: version, Displays: mac.Displays}
		if sock := os.Getenv("CLAWDYX_CONTROL_SOCKET"); sock != "" {
			// Ask the worker before every screenshot: the owner may have
			// switched screen features off since this job started.
			srv.Authorize = func(ctx context.Context, display string) error {
				cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				c, err := control.Dial(cctx, sock)
				if err != nil {
					return errors.New("the clawdyx worker is not reachable")
				}
				defer c.Close()
				return c.Call(cctx, "screens.authorize", control.AuthorizeParams{Display: display}, nil)
			}
		}
		if err := srv.Serve(context.Background(), os.Stdin, os.Stdout); err != nil {
			os.Exit(exitError)
		}
	case "version", "-v", "--version":
		fmt.Println("clawdyx", version)
	case "help", "-h", "--help":
		usage()
	default:
		usage()
		os.Exit(exitConfig)
	}
}

// migrateLegacy moves an earlier release's settings to this OS's standard
// folder (Windows 0.4.x: %USERPROFILE%\.config\clawdyx) and returns the
// config path to use from now on. It runs only while no worker uses the
// old file. If the move fails, the old file keeps working.
func migrateLegacy(cfgPath string) string {
	moved, from, err := config.MigrateDefault(cfgPath)
	switch {
	case err != nil:
		fmt.Fprintln(os.Stderr, "note: the settings stay in", filepath.Dir(from)+"; moving them failed:", observability.Redact(err.Error()))
	case moved:
		fmt.Printf("moved the settings from %s to %s (the stored token too)\n", filepath.Dir(from), filepath.Dir(config.StandardPath()))
		return config.StandardPath()
	}
	return cfgPath
}

func loadConfigOnly(path string) (*config.Config, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !config.Private(st) {
		return nil, fmt.Errorf("%s must not be readable by group or others (chmod 600)", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return config.Parse(b)
}

// run serves the bot until it is stopped. background reports whether it
// ran without a console (Windows only; see detachConsole).
func run(cfgPath, envPath string) (code int, background bool) {
	cfg, err := config.Load(cfgPath, envPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, observability.Redact(err.Error()))
		fmt.Fprintln(os.Stderr, "Run `clawdyx setup` (or `make setup`) to create the configuration.")
		return exitConfig, false
	}
	background = detachConsole(cfg.LogDir)
	if err := os.MkdirAll(cfg.StateDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "state dir:", err)
		return exitError, background
	}
	_ = os.Chmod(cfg.StateDir, 0o700)
	logFile, err := observability.OpenRotating(filepath.Join(cfg.LogDir, "bot.log"), 20<<20, 5)
	if err != nil {
		fmt.Fprintln(os.Stderr, "log file:", err)
		return exitError, background
	}
	defer logFile.Close()
	var console io.Writer = os.Stderr
	if background {
		console = io.Discard // no console; a failing writer would stop io.MultiWriter
	}
	log := observability.NewLogger(io.MultiWriter(console, logFile), cfg.Logging.Level)

	lock := flock.New(filepath.Join(cfg.StateDir, "bot.lock"))
	locked, err := lock.TryLock()
	if err != nil || !locked {
		log.Error("another clawdyx instance is running (state lock held); exiting")
		return exitLocked, background
	}
	defer lock.Unlock()
	defer writePID(cfg.StateDir)()

	if os.Getenv("ANTHROPIC_API_KEY") != "" || os.Getenv("CLAUDE_CODE_OAUTH_TOKEN") != "" {
		log.Warn("an Anthropic credential variable is set in the bot environment; it is NOT passed to claude (the bot uses the local claude.ai login only)")
	}

	st, err := store.Open(filepath.Join(cfg.StateDir, "state.db"))
	if err != nil {
		log.Error("open state database", "err", err)
		return exitError, background
	}
	defer st.Close()
	if from := config.TakeMigratedMarker(cfgPath); from != "" {
		_ = st.SetSetting("migrated_from", from)
		log.Info("settings were moved to the standard folder", "from", from, "to", filepath.Dir(cfgPath))
	}

	app, err := telegram.New(cfg, st, log, telegram.Options{})
	if err != nil {
		log.Error("start telegram", "err", observability.Redact(err.Error()))
		return exitError, background
	}
	if abs, err := filepath.Abs(cfgPath); err == nil {
		app.ConfigPath = abs
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv := control.NewServer(log)
	app.RegisterControl(srv)
	if err := srv.Listen(cfg.ControlSocket()); err != nil {
		// The bot still works without the control channel; the desktop app does not.
		log.Error("control socket unavailable", "err", err)
	} else {
		go func() {
			if err := srv.Serve(ctx); err != nil {
				log.Error("control socket stopped", "err", err)
			}
		}()
		defer srv.Close()
	}
	if err := app.Run(ctx); err != nil {
		log.Error("run", "err", observability.Redact(err.Error()))
		return exitError, background
	}
	select {
	case <-app.RestartRequested:
		// A non-zero exit makes launchd start the worker again (KeepAlive
		// with SuccessfulExit=false; on Windows see relaunch); in a
		// terminal, start it again by hand.
		log.Info("stopped for a restart")
		return exitRestart, background
	default:
	}
	log.Info("stopped")
	return exitOK, background
}

func check(cfgPath, envPath string) int {
	failed := false
	line := func(ok bool, format string, a ...any) {
		mark := "OK  "
		if !ok {
			mark, failed = "FAIL", true
		}
		fmt.Printf("[%s] %s\n", mark, fmt.Sprintf(format, a...))
	}
	cfg, err := config.Load(cfgPath, envPath)
	if err != nil {
		line(false, "config: %s", observability.Redact(err.Error()))
		return exitConfig
	}
	line(true, "config %s (owner %d, %d workspace(s), default %s)", cfgPath, cfg.Telegram.OwnerUserID, len(cfg.Workspaces), cfg.DefaultWorkspace)
	line(true, "token loaded from %s (value not shown)", map[string]string{"keychain": platform.W.TokenStore, "file": envPath, "env": "the TELEGRAM_BOT_TOKEN variable"}[cfg.TokenSource])
	if cfg.TokenSource == "file" {
		fmt.Println("       tip: clawdyx setup -migrate-token moves it into " + platform.W.TokenStore)
	}
	_ = os.MkdirAll(cfg.StateDir, 0o700)
	cli := agent.CLI{Binary: cfg.Claude.Binary, Env: agent.ChildEnv(cfg.Claude.ChildPath, 0), Dir: cfg.StateDir}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	v, err := cli.Version(ctx)
	line(err == nil, "claude %s version %s %v", cfg.Claude.Binary, v, errString(err))
	fmt.Println("       checking claude auth status (can take ~30 s)…")
	a, err := cli.AuthStatus(ctx)
	if err != nil {
		line(false, "claude auth status: %s", observability.Redact(err.Error()))
	} else {
		line(a.LoggedIn, "claude auth: loggedIn=%v method=%s provider=%s subscription=%s", a.LoggedIn, a.AuthMethod, a.APIProvider, a.SubscriptionType)
	}
	b, err := bot.New(cfg.Token, bot.WithSkipGetMe())
	if err == nil {
		me, err := b.GetMe(ctx)
		if err != nil {
			line(false, "telegram getMe: %s", observability.Redact(err.Error()))
		} else {
			line(true, "telegram bot @%s (id %d)", me.Username, me.ID)
		}
		if info, err := b.GetWebhookInfo(ctx); err == nil {
			line(info.URL == "", "telegram webhook: %s", map[bool]string{true: "none (long polling ready)", false: "SET — 'run' will delete it"}[info.URL == ""])
		}
	} else {
		line(false, "telegram client: %s", observability.Redact(err.Error()))
	}
	lock := flock.New(filepath.Join(cfg.StateDir, "bot.lock"))
	if ok, _ := lock.TryLock(); ok {
		_ = lock.Unlock()
		line(true, "no running instance holds the state lock")
	} else {
		fmt.Println("[INFO] a running instance holds the state lock")
	}
	if failed {
		return exitError
	}
	return exitOK
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return "(" + observability.Redact(err.Error()) + ")"
}

func backup(cfgPath string) int {
	cfg, err := loadConfigOnly(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, observability.Redact(err.Error()))
		return exitConfig
	}
	st, err := store.Open(filepath.Join(cfg.StateDir, "state.db"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		return exitError
	}
	defer st.Close()
	dir := filepath.Join(cfg.StateDir, "backups")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitError
	}
	_ = os.Chmod(dir, 0o700)
	out := filepath.Join(dir, "state-"+time.Now().Format("20060102-150405")+".db")
	if err := st.Backup(context.Background(), out); err != nil {
		fmt.Fprintln(os.Stderr, "backup:", err)
		return exitError
	}
	files, _ := filepath.Glob(filepath.Join(dir, "state-*.db"))
	sort.Strings(files)
	for len(files) > 7 {
		_ = os.Remove(files[0])
		files = files[1:]
	}
	fmt.Println("backup written:", out)
	return exitOK
}

var errNoSub = errors.New("usage: clawdyx service start|stop|restart|status|install|where|uninstall")
