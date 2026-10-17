package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/Publicthrone547/clawdyX/internal/agent"
	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/jobs"
	"github.com/Publicthrone547/clawdyX/internal/observability"
	"github.com/Publicthrone547/clawdyX/internal/processes"
	"github.com/Publicthrone547/clawdyX/internal/screen"
	"github.com/Publicthrone547/clawdyX/internal/sessions"
	"github.com/Publicthrone547/clawdyX/internal/store"
)

// Version is the bot version (set by main).
var Version = "dev"

// MaxPromptBytes rejects very long prompts (ask for a file instead).
const MaxPromptBytes = 32 << 10

const keyLastUpdate = "last_update_id"
const keyCLIVersion = "cli_version_seen"

// App is the running bot.
type App struct {
	cfg      *config.Config
	st       *store.Store
	sess     *sessions.Service
	jobs     *jobs.Manager
	cli      agent.CLI
	agents   *processes.AgentsCache
	log      *slog.Logger
	counters *observability.Counters
	b        *bot.Bot
	send     *Sender
	outbox   *Outbox
	confirms *Confirms
	streams  *streams
	started  time.Time
	opener   func(ctx context.Context, app, url string) error
	desktop  screen.Desktop

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	authMu  sync.Mutex
	auth    *agent.AuthInfo
	authAt  time.Time
	authErr error

	verMu      sync.Mutex
	cliVersion string

	errMu      sync.Mutex
	lastErr    string
	lastErrAt  time.Time
	lastUpdate time.Time

	// ShutdownRequested is closed when /shutdown was confirmed.
	ShutdownRequested chan struct{}
	shutdownOnce      sync.Once

	poll        poller
	events      atomic.Pointer[func(typ string, data any)]
	nameMu      sync.Mutex
	botUsername string
	webhookGone bool
}

// SetEventSink receives worker events (the control server's Publish).
func (a *App) SetEventSink(f func(typ string, data any)) { a.events.Store(&f) }

func (a *App) emit(typ string, data any) {
	if f := a.events.Load(); f != nil && *f != nil {
		(*f)(typ, data)
	}
}

func (a *App) setBotUsername(u string) {
	a.nameMu.Lock()
	a.botUsername = u
	a.nameMu.Unlock()
}

// BotUsername returns the bot's @username once known.
func (a *App) BotUsername() string {
	a.nameMu.Lock()
	defer a.nameMu.Unlock()
	return a.botUsername
}

// Options configure New.
type Options struct {
	ServerURL string // Telegram API base URL override (tests)
	Start     jobs.StartFunc
	SkipGetMe bool
	// Opener launches a URL in a macOS app (replaceable in tests).
	Opener func(ctx context.Context, app, url string) error
	// Desktop takes screenshots for /screenshot (replaceable in tests).
	Desktop screen.Desktop
}

// New builds the app and connects to Telegram (getMe).
func New(cfg *config.Config, st *store.Store, log *slog.Logger, opt Options) (*App, error) {
	a := &App{cfg: cfg, st: st, log: log, counters: &observability.Counters{}, started: time.Now(),
		confirms: NewConfirms(), ShutdownRequested: make(chan struct{})}
	a.sess = sessions.New(st, cfg)
	a.cli = agent.CLI{Binary: cfg.Claude.Binary, Env: agent.ChildEnv(cfg.Claude.ChildPath, 0), Dir: cfg.StateDir}
	a.agents = processes.NewAgentsCache(a.cli)
	a.streams = newStreams()

	last, _ := strconv.ParseInt(st.Setting(keyLastUpdate, "0"), 10, 64)
	bopts := []bot.Option{
		bot.WithDefaultHandler(a.handle),
		bot.WithNotAsyncHandlers(),
		bot.WithAllowedUpdates(AllowedUpdates),
		bot.WithErrorsHandler(a.onPollError),
		// The library uses pollTimeout-1s as the getUpdates timeout; the HTTP
		// client must outlive the long poll.
		bot.WithHTTPClient(time.Duration(cfg.Telegram.PollTimeoutSeconds+1)*time.Second,
			&http.Client{Timeout: time.Duration(cfg.Telegram.PollTimeoutSeconds+15) * time.Second}),
		bot.WithCheckInitTimeout(15 * time.Second),
	}
	if last > 0 {
		bopts = append(bopts, bot.WithInitialOffset(last))
	}
	if opt.ServerURL != "" {
		bopts = append(bopts, bot.WithServerURL(opt.ServerURL))
	}
	if opt.SkipGetMe {
		bopts = append(bopts, bot.WithSkipGetMe())
	}
	b, err := bot.New(cfg.Token, bopts...)
	if err != nil {
		return nil, fmt.Errorf("telegram: %s", observability.Redact(err.Error()))
	}
	a.b = b
	a.send = NewSender(b, cfg.Telegram.OwnerUserID, log, a.counters)
	a.outbox = NewOutbox(a.send, log, 200)
	a.jobs = jobs.New(cfg, st, a.sess, log, a, opt.Start)
	a.opener = opt.Opener
	if a.opener == nil {
		a.opener = openURL
	}
	if exe, err := os.Executable(); err == nil {
		if exe, err = filepath.EvalSymlinks(exe); err == nil {
			a.jobs.ScreenCommand = exe
		}
	}
	a.desktop = opt.Desktop
	if a.desktop == nil {
		a.desktop = screen.Mac{TmpDir: os.TempDir()}
	}
	return a, nil
}

