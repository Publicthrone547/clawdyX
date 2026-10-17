package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"golang.org/x/term"

	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/observability"
)

// setupOptions are the non-interactive inputs of `clawdyx setup`. Anything
// left empty is asked for interactively unless Yes is set.
type setupOptions struct {
	OwnerID      int64
	Workspace    string
	Browser      string
	ClaudeBinary string
	TokenStdin   bool // read the token from the first line of stdin (implies Yes)
	Force        bool // replace existing files without asking
	NoCheck      bool // skip the read-only checks at the end
	Yes          bool // never prompt: use flags and defaults, fail when something is missing
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

// getMeFunc verifies a token with Telegram and returns the bot's username.
// Tests replace it.
var getMeFunc = func(ctx context.Context, token string) (string, error) {
	b, err := bot.New(token, bot.WithSkipGetMe())
	if err != nil {
		return "", err
	}
	me, err := b.GetMe(ctx)
	if err != nil {
		return "", err
	}
	return me.Username, nil
}

// runSetup is the guided first-time setup. It writes the private config and
// token files, never prints the token, and ends with the read-only checks.
func runSetup(cfgPath, envPath string, o setupOptions, in io.Reader, out io.Writer) int {
	w := &wizard{in: bufio.NewReader(in), out: out, yes: o.Yes || o.TokenStdin}
	if f, ok := in.(*os.File); ok && !o.TokenStdin && term.IsTerminal(int(f.Fd())) {
		w.tty = f
	}
	w.say("clawdyX setup — writes %s and %s (private, mode 0600).", cfgPath, envPath)
	w.say("Nothing leaves this Mac except one call to Telegram's Bot API to verify the token.")
	w.say("")

	cfgExists, envExists := fileExists(cfgPath), fileExists(envPath)
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
			w.say("A bot token is already stored in %s; pass -force to replace it.", envPath)
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
		t, err := config.LoadToken(envPath)
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
		if browser == "" {
			if browser, err = w.ask("Browser app used by /open", detectBrowser()); err != nil {
				return exitConfig
			}
		}
		claudeBin := o.ClaudeBinary
		if claudeBin == "" {
			claudeBin = detectClaude()
		}
		if claudeBin == "" {
			w.say("The claude CLI was not found. Install Claude Code (brew install --cask claude-code), run `claude` once to log in, then rerun setup. Pass -claude-binary PATH if it lives elsewhere.")
			return exitConfig
		}
		data, err := json.MarshalIndent(generateConfig(owner, ws, browser, claudeBin), "", "  ")
		if err != nil {
			w.say("internal error: %v", err)
			return exitError
		}
		data = append(data, '\n')
		if _, err := config.Parse(data); err != nil {
			w.say("The generated configuration is not valid:\n%s", observability.Redact(err.Error()))
			return exitConfig
		}
		if err := writePrivate(cfgPath, data); err != nil {
			w.say("write config: %v", err)
			return exitError
		}
		w.say("✓ Wrote %s (owner %d, workspace %s)", cfgPath, owner, ws)
	}
	if !keepToken {
		if err := writePrivate(envPath, []byte("TELEGRAM_BOT_TOKEN="+token+"\n")); err != nil {
			w.say("write token file: %v", err)
			return exitError
		}
		w.say("✓ Stored the token in %s (it is never shown again)", envPath)
	}
	if !o.NoCheck {
		w.say("\nRunning the read-only checks (claude auth status can take about 30 s)…")
		if check(cfgPath, envPath) != exitOK {
			w.say("\nSome checks failed. Fix them, then run `clawdyx check` (or `make doctor`) again.")
		}
	}
	w.say("\nDone. Next:\n  make start      run the bot in the background now and at every login\n  make run        or run it in the foreground (Ctrl-C stops it)\nThen open Telegram and send /start to @%s.", botName)
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

type tgUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

type tgUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		From *tgUser `json:"from"`
		Chat struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		} `json:"chat"`
	} `json:"message"`
}

