package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/observability"
	"github.com/Publicthrone547/clawdyX/internal/platform"
	"github.com/Publicthrone547/clawdyX/internal/setup"
	"github.com/Publicthrone547/clawdyX/internal/vault"
)

// setupOptions are the non-interactive inputs of `clawdyx setup`. Anything
// left empty is asked for interactively unless Yes is set.
type setupOptions struct {
	OwnerID      int64
	Workspace    string
	Browser      string
	ClaudeBinary string
	TokenStdin   bool // read the token from the first line of stdin (implies Yes)
	TokenFile    bool // keep the token in the 0600 .env file instead of the token store
	MigrateToken bool // move an existing .env token into the token store and stop
	Force        bool // replace existing files without asking
	NoCheck      bool // skip the read-only checks at the end
	Yes          bool // never prompt: use flags and defaults, fail when something is missing
	Headless     bool // a server without a desktop: no browser, /open off, the service steps for it
}

// wizard bundles the I/O of one setup run so tests can drive it.
type wizard struct {
	in  *bufio.Reader
	out io.Writer
	tty *os.File // set when input is an interactive terminal (hidden token entry)
	yes bool
}

func (w *wizard) say(format string, a ...any) { fmt.Fprintf(w.out, format+"\n", a...) }

// ask prints a question and returns the trimmed answer, or def when empty.
func (w *wizard) ask(label, def string) (string, error) {
	if w.yes {
		return def, nil
	}
	if def != "" {
		fmt.Fprintf(w.out, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(w.out, "%s: ", label)
	}
	line, err := w.in.ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("no input: %w", err)
	}
	if line = strings.TrimSpace(line); line != "" {
		return line, nil
	}
	return def, nil
}

func (w *wizard) yesNo(label string, def bool) bool {
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	ans, err := w.ask(label+" ("+hint+")", "")
	if err != nil || ans == "" {
		return def
	}
	switch strings.ToLower(ans) {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	}
	return def
}