// Jobs exposes the job manager (tests).
func (a *App) Jobs() *jobs.Manager { return a.jobs }

func (a *App) onPollError(err error) {
	if errors.Is(err, bot.ErrorConflict) {
		a.counters.Conflicts409.Add(1)
		a.counters.LastConflict.Store(time.Now().Unix())
	} else {
		a.counters.PollErrors.Add(1)
	}
	msg := observability.Truncate(observability.Redact(err.Error()), 200)
	a.setErr("telegram: " + msg)
	a.log.Warn("telegram poll error", "err", msg)
}

func (a *App) setErr(msg string) {
	a.errMu.Lock()
	a.lastErr, a.lastErrAt = msg, time.Now()
	a.errMu.Unlock()
}

func (a *App) go_(f func()) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		f()
	}()
}

// Run starts polling and background work; it returns when ctx ends and
// everything has shut down gracefully.
func (a *App) Run(ctx context.Context) error {
	a.ctx, a.cancel = context.WithCancel(ctx)
	defer a.cancel()

	if info, err := a.b.GetWebhookInfo(a.ctx); err == nil && info.URL != "" {
		_, _ = a.b.DeleteWebhook(a.ctx, &bot.DeleteWebhookParams{})
		a.outbox.Push(Out{HTML: "⚠️ A webhook was configured for this bot and has been removed. If you did not set it, the bot token may have leaked: rotate it (see the runbook)."})
		a.log.Warn("webhook was set; deleted")
		a.webhookGone = true
	}
	if me, err := a.b.GetMe(a.ctx); err == nil {
		a.setBotUsername(me.Username)
	}
	a.registerCommands()
	a.jobs.SetPaused(a.st.Setting("paused", "false") == "true")

	if rec := a.jobs.Recover(); len(rec) > 0 {
		var sb strings.Builder
		sb.WriteString("♻️ <b>Bot restarted.</b> These jobs were interrupted and were <b>not</b> re-run:\n")
		for _, r := range rec {
			extra := ""
			if r.Killed {
				extra = " (leftover process stopped)"
			}
			fmt.Fprintf(&sb, "• %s in S%d%s\n", r.Job.Label(), r.Job.SessionID, extra)
		}
		sb.WriteString("Send the prompt again if you still need it.")
		a.outbox.Push(Out{HTML: sb.String()})
	}

	a.go_(func() { a.outbox.Run(a.ctx) })
	a.go_(func() { a.agents.Run(a.ctx, 60*time.Second, a.jobs.SlotsFull) })
	a.go_(a.authLoop)
	a.go_(a.versionLoop)
	a.go_(a.maintenanceLoop)
	a.log.Info("bot started", "version", Version)

	if a.st.Setting(keyBotEnabled, "true") != "false" {
		// No getMe gate here: at login the network may not be up yet, and the
		// library retries getUpdates with backoff.
		_, _ = a.StartPolling(false)
	} else {
		a.poll.set("stopped", "")
		a.log.Info("telegram polling is switched off (start it from the clawdyX app or with: clawdyx ctl bot.start)")
	}
	<-a.ctx.Done()
	a.StopPolling(false)

	a.log.Info("shutting down")
	sctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	a.jobs.Shutdown(sctx)
	fctx, fcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer fcancel()
	a.outbox.Flush(fctx)
	a.wg.Wait()
	return nil
}

// Stop requests a graceful shutdown.
func (a *App) Stop() {
	if a.cancel != nil {
		a.cancel()
	}
}