// tgGetUpdates fetches pending updates without long polling. offset > 0
// acknowledges every earlier update.
func tgGetUpdates(ctx context.Context, token string, offset int64) ([]tgUpdate, error) {
	params := map[string]any{"timeout": 0, "allowed_updates": []string{"message"}}
	if offset > 0 {
		params["offset"] = offset
	}
	body, _ := json.Marshal(params)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+token+"/getUpdates", bytes.NewReader(body))
	if err != nil {
		return nil, errors.New(observability.Redact(err.Error()))
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.New(observability.Redact(err.Error()))
	}
	defer resp.Body.Close()
	var out struct {
		OK          bool       `json:"ok"`
		Result      []tgUpdate `json:"result"`
		Description string     `json:"description"`
		ErrorCode   int        `json:"error_code"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("telegram: unreadable response (HTTP %d)", resp.StatusCode)
	}
	if !out.OK {
		return nil, fmt.Errorf("telegram: %s (error %d)", observability.Redact(out.Description), out.ErrorCode)
	}
	return out.Result, nil
}

// detectOwner finds the owner's ID from a message they send to the bot, then
// acknowledges the pending updates so they are not replayed as prompts.
func (w *wizard) detectOwner(ctx context.Context, token string) (int64, error) {
	w.say("Open Telegram, send any message to your bot, then press Enter here.")
	if _, err := w.ask("Press Enter when the message is sent", ""); err != nil {
		return 0, err
	}
	ups, err := tgGetUpdates(ctx, token, 0)
	if err != nil {
		return 0, err
	}
	seen := map[int64]string{}
	var maxID int64
	for _, u := range ups {
		if u.UpdateID > maxID {
			maxID = u.UpdateID
		}
		m := u.Message
		if m == nil || m.From == nil || m.From.IsBot || m.Chat.Type != "private" || m.Chat.ID != m.From.ID {
			continue
		}
		if _, ok := seen[m.From.ID]; !ok {
			label := m.From.FirstName
			if m.From.Username != "" {
				label += " @" + m.From.Username
			}
			seen[m.From.ID] = strings.TrimSpace(label)
		}
	}
	if len(seen) == 0 {
		return 0, errors.New("no private message to the bot was found; send one and try again, or type the ID")
	}
	ids := make([]int64, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var chosen int64
	if len(ids) == 1 {
		w.say("Found one sender: %d (%s)", ids[0], seen[ids[0]])
		if !w.yesNo("Use this ID as the owner?", true) {
			return 0, errors.New("not confirmed")
		}
		chosen = ids[0]
	} else {
		w.say("Several people have messaged the bot:")
		for i, id := range ids {
			w.say("  %d) %d (%s)", i+1, id, seen[id])
		}
		ans, err := w.ask("Which one is you? (number)", "")
		if err != nil {
			return 0, err
		}
		n, err := strconv.Atoi(ans)
		if err != nil || n < 1 || n > len(ids) {
			return 0, errors.New("invalid choice")
		}
		chosen = ids[n-1]
	}
	if _, err := tgGetUpdates(ctx, token, maxID+1); err == nil {
		w.say("Cleared %d pending update(s) so the bot starts clean.", len(ups))
	}
	return chosen, nil
}

func (w *wizard) chooseWorkspace(flag string) (string, error) {
	def := flag
	if def == "" {
		if cwd, err := os.Getwd(); err == nil && workspaceOK(cwd) == nil {
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
		p = expandHome(p)
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		if err := workspaceOK(p); err != nil {
			if w.yes || flag != "" {
				return "", err
			}
			w.say("%s", err)
			continue
		}
		if warn := privacyWarning(p); warn != "" {
			w.say("Note: %s", warn)
		}
		return p, nil
	}
}

func workspaceOK(p string) error {
	if p == "" {
		return errors.New("a project folder is required")
	}
	if !filepath.IsAbs(p) {
		return fmt.Errorf("%s is not an absolute path", p)
	}
	st, err := os.Stat(p)
	if err != nil {
		return fmt.Errorf("%v", err)
	}
	if !st.IsDir() {
		return fmt.Errorf("%s is not a directory", p)
	}
	canon, err := filepath.EvalSymlinks(p)
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	if canon == "/" || canon == filepath.Clean(home) {
		return errors.New("the home folder and the filesystem root cannot be workspaces; pick a project folder such as ~/projects/<name>")
	}
	return nil
}

// privacyWarning flags folders that macOS privacy protection can hide from a
// background (launchd) process.
func privacyWarning(p string) string {
	home, _ := os.UserHomeDir()
	for _, d := range []string{"Desktop", "Documents", "Downloads"} {
		root := filepath.Join(home, d)
		if p == root || strings.HasPrefix(p, root+string(filepath.Separator)) {
			return fmt.Sprintf("%s is protected by macOS privacy settings; the background service may be denied access there. A folder such as ~/projects/<name> avoids that.", root)
		}
	}
	return ""
}

func detectBrowser() string {
	for _, c := range [][2]string{{"/Applications/Firefox.app", "Firefox"}, {"/Applications/Google Chrome.app", "Google Chrome"}, {"/Applications/Brave Browser.app", "Brave Browser"}, {"/Applications/Arc.app", "Arc"}} {
		if st, err := os.Stat(c[0]); err == nil && st.IsDir() {
			return c[1]
		}
	}
	return "Safari"
}

func detectClaude() string {
	if p, err := exec.LookPath("claude"); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
		return p
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{"/opt/homebrew/bin/claude", "/usr/local/bin/claude", filepath.Join(home, ".local", "bin", "claude")} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

var wsNameClean = regexp.MustCompile(`[^a-z0-9_-]+`)

// workspaceName derives a config-safe workspace name from a folder path.
func workspaceName(path string) string {
	n := strings.ToLower(filepath.Base(filepath.Clean(path)))
	n = wsNameClean.ReplaceAllString(n, "-")
	n = strings.Trim(n, "-_")
	if len(n) > 32 {
		n = strings.TrimRight(n[:32], "-_")
	}
	if n == "" {
		return "workspace"
	}
	return n
}

// generateConfig builds the default configuration for one workspace.
func generateConfig(owner int64, wsPath, browser, claudeBin string) *config.Config {
	open := true
	name := workspaceName(wsPath)
	childPath := "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
	if dir := filepath.Dir(claudeBin); !strings.Contains(":"+childPath+":", ":"+dir+":") {
		childPath = dir + ":" + childPath
	}
	return &config.Config{
		Telegram: config.Telegram{OwnerUserID: owner, PollTimeoutSeconds: 30},
		Claude: config.Claude{
			Binary: claudeBin, DefaultModel: "fable", DefaultEffort: "auto", SettingSources: "user", StrictMCPConfig: true,
			JobTimeout: config.Duration{Duration: 15 * time.Minute}, MaxTurns: 60, MaxConcurrentJobs: 2, MaxQueuePerSession: 5,
			MaxOutputBytes: 1 << 20, MaxChildRSSMB: 2048, BgWaitCeilingMS: 60000, ChildPath: childPath, ExtraDisallowedTools: []string{},
		},
		Workspaces: []config.Workspace{{
			Name: name, Path: wsPath, DefaultMode: config.ModeObserve,
			AllowedCommands: []string{"Bash(git status *)", "Bash(git diff *)", "Bash(git log *)", "Bash(make test *)", "Bash(go test *)", "Bash(go build *)", "Bash(go vet *)"},
			DeniedCommands:  []string{"Bash(rm *)", "Bash(rmdir *)", "Bash(sudo *)", "Bash(git push *)", "Bash(git remote *)", "Bash(curl *)", "Bash(wget *)", "Bash(brew *)", "Bash(claude *)"},
		}},
		DefaultWorkspace: name,
		Desktop:          config.Desktop{OpenURLs: &open, BrowserApp: browser},
		Logging:          config.Logging{Level: "info"},
		Retention:        config.Retention{JobsDays: 90, JobEventsDays: 14, ProcessedUpdatesDays: 7, AuditDays: 180},
	}
}

// writePrivate writes data to path atomically with mode 0600 inside a 0700 directory.
func writePrivate(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	_ = os.Chmod(dir, 0o700)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if err := tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(data)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Rename(name, path)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