// secret reads one line without echo when attached to a terminal.
func (w *wizard) secret(label string) (string, error) {
	fmt.Fprintf(w.out, "%s: ", label)
	if w.tty != nil {
		b, err := term.ReadPassword(int(w.tty.Fd()))
		fmt.Fprintln(w.out)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	line, err := w.in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// getMeFunc verifies a token with Telegram (tests replace it).
var getMeFunc = setup.GetMe

// runSetup is the guided first-time setup. It writes the private config and
// token files, never prints the token, and ends with the read-only checks.
func runSetup(cfgPath, envPath string, o setupOptions, in io.Reader, out io.Writer) int {
	w := &wizard{in: bufio.NewReader(in), out: out, yes: o.Yes || o.TokenStdin}
	if o.MigrateToken {
		return migrateToken(cfgPath, envPath, out)
	}
	if f, ok := in.(*os.File); ok && !o.TokenStdin && term.IsTerminal(int(f.Fd())) {
		w.tty = f
	}
	// A Linux machine without a graphical session is a server.
	headless := o.Headless || platform.Current().Headless
	noKeyring := !o.TokenFile && !vault.Available()
	if noKeyring {
		o.TokenFile = true
	}
	where := platform.W.TokenStore + " (item \"" + vault.Service + "\")"
	if o.TokenFile {
		where = envPath + " (mode 0600)"
	}
	w.say("clawdyX setup — writes %s (private, mode 0600) and stores the bot token in %s.", cfgPath, where)
	if noKeyring {
		w.say("%s", platform.W.NoKeyringNote)
	}
	w.say("Nothing leaves %s except one call to Telegram's Bot API to verify the token.", platform.W.ThisComputer)
	w.say("")

	_, verr := config.StoredToken(cfgPath)
	cfgExists, envExists := fileExists(cfgPath), fileExists(envPath) || verr == nil
	keepConfig, keepToken := false, false
	if cfgExists && !o.Force {
		if w.yes {
			keepConfig = true
		} else {
			keepConfig = !w.yesNo(cfgPath+" already exists. Replace it? (workspaces and settings are regenerated)", false)
		}
	}
	if envExists && !o.Force {
		if o.TokenStdin {
			w.say("A bot token is already stored; pass -force to replace it.")
			return exitConfig
		}
		if w.yes {
			keepToken = true
		} else {
			keepToken = !w.yesNo("A bot token is already stored. Replace it?", false)
		}
	}

	var token string
	if keepToken {
		t, err := config.StoredToken(cfgPath)
		if err != nil {
			t, err = config.LoadToken(envPath)
		}
		if err != nil {
			w.say("The stored token cannot be used: %s", observability.Redact(err.Error()))
			return exitConfig
		}
		token = t
	} else {
		t, err := w.obtainToken(o)
		if err != nil {
			w.say("%s", observability.Redact(err.Error()))
			return exitConfig
		}
		token = t
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	botName, err := getMeFunc(ctx, token)
	if err != nil {
		w.say("Telegram rejected the token: %s", observability.Redact(err.Error()))
		return exitConfig
	}
	w.say("✓ Token accepted: bot @%s", botName)

	if keepConfig {
		w.say("Keeping the existing %s.", cfgPath)
	} else {
		owner := o.OwnerID
		for owner <= 0 {
			if w.yes {
				w.say("-owner-id N is required in non-interactive mode (your numeric Telegram user ID).")
				return exitConfig
			}
			ans, err := w.ask("Your numeric Telegram user ID (leave empty to detect it from a message you send to the bot)", "")
			if err != nil {
				return exitConfig
			}
			if ans == "" {
				id, err := w.detectOwner(ctx, token)
				if err != nil {
					w.say("Detection did not work: %s", observability.Redact(err.Error()))
					continue
				}
				owner = id
				continue
			}
			id, err := strconv.ParseInt(ans, 10, 64)
			if err != nil || id <= 0 {
				w.say("A Telegram user ID is a positive number such as 123456789.")
				continue
			}
			owner = id
		}
		ws, err := w.chooseWorkspace(o.Workspace)
		if err != nil {
			w.say("%s", err)
			return exitConfig
		}
		browser := o.Browser
		if browser == "" && !headless {
			if browser, err = w.ask("Browser app used by /open", setup.DetectBrowser()); err != nil {
				return exitConfig
			}
		}
		claudeBin := o.ClaudeBinary
		if claudeBin == "" {
			claudeBin = setup.DetectClaude()
		}
		if claudeBin == "" {
			w.say("The claude CLI was not found. Install Claude Code (%s), run `claude` once to log in, then rerun setup. Pass -claude-binary PATH if it lives elsewhere.", platform.W.InstallClaude)
			return exitConfig
		}
		gen := setup.GenerateConfig(owner, ws, browser, claudeBin)
		if headless {
			off := false
			gen.Desktop.OpenURLs, gen.Desktop.BrowserApp = &off, ""
		}
		if err := setup.WriteConfig(cfgPath, gen); err != nil {
			w.say("Could not write the configuration:\n%s", observability.Redact(err.Error()))
			return exitConfig
		}
		w.say("✓ Wrote %s (owner %d, workspace %s)", cfgPath, owner, ws)
	}
	if !keepToken {
		if o.TokenFile {
			if err := setup.WritePrivate(envPath, []byte("TELEGRAM_BOT_TOKEN="+token+"\n")); err != nil {
				w.say("write token file: %v", err)
				return exitError
			}
			_ = config.DeleteStoredToken(cfgPath)
			w.say("✓ Stored the token in %s (it is never shown again)", envPath)
		} else {
			if err := storeAndVerify(cfgPath, token); err != nil {
				w.say("Could not store the token in %s: %s. Rerun with -token-file to keep it in a private file instead.", platform.W.TokenStore, observability.Redact(err.Error()))
				return exitError
			}
			if fileExists(envPath) {
				_ = os.Remove(envPath)
				w.say("✓ Removed the old token file %s", envPath)
			}
			w.say("✓ Stored the token in %s (item %q; it is never shown again)", platform.W.TokenStore, vault.Service)
		}
	}
	if !o.NoCheck {
		w.say("\nRunning the read-only checks (claude auth status can take about 30 s)…")
		if check(cfgPath, envPath) != exitOK {
			w.say("\nSome checks failed. Fix them, then run `clawdyx check` (or `make doctor`) again.")
		}
	}
	w.say("\nDone. Next:\n  %s\nThen open Telegram and send /start to @%s.", nextSteps(headless), botName)
	return exitOK
}

func (w *wizard) obtainToken(o setupOptions) (string, error) {
	if o.TokenStdin {
		line, err := w.in.ReadString('\n')
		if err != nil && line == "" {
			return "", errors.New("no token on stdin")
		}
		t := strings.TrimSpace(line)
		if !config.ValidToken(t) {
			return "", errors.New("the token on stdin does not look like a Telegram bot token")
		}
		return t, nil
	}
	if env := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")); env != "" && config.ValidToken(env) {
		if w.yes || w.yesNo("TELEGRAM_BOT_TOKEN is set in this shell. Use it?", true) {
			return env, nil
		}
	}
	if w.yes {
		return "", errors.New("no token: pipe it with -token-stdin or set TELEGRAM_BOT_TOKEN")
	}
	w.say("Create a bot with @BotFather in Telegram (/newbot) and copy its token.")
	for i := 0; i < 3; i++ {
		t, err := w.secret("Bot token (input is hidden)")
		if err != nil {
			return "", err
		}
		if config.ValidToken(t) {
			return t, nil
		}
		w.say("That does not look like a bot token (expected 123456789:AAAA…, 35 characters after the colon).")
	}
	return "", errors.New("no valid token entered")
}

// detectOwner finds the owner's ID from a message they send to the bot, then
// acknowledges the pending updates so they are not replayed as prompts.
func (w *wizard) detectOwner(ctx context.Context, token string) (int64, error) {
	w.say("Open Telegram, send any message to your bot, then press Enter here.")
	if _, err := w.ask("Press Enter when the message is sent", ""); err != nil {
		return 0, err
	}
	senders, n, err := setup.DetectSenders(ctx, token)
	if err != nil {
		return 0, err
	}
	if len(senders) == 0 {
		return 0, errors.New("no private message to the bot was found; send one and try again, or type the ID")
	}
	label := func(s setup.Sender) string {
		l := s.FirstName
		if s.Username != "" {
			l += " @" + s.Username
		}
		return strings.TrimSpace(l)
	}
	var chosen int64
	if len(senders) == 1 {
		w.say("Found one sender: %d (%s)", senders[0].ID, label(senders[0]))
		if !w.yesNo("Use this ID as the owner?", true) {
			return 0, errors.New("not confirmed")
		}
		chosen = senders[0].ID
	} else {
		w.say("Several people have messaged the bot:")
		for i, s := range senders {
			w.say("  %d) %d (%s)", i+1, s.ID, label(s))
		}
		ans, err := w.ask("Which one is you? (number)", "")
		if err != nil {
			return 0, err
		}
		k, err := strconv.Atoi(ans)
		if err != nil || k < 1 || k > len(senders) {
			return 0, errors.New("invalid choice")
		}
		chosen = senders[k-1].ID
	}
	w.say("Cleared %d pending update(s) so the bot starts clean.", n)
	return chosen, nil
}

func (w *wizard) chooseWorkspace(flag string) (string, error) {
	def := flag
	if def == "" {
		if cwd, err := os.Getwd(); err == nil && setup.WorkspaceOK(cwd) == nil {
			def = cwd
		}
	}
	for {
		p := flag
		if p == "" {
			var err error
			if p, err = w.ask("Project folder Claude may work in (absolute path)", def); err != nil {
				return "", err
			}
		}
		p = setup.ExpandHome(p)
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		if err := setup.WorkspaceOK(p); err != nil {
			if w.yes || flag != "" {
				return "", err
			}
			w.say("%s", err)
			continue
		}
		if warn := setup.PrivacyWarning(p); warn != "" {
			w.say("Note: %s", warn)
		}
		return p, nil
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// storeAndVerify saves the token in the vault and reads it back.
func storeAndVerify(cfgPath, token string) error {
	if err := config.StoreToken(cfgPath, token); err != nil {
		return err
	}
	got, err := config.StoredToken(cfgPath)
	if err != nil {
		return err
	}
	if got != token {
		return errors.New("the token read back from " + platform.W.TokenStore + " differs")
	}
	return nil
}

// migrateToken moves a token from the .env file into the token store. The file
// is deleted only after the stored copy was read back successfully.
func migrateToken(cfgPath, envPath string, out io.Writer) int {
	tok, err := config.LoadToken(envPath)
	if err != nil {
		fmt.Fprintln(out, "No usable token file to migrate:", observability.Redact(err.Error()))
		return exitConfig
	}
	if err := storeAndVerify(cfgPath, tok); err != nil {
		fmt.Fprintln(out, "Not migrated; the token file is unchanged:", observability.Redact(err.Error()))
		return exitError
	}
	if err := os.Remove(envPath); err != nil {
		fmt.Fprintln(out, "The token is in "+platform.W.TokenStore+", but the old file could not be removed:", err)
		return exitError
	}
	fmt.Fprintf(out, "✓ Moved the bot token into %s (item %q) and removed %s.\nRestart the worker to use it: clawdyx service restart\n", platform.W.TokenStore, vault.Service, envPath)
	return exitOK
}

// nextSteps is the end of setup: how to start the worker on this OS, and on
// a server how to keep it running without a login.
func nextSteps(headless bool) string {
	if headless && platform.IsLinux {
		return "clawdyx service start           install and start the systemd user service\n" +
			"  loginctl enable-linger $USER    once: keep it running from boot, without a login\n" +
			"  clawdyx service status          check it (logs: journalctl --user -u clawdyx)"
	}
	return platform.W.SetupNext
}