func (a *App) registerCommands() {
	cmds := []models.BotCommand{
		{Command: "help", Description: "Command reference"},
		{Command: "status", Description: "Bot, Claude CLI and session overview"},
		{Command: "new", Description: "Start a new session: /new [name] [--ws name]"},
		{Command: "sessions", Description: "List sessions"},
		{Command: "switch", Description: "Switch session: /switch S2"},
		{Command: "model", Description: "Show or set the model"},
		{Command: "effort", Description: "Show or set the effort level"},
		{Command: "mode", Description: "Permission tier: observe | assisted | automation"},
		{Command: "stop", Description: "Stop the current job (or j-ID, S-ID, all)"},
		{Command: "jobs", Description: "Recent jobs"},
		{Command: "logs", Description: "Activity log of a job: /logs j-17"},
		{Command: "processes", Description: "Claude jobs, sessions and system load"},
		{Command: "usage", Description: "Uptime, jobs, tokens, rate limits"},
		{Command: "cwd", Description: "Workspaces for new sessions"},
		{Command: "aliases", Description: "Custom model aliases"},
		{Command: "doctor", Description: "Read-only diagnostics"},
		{Command: "open", Description: "Open a website in the Mac's browser: /open youtube.com"},
		{Command: "screenshot", Description: "Photo of the Mac's screen (needs screen_control)"},
		{Command: "pause", Description: "Stop accepting new prompts"},
		{Command: "start", Description: "Menu; resume after /pause"},
	}
	_, err := a.b.SetMyCommands(a.ctx, &bot.SetMyCommandsParams{Commands: cmds, Scope: &models.BotCommandScopeChat{ChatID: a.cfg.Telegram.OwnerUserID}})
	if err != nil {
		a.log.Warn("setMyCommands failed", "err", observability.Redact(err.Error()))
	}
}

// handle is the single entry point for every update.
func (a *App) handle(ctx context.Context, _ *bot.Bot, u *models.Update) {
	if !Authorize(u, a.cfg.Telegram.OwnerUserID) {
		// Silent: no reply, no lookup, no log line with sender or content.
		a.counters.Unauthorized.Add(1)
		return
	}
	first, err := a.st.MarkUpdate(u.ID)
	if err != nil {
		a.log.Error("dedupe failed", "err", err)
		return
	}
	_ = a.st.SetSetting(keyLastUpdate, strconv.FormatInt(u.ID, 10))
	if !first {
		return
	}
	a.counters.UpdatesHandled.Add(1)
	a.errMu.Lock()
	a.lastUpdate = time.Now()
	a.errMu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			a.log.Error("handler panic", "panic", fmt.Sprint(r))
			a.reply(0, "⚠️ Internal error while handling that update. It was logged.")
		}
	}()

	switch {
	case u.CallbackQuery != nil:
		a.onCallback(ctx, u.CallbackQuery)
	case u.StoppedMessageGeneration != nil:
		id := int64(u.StoppedMessageGeneration.DraftID)
		if id > 0 {
			if _, err := a.jobs.Cancel(id); err == nil {
				a.st.Audit("job", "stop_button", fmt.Sprintf("j-%d", id), "ok", id)
			}
		}
	case u.Message != nil:
		a.onMessage(ctx, u)
	}
}

func (a *App) onMessage(ctx context.Context, u *models.Update) {
	m := u.Message
	text := strings.TrimSpace(m.Text)
	if text == "" {
		if m.Document != nil || len(m.Photo) > 0 || m.Voice != nil || m.Video != nil || m.Audio != nil || m.Sticker != nil {
			a.reply(m.ID, "Attachments are not enabled. Put the file inside the workspace and mention its path instead.")
		}
		return
	}
	if strings.HasPrefix(text, "/") {
		a.command(ctx, m, text)
		return
	}
	a.prompt(m, u.ID, text)
}

func (a *App) prompt(m *models.Message, updateID int64, text string) {
	if len(text) > MaxPromptBytes {
		a.reply(m.ID, "That message is too long (over 32 KB). Put it in a file inside the workspace and reference the path.")
		return
	}
	s, created, err := a.sess.EnsureActive()
	if err != nil {
		a.reply(m.ID, "⚠️ Could not open a session: "+observability.Redact(err.Error()))
		return
	}
	if created {
		a.reply(0, fmt.Sprintf("Started session <b>%s</b> in workspace <code>%s</code> (%s mode).", s.Label(), Esc(s.WorkspaceName), s.Mode))
	}
	_, _, err = a.jobs.Submit(jobs.Request{Session: s, Prompt: text, UpdateID: updateID, MessageID: m.ID})
	switch {
	case err == nil:
	case errors.Is(err, store.ErrDuplicate):
	case errors.Is(err, jobs.ErrQueueFull):
		a.reply(m.ID, fmt.Sprintf("Queue for %s is full (%d). Use /stop to clear it or wait.", s.Label(), a.cfg.Claude.MaxQueuePerSession))
	case errors.Is(err, jobs.ErrPaused):
		a.reply(m.ID, "The bot is paused; prompts are not accepted. Send /start to resume.")
	case errors.Is(err, jobs.ErrShutdown):
		a.reply(m.ID, "The bot is shutting down; try again in a moment.")
	default:
		a.reply(m.ID, "⚠️ Could not queue the prompt: "+observability.Redact(err.Error()))
	}
}

// reply sends an HTML message directly (replyTo 0 = no reply).
func (a *App) reply(replyTo int, html string) int {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	id, err := a.send.Send(ctx, Out{HTML: html, ReplyTo: replyTo})
	if err != nil {
		a.outbox.Push(Out{HTML: html, ReplyTo: replyTo})
	}
	return id
}

// replyLong sends possibly long HTML lines, chunked on line boundaries.
func (a *App) replyLong(replyTo int, html string) {
	var cur strings.Builder
	for _, line := range strings.Split(html, "\n") {
		if cur.Len()+len(line)+1 > 3900 && cur.Len() > 0 {
			a.reply(replyTo, cur.String())
			cur.Reset()
			replyTo = 0
		}
		if cur.Len() > 0 {
			cur.WriteString("\n")
		}
		cur.WriteString(line)
	}
	if cur.Len() > 0 {
		a.reply(replyTo, cur.String())
	}
}

// ---- background loops ----

func (a *App) refreshAuth() {
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	info, err := a.cli.AuthStatus(ctx)
	a.authMu.Lock()
	defer a.authMu.Unlock()
	a.authAt = time.Now()
	if err != nil {
		a.authErr = err
		return
	}
	a.auth, a.authErr = &info, nil
	if !info.LoggedIn {
		a.setErr("claude: not logged in")
	}
}

func (a *App) authLoop() {
	a.refreshAuth()
	t := time.NewTicker(15 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-t.C:
			a.refreshAuth()
		}
	}
}

func (a *App) authSnapshot() (*agent.AuthInfo, time.Time, error) {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	return a.auth, a.authAt, a.authErr
}

// CLIVersion returns the last detected CLI version.
func (a *App) CLIVersion() string {
	a.verMu.Lock()
	defer a.verMu.Unlock()
	return a.cliVersion
}

func (a *App) checkVersion() {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	v, err := a.cli.Version(ctx)
	if err != nil {
		a.setErr("claude --version failed: " + observability.Redact(err.Error()))
		return
	}
	a.verMu.Lock()
	a.cliVersion = v
	a.verMu.Unlock()
	prev := a.st.Setting(keyCLIVersion, "")
	if prev == v {
		return
	}
	_ = a.st.SetSetting(keyCLIVersion, v)
	_ = a.st.ClearCapabilities()
	if prev == "" {
		return
	}
	a.log.Info("claude CLI version changed", "from", prev, "to", v)
	model := a.sess.DefaultModel()
	pctx, pcancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer pcancel()
	res, perr := a.cli.ProbeModel(pctx, model, a.cfg.Claude.SettingSources)
	status := "✅ smoke probe passed"
	switch {
	case perr != nil:
		status = "❌ smoke probe could not run: " + Esc(observability.Redact(perr.Error())) + ". Check /doctor; if authentication fails, print mode may now require an API key (see the runbook)."
	case !res.OK:
		status = "❌ default model " + Esc(model) + " rejected: " + Esc(res.Reason)
	default:
		_ = a.st.SetCapability("model:"+model, res.Canonical, v)
		status += " (" + Esc(model) + " → " + Esc(res.Canonical) + ")"
	}
	a.outbox.Push(Out{HTML: fmt.Sprintf("ℹ️ Claude Code changed from %s to %s. Capability cache cleared; %s", Esc(prev), Esc(v), status)})
}

func (a *App) versionLoop() {
	a.checkVersion()
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-t.C:
			a.checkVersion()
		}
	}
}

func (a *App) maintenanceLoop() {
	r := a.cfg.Retention
	prune := func() {
		if err := a.st.Prune(store.Retention{JobsDays: r.JobsDays, JobEventsDays: r.JobEventsDays, ProcessedUpdatesDays: r.ProcessedUpdatesDays, AuditDays: r.AuditDays}); err != nil {
			a.log.Warn("prune failed", "err", err)
		}
		a.confirms.Sweep()
	}
	prune()
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-t.C:
			prune()
		}
	}
}
