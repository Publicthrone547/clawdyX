package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot/models"

	"github.com/Publicthrone547/clawdyX/internal/agent"
	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/control"
	"github.com/Publicthrone547/clawdyX/internal/jobs"
	"github.com/Publicthrone547/clawdyX/internal/observability"
	"github.com/Publicthrone547/clawdyX/internal/processes"
	"github.com/Publicthrone547/clawdyX/internal/sessions"
	"github.com/Publicthrone547/clawdyX/internal/store"
)

// BuiltinModelAliases are the primary-model aliases documented for Claude
// Code (model-config docs, checked 2026-10-16). They are validated by a probe
// before use; this list is not treated as permanently valid.
var BuiltinModelAliases = []string{"default", "best", "fable", "opus", "sonnet", "haiku", "opusplan", "opus[1m]", "sonnet[1m]", "fable[1m]"}

var (
	fullModelRe = regexp.MustCompile(`^claude-[a-z0-9.-]{3,60}(\[1m\])?$`)
	aliasNameRe = regexp.MustCompile(`^[a-z0-9_-]{1,24}$`)
	jobRefRe    = regexp.MustCompile(`^(?i:j-?)?(\d{1,12})$`)
)

// Models documented without xhigh support (they fall back to high).
var lacksXHigh = map[string]bool{"claude-opus-4-6": true, "claude-sonnet-4-6": true}

const userHelpText = `<b>clawdyX</b> — send any message to talk to Claude in the active session.

/status · /sessions · /switch S2 · /rename S2 name
/new [name] · /newf [name] · /clear
/model [name|reset] · /effort [level] — for the active session
/aliases · /stop · /cancel j-ID — your own jobs
<i>Sessions are shared with the bot's owner. Tiers, defaults, screens, the camera and the machine are owner-only.</i>`

const helpTextTmpl = `<b>Claude control center</b> — send any message to talk to Claude in the active session.

<b>Sessions</b>
/status — overview · /sessions [all] · /switch S2 · /rename S2 name
/new [name] [--ws name] — new session, keep the current one
/newf [name] [--delete] — new session, forget the current one (--delete also erases its Claude Code history)
/clear — fresh context: new session with the same settings, old one kept
/sessions archive S2 · /sessions restore S2 · /forget S2 [--delete]
/cwd [workspace] — default workspace for new sessions
<b>Claude settings</b>
/model [name|alias|reset] [--default] · /effort [low|medium|high|xhigh|max|auto] [--default]
/mode [observe|assisted|automation] — permission tier of the active session
/aliases · /alias set fast haiku · /alias rm fast
<b>Jobs</b>
/stop [j-ID|S-ID|all] · /cancel j-ID · /jobs [n] · /logs j-ID [lines]
<b>Machine</b>
/processes · /usage · /doctor · /open youtube.com — open a website on {{THIS}}
/screens — list displays · /screen [N|all] — photo of display N · /look [N|all] [question] — ask Claude about display N or all of them · /screen off
/cameras — list cameras · /camera [N] — photo · /camera video SECONDS [N] — clip of up to 60 s, no sound (/screen off switches the camera off too)
<b>Bot</b>
/pause · /start (resume) · /confirm TOKEN · /deny TOKEN`

func parseJobRef(s string) (int64, bool) {
	m := jobRefRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, false
	}
	id, err := strconv.ParseInt(m[1], 10, 64)
	return id, err == nil
}

func isSessionRef(s string) bool {
	return len(s) > 1 && (s[0] == 'S' || s[0] == 's') && strings.Trim(s[1:], "0123456789") == ""
}

// takeFlag removes "--name value" from args.
func takeFlag(args []string, name string) ([]string, string, bool) {
	for i := 0; i < len(args); i++ {
		if args[i] == name {
			if i+1 < len(args) {
				v := args[i+1]
				return append(append([]string{}, args[:i]...), args[i+2:]...), v, true
			}
			return append(append([]string{}, args[:i]...), args[i+1:]...), "", true
		}
	}
	return args, "", false
}

// userCommands are available to allowed users with the "user" role. The
// owner can use every command. Users share the bot's sessions but cannot
// change tiers, defaults, screen features, the machine, or delete anything.
var userCommands = map[string]bool{
	"start": true, "help": true, "status": true, "sessions": true, "new": true, "newf": true, "clear": true,
	"switch": true, "resume": true, "rename": true, "model": true, "effort": true, "aliases": true,
	"stop": true, "cancel": true, "confirm": true, "deny": true,
}

// ownerOnlyArgs reports flags or sub-commands of user commands that are owner-only.
func ownerOnlyArgs(cmd string, args []string) bool {
	for _, a := range args {
		if a == "--default" || a == "--delete" || a == "--session" || a == "--ws" {
			return true
		}
	}
	switch cmd {
	case "sessions":
		return len(args) > 0 && args[0] != "all"
	case "stop":
		return len(args) > 0 && strings.EqualFold(args[0], "all")
	}
	return false
}

// mayControlJob: the owner may stop any job; a user only jobs from their chat.
func (a *App) mayControlJob(uid int64, role string, jobID int64) bool {
	if role == config.RoleOwner {
		return true
	}
	j, err := a.st.GetJob(jobID)
	return err == nil && j.TgChatID == uid
}

func (a *App) command(ctx context.Context, m *models.Message, text, role string) {
	fields := strings.Fields(text)
	cmd := strings.ToLower(strings.TrimPrefix(fields[0], "/"))
	if i := strings.IndexByte(cmd, '@'); i >= 0 {
		cmd = cmd[:i]
	}
	args := fields[1:]
	a.st.Audit("command", cmd, "", role, 0)
	if role != config.RoleOwner && (!userCommands[cmd] || ownerOnlyArgs(cmd, args)) {
		a.replyTo(m, "Only the owner can do that. /help lists what you can use.")
		return
	}
	isOwner := role == config.RoleOwner
	defer a.emit("telegram.command", map[string]string{"command": cmd})
	switch cmd {
	case "start":
		a.cmdStart(m, isOwner)
	case "help":
		if isOwner {
			a.replyTo(m, strings.ReplaceAll(helpTextTmpl, "{{THIS}}", a.words.ThisComputer))
		} else {
			a.replyTo(m, userHelpText)
		}
	case "status":
		a.cmdStatus(m)
	case "usage":
		a.cmdUsage(m)
	case "stop":
		a.cmdStop(m, args, isOwner)
	case "cancel":
		a.cmdCancel(m, args, isOwner)
	case "jobs":
		a.cmdJobs(m, args)
	case "logs":
		a.cmdLogs(m, args)
	case "processes", "ps":
		a.cmdProcesses(m)
	case "model":
		a.cmdModel(m, args)
	case "effort":
		a.cmdEffort(m, args)
	case "aliases":
		a.cmdAliases(m)
	case "alias":
		a.cmdAlias(m, args)
	case "sessions":
		a.cmdSessions(m, args)
	case "new":
		a.cmdNew(m, args)
	case "newf":
		a.cmdNewForget(m, args)
	case "clear":
		a.cmdClear(m)
	case "forget":
		a.cmdForget(m, args)
	case "rename":
		a.cmdRename(m, args)
	case "switch", "resume":
		a.cmdSwitch(m, args)
	case "cwd", "workspace":
		a.cmdCwd(m, args)
	case "mode":
		a.cmdMode(m, args)
	case "open":
		a.cmdOpen(m, args)
	case "screenshot", "screen":
		a.cmdScreen(m, args)
	case "screens":
		a.cmdScreens(m)
	case "cameras":
		a.cmdCameras(m)
	case "camera":
		a.cmdCamera(m, args)
	case "look":
		a.cmdLook(m, args)
	case "doctor":
		a.cmdDoctor(m)
	case "pause":
		a.jobs.SetPaused(true)
		a.replyTo(m, "⏸ Paused. Running jobs continue; new prompts are refused. /start resumes.")
	case "confirm":
		if len(args) != 1 {
			a.replyTo(m, "Usage: /confirm TOKEN")
			return
		}
		a.confirm(tgt(m), 0, args[0])
	case "deny":
		if len(args) != 1 {
			a.replyTo(m, "Usage: /deny TOKEN")
			return
		}
		if a.confirms.Deny(args[0], m.From.ID) {
			a.replyTo(m, "OK, not doing it.")
		} else {
			a.replyTo(m, "That confirmation is unknown, expired, or already used.")
		}
	case "shutdown":
		a.cmdShutdown(m)
	default:
		a.replyTo(m, "Unknown command. /help lists the commands.")
	}
}

// ---- /start /status /usage ----

func (a *App) activeLine() string {
	s, err := a.sess.Active()
	if err != nil || s == nil {
		ws := a.sess.DefaultWorkspace()
		return fmt.Sprintf("No active session. The next message starts one in <code>%s</code>.", Esc(ws.Name))
	}
	model, effort := a.sess.Effective(s)
	name := ""
	if s.Name != "" {
		name = " “" + Esc(s.Name) + "”"
	}
	return fmt.Sprintf("Session <b>%s</b>%s · ws <code>%s</code> · %s · %s · %d turns · <code>%s</code>",
		s.Label(), name, Esc(s.WorkspaceName), s.Mode, Esc(modelEffort(model, effort)), s.Turns, shortID(s.ClaudeSessionID))
}

func (a *App) cmdStart(m *models.Message, isOwner bool) {
	resumed := ""
	if isOwner && a.jobs.Paused() {
		a.jobs.SetPaused(false)
		resumed = "▶️ Resumed: prompts are accepted again.\n\n"
	}
	a.registerCommands()
	a.replyTo(m, fmt.Sprintf("%s<b>clawdyx %s</b> · up %s\n%s\n\nSend a message to talk to Claude. /help for commands.", resumed, Esc(Version), dur(time.Since(a.started)), a.activeLine()))
}

func (a *App) cmdStatus(m *models.Message) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "<b>clawdyx %s</b> · up %s · pid %d\n", Esc(Version), dur(time.Since(a.started)), os.Getpid())
	a.errMu.Lock()
	lastUpd, lastErr, lastErrAt := a.lastUpdate, a.lastErr, a.lastErrAt
	a.errMu.Unlock()
	fmt.Fprintf(&sb, "Telegram: last update %s · poll errors %d · send errors %d · outbox %d\n", ago(lastUpd), a.counters.PollErrors.Load(), a.counters.SendErrors.Load(), a.outbox.Len())
	if n := a.counters.Conflicts409.Load(); n > 0 {
		fmt.Fprintf(&sb, "⚠️ Duplicate poller detected %d time(s), last %s. Another instance is running or the token leaked.\n", n, ago(time.Unix(a.counters.LastConflict.Load(), 0)))
	}
	if n := a.counters.Unauthorized.Load(); n > 0 {
		fmt.Fprintf(&sb, "Unauthorized updates dropped: %d\n", n)
	}
	auth, at, aerr := a.authSnapshot()
	v := a.CLIVersion()
	if v == "" {
		v = "unknown"
	}
	switch {
	case auth != nil:
		state := "logged in"
		if !auth.LoggedIn {
			state = "❌ NOT logged in"
		}
		fmt.Fprintf(&sb, "Claude CLI: %s · %s via %s (%s) · checked %s\n", Esc(v), state, Esc(auth.AuthMethod), Esc(auth.SubscriptionType), ago(at))
	case aerr != nil:
		fmt.Fprintf(&sb, "Claude CLI: %s · auth check failed: %s\n", Esc(v), Esc(observability.Truncate(observability.Redact(aerr.Error()), 120)))
	default:
		fmt.Fprintf(&sb, "Claude CLI: %s · auth check pending\n", Esc(v))
	}
	sb.WriteString(a.activeLine() + "\n")
	running, queued := 0, 0
	for _, x := range a.jobs.ActiveJobs() {
		if x.State == store.JobQueued {
			queued++
		} else {
			running++
		}
	}
	paused := "no"
	if a.jobs.Paused() {
		paused = "yes"
	}
	fmt.Fprintf(&sb, "Jobs: %d running, %d queued (max %d concurrent) · paused: %s\n", running, queued, a.cfg.Claude.MaxConcurrentJobs, paused)
	if lastErr != "" {
		fmt.Fprintf(&sb, "Last error (%s): %s", ago(lastErrAt), Esc(observability.Truncate(lastErr, 200)))
	} else {
		sb.WriteString("Last error: none")
	}
	a.replyTo(m, sb.String())
}

func (a *App) cmdUsage(m *models.Message) {
	var sb strings.Builder
	y, mo, d := time.Now().Date()
	day := time.Date(y, mo, d, 0, 0, 0, 0, time.Local)
	today, err1 := a.st.Stats(day)
	week, err2 := a.st.Stats(time.Now().Add(-7 * 24 * time.Hour))
	if err1 != nil || err2 != nil {
		a.replyTo(m, "⚠️ Could not read job statistics from the database.")
		return
	}
	active, total := a.st.CountSessions()
	fmt.Fprintf(&sb, "<b>Usage</b> · up %s · sessions %d active / %d total\n", dur(time.Since(a.started)), active, total)
	fmt.Fprintf(&sb, "Jobs today: %d (%s)\n", today.Jobs, stateSummary(today.ByState))
	fmt.Fprintf(&sb, "Jobs 7 days: %d (%s)\n", week.Jobs, stateSummary(week.ByState))
	fmt.Fprintf(&sb, "Tokens today: in %s · out %s · cache read %s · cache create %s\n", tokens(today.InTokens), tokens(today.OutTokens), tokens(today.CacheReadTokens), tokens(today.CacheCreate))
	fmt.Fprintf(&sb, "Tokens 7 days: in %s · out %s · cache read %s · cache create %s\n", tokens(week.InTokens), tokens(week.OutTokens), tokens(week.CacheReadTokens), tokens(week.CacheCreate))
	sb.WriteString("<i>Token counts are summed from Claude's per-message usage in the stream.</i>\n")
	if cost, n := a.sessionCosts(); n > 0 {
		fmt.Fprintf(&sb, "Cost estimate: $%.2f across %d bot session(s) — client-side list-price estimate from the CLI; included in a subscription, not a bill.\n", cost, n)
	} else {
		sb.WriteString("Cost estimate: unavailable (no completed jobs yet)\n")
	}
	if rl, at := a.jobs.RateLimit(); rl != nil {
		var ws []string
		keys := make([]string, 0, len(rl.Windows))
		for k := range rl.Windows {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			w := rl.Windows[k]
			reset := "?"
			if w.ResetsAt > 0 {
				reset = time.Unix(w.ResetsAt, 0).Format("Jan 2 15:04")
			}
			ws = append(ws, fmt.Sprintf("%s %.0f%% (resets %s)", strings.ReplaceAll(k, "_", "-"), w.Utilization*100, reset))
		}
		fmt.Fprintf(&sb, "Rate limits (observed %s; undocumented CLI field): status %s · %s\n", ago(at), Esc(rl.Status), Esc(strings.Join(ws, " · ")))
	} else {
		sb.WriteString("Rate limits: unavailable (not observed yet)\n")
	}
	pids := []int{os.Getpid()}
	for _, x := range a.jobs.ActiveJobs() {
		if x.PID > 0 {
			pids = append(pids, x.PID)
		}
	}
	stats := processes.Sample(context.Background(), pids)
	var child int64
	for _, p := range pids[1:] {
		child += stats[p].RSSKB
	}
	sys := processes.SystemSummary(context.Background())
	fmt.Fprintf(&sb, "Memory: bot %s · Claude children %s", mb(stats[os.Getpid()].RSSKB), mb(child))
	if sys.OK {
		fmt.Fprintf(&sb, " · free or reclaimable %s of %s", gib(sys.MemFree), gib(sys.MemTotal))
		if sys.HasLoad {
			fmt.Fprintf(&sb, " · load %.2f", sys.Load1)
		}
	}
	a.replyTo(m, sb.String())
}

func (a *App) sessionCosts() (float64, int) {
	list, _ := a.st.ListSessions(true, 10000)
	var sum float64
	n := 0
	for _, s := range list {
		if v := a.st.Setting(fmt.Sprintf("session_cost_%d", s.ID), ""); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				sum += f
				n++
			}
		}
	}
	return sum, n
}

func stateSummary(m map[string]int) string {
	if len(m) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []string
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%d %s", m[k], k))
	}
	return strings.Join(out, ", ")
}

// ---- /stop /cancel /jobs /logs ----

func (a *App) cmdStop(m *models.Message, args []string, isOwner bool) {
	act := a.jobs.ActiveJobs()
	if !isOwner {
		var own []jobs.Active
		for _, x := range act {
			if a.mayControlJob(m.From.ID, config.RoleUser, x.JobID) {
				own = append(own, x)
			}
		}
		act = own
	}
	if len(args) == 0 {
		s, _ := a.sess.Active()
		if s == nil {
			a.replyTo(m, "No active session; nothing to stop.")
			return
		}
		var stopped []string
		for _, x := range act {
			if x.SessionID == s.ID {
				if prev, err := a.jobs.Cancel(x.JobID); err == nil {
					stopped = append(stopped, fmt.Sprintf("j-%d (%s)", x.JobID, prev))
				}
			}
		}
		if len(stopped) == 0 {
			a.replyTo(m, fmt.Sprintf("Nothing is running in %s. The bot itself keeps running.", s.Label()))
			return
		}
		a.replyTo(m, fmt.Sprintf("⛔ Stopping in %s: %s.", s.Label(), strings.Join(stopped, ", ")))
		return
	}
	arg := args[0]
	switch {
	case strings.EqualFold(arg, "all"):
		if len(act) == 0 {
			a.replyTo(m, "No jobs are running or queued.")
			return
		}
		ids := make([]string, len(act))
		for i, x := range act {
			ids[i] = strconv.FormatInt(x.JobID, 10)
		}
		if len(act) == 1 {
			a.stopJobs(tgt(m), ids)
			return
		}
		tok := a.confirms.New(m.From.ID, "stop_all", strings.Join(ids, ","))
		labels := make([]string, len(act))
		for i, x := range act {
			labels[i] = fmt.Sprintf("j-%d (S%d, %s)", x.JobID, x.SessionID, x.State)
		}
		a.askConfirm(tgt(m), fmt.Sprintf("Stop %d jobs: %s?", len(act), strings.Join(labels, ", ")), tok, "Stop them", "Keep running")
	case isSessionRef(arg):
		s, err := a.sess.Resolve(arg)
		if err != nil {
			a.replyTo(m, "No such session: "+Esc(arg))
			return
		}
		var ids []string
		for _, x := range act {
			if x.SessionID == s.ID {
				ids = append(ids, strconv.FormatInt(x.JobID, 10))
			}
		}
		if len(ids) == 0 {
			a.replyTo(m, fmt.Sprintf("Nothing is running or queued in %s.", s.Label()))
			return
		}
		a.stopJobs(tgt(m), ids)
	default:
		if id, ok := parseJobRef(arg); ok {
			a.cancelOne(tgt(m), id)
			return
		}
		a.replyTo(m, "Usage: /stop [j-ID | S-ID | all]. Without an argument it stops the active session's job.")
	}
}

func (a *App) stopJobs(t target, ids []string) {
	var done []string
	for _, s := range ids {
		id, _ := strconv.ParseInt(s, 10, 64)
		if prev, err := a.jobs.Cancel(id); err == nil {
			done = append(done, fmt.Sprintf("j-%d (%s)", id, prev))
		}
	}
	if len(done) == 0 {
		a.say(t, "Those jobs already finished.")
		return
	}
	a.say(t, "⛔ Stopping: "+strings.Join(done, ", ")+".")
}

func (a *App) cancelOne(t target, id int64) {
	prev, err := a.jobs.Cancel(id)
	if err == nil {
		a.say(t, fmt.Sprintf("⛔ Cancelling j-%d (was %s).", id, prev))
		return
	}
	j, gerr := a.st.GetJob(id)
	if gerr != nil {
		a.say(t, fmt.Sprintf("No job j-%d.", id))
		return
	}
	a.say(t, fmt.Sprintf("j-%d already %s %s.", id, j.State, ago(j.FinishedAt)))
}

func (a *App) cmdCancel(m *models.Message, args []string, isOwner bool) {
	if len(args) != 1 {
		a.replyTo(m, "Usage: /cancel j-ID")
		return
	}
	id, ok := parseJobRef(args[0])
	if !ok {
		a.replyTo(m, "Usage: /cancel j-ID")
		return
	}
	if !isOwner && !a.mayControlJob(m.From.ID, config.RoleUser, id) {
		a.replyTo(m, "You can only cancel your own jobs.")
		return
	}
	a.cancelOne(tgt(m), id)
}

func (a *App) cmdJobs(m *models.Message, args []string) {
	n := 10
	if len(args) == 1 {
		if v, err := strconv.Atoi(args[0]); err == nil && v > 0 && v <= 50 {
			n = v
		}
	}
	list, err := a.st.ListJobs(n)
	if err != nil {
		a.replyTo(m, "⚠️ Could not read jobs.")
		return
	}
	if len(list) == 0 {
		a.replyTo(m, "No jobs yet.")
		return
	}
	var sb strings.Builder
	sb.WriteString("<b>Recent jobs</b>\n")
	for _, j := range list {
		icon := stateIcon[j.State]
		if icon == "" {
			icon = "▶️"
		}
		d := ""
		if !j.StartedAt.IsZero() && !j.FinishedAt.IsZero() {
			d = " · " + dur(j.FinishedAt.Sub(j.StartedAt))
		}
		fmt.Fprintf(&sb, "%s %s %s · S%d · %s%s · in %s/out %s · %s\n", icon, j.Label(), j.State, j.SessionID, Esc(modelEffort(j.Model, j.Effort)), d, tokens(j.InTokens), tokens(j.OutTokens), ago(j.CreatedAt))
	}
	a.replyTo(m, sb.String())
}

func (a *App) cmdLogs(m *models.Message, args []string) {
	if len(args) < 1 {
		a.replyTo(m, "Usage: /logs j-ID [lines]")
		return
	}
	id, ok := parseJobRef(args[0])
	if !ok {
		a.replyTo(m, "Usage: /logs j-ID [lines]")
		return
	}
	lines := 40
	if len(args) > 1 {
		if v, err := strconv.Atoi(args[1]); err == nil && v > 0 {
			lines = min(v, 200)
		}
	}
	j, err := a.st.GetJob(id)
	if err != nil {
		a.replyTo(m, fmt.Sprintf("No job j-%d.", id))
		return
	}
	evs, _ := a.st.JobEvents(id, lines)
	var sb strings.Builder
	fmt.Fprintf(&sb, "<b>%s</b> %s · S%d · %s · pid %d\n", j.Label(), j.State, j.SessionID, Esc(modelEffort(j.Model, j.Effort)), j.PID)
	if len(evs) == 0 {
		sb.WriteString("No tool activity recorded.\n")
	}
	for _, e := range evs {
		icon := map[string]string{"tool_use": "🔧", "denial": "🚫", "api_retry": "↻"}[e.Type]
		fmt.Fprintf(&sb, "%s %s <code>%s</code>\n", e.TS.Format("15:04:05"), icon, Esc(observability.Truncate(e.Payload, 200)))
	}
	if j.ErrorSummary != "" {
		sb.WriteString("Error: " + Esc(j.ErrorSummary) + "\n")
	}
	if j.ResultPreview != "" {
		sb.WriteString("Result preview:\n<pre>" + Esc(observability.Truncate(j.ResultPreview, 1500)) + "</pre>")
	}
	a.replyLongTo(m, sb.String())
}

// ---- /processes ----

func (a *App) cmdProcesses(m *models.Message) {
	var sb strings.Builder
	act := a.jobs.ActiveJobs()
	var pids []int
	for _, x := range act {
		if x.PID > 0 {
			pids = append(pids, x.PID)
		}
	}
	stats := processes.Sample(context.Background(), pids)
	sb.WriteString("<b>Bot-owned Claude jobs</b>\n")
	ownSessions := map[string]string{}
	if list, err := a.st.ListSessions(true, 10000); err == nil {
		for _, s := range list {
			ownSessions[s.ClaudeSessionID] = s.Label()
		}
	}
	ownPIDs := map[int]bool{}
	if len(act) == 0 {
		sb.WriteString("• none\n")
	}
	for _, x := range act {
		s, _ := a.st.GetSession(x.SessionID)
		label := fmt.Sprintf("S%d", x.SessionID)
		ws, me := "", ""
		if s != nil {
			if s.Name != "" {
				label += " " + Esc(s.Name)
			}
			ws = " · ws " + Esc(s.WorkspaceName)
			mo, ef := a.sess.Effective(s)
			me = " · " + Esc(modelEffort(mo, ef)) + " · " + s.Mode
		}
		if x.PID > 0 {
			ownPIDs[x.PID] = true
			st := stats[x.PID]
			fmt.Fprintf(&sb, "• j-%d · %s · pid %d · %s %s%s%s · %s · %.0f%% CPU (avg)\n", x.JobID, label, x.PID, x.State, dur(time.Since(x.Started)), me, ws, mb(st.RSSKB), st.CPU)
		} else {
			fmt.Fprintf(&sb, "• j-%d · %s · %s%s\n", x.JobID, label, x.State, me)
		}
	}
	entries, at, err := a.agents.Get()
	age := "never refreshed"
	if !at.IsZero() {
		age = ago(at)
		if time.Since(at) > 3*time.Minute {
			age += ", stale"
		}
	}
	fmt.Fprintf(&sb, "\n<b>Claude Code sessions</b> (claude agents --json, %s)\n", age)
	if err != nil && at.IsZero() {
		sb.WriteString("• unavailable: " + Esc(observability.Truncate(observability.Redact(err.Error()), 120)) + "\n")
	} else if len(entries) == 0 {
		sb.WriteString("• none\n")
	}
	for _, e := range entries {
		mark := ""
		if l, ok := ownSessions[e.SessionID]; ok {
			mark = " · bot " + l
		} else if ownPIDs[e.PID] {
			mark = " · bot job"
		}
		status := e.Status
		if status == "" {
			status = e.State
		}
		name := e.Name
		if name == "" {
			name = e.ID
		}
		started := ""
		if e.StartedAt > 0 {
			started = " · since " + time.UnixMilli(e.StartedAt).Format("Jan 2 15:04")
		}
		fmt.Fprintf(&sb, "• %s · %s · %s · pid %d%s · %s%s\n", Esc(e.Kind), Esc(name), Esc(status), e.PID, started, Esc(homeRel(e.CWD, 60)), mark)
	}
	if n := processes.CountClaude(context.Background()); n >= 0 {
		fmt.Fprintf(&sb, "\nAll processes named claude: %d (only bot-owned jobs can be stopped from here)\n", n)
	}
	if sys := processes.SystemSummary(context.Background()); sys.OK {
		sb.WriteString("System")
		if sys.HasLoad {
			fmt.Fprintf(&sb, " · load %.2f / %.2f / %.2f", sys.Load1, sys.Load5, sys.Load15)
		}
		fmt.Fprintf(&sb, " · free or reclaimable %s of %s\n", gib(sys.MemFree), gib(sys.MemTotal))
	}
	sb.WriteString("CPU is an average, not an instantaneous reading. Stop a bot job: /cancel j-ID")
	a.replyLongTo(m, sb.String())
}

// ---- /model /effort /aliases ----

func (a *App) resolveModelName(name string) (string, string, error) {
	lower := strings.ToLower(strings.TrimSpace(name))
	if al, err := a.st.GetAlias(lower); err == nil {
		return al.Model, "custom alias " + lower + " → " + al.Model, nil
	}
	for _, b := range BuiltinModelAliases {
		if lower == b {
			return lower, "", nil
		}
	}
	if fullModelRe.MatchString(lower) {
		return lower, "", nil
	}
	return "", "", fmt.Errorf("%q is not a known alias, custom alias, or claude-* model ID. Built-in aliases: %s. /aliases for custom names", name, strings.Join(BuiltinModelAliases, ", "))
}

// validateModel returns the canonical model ID, using the per-CLI-version cache.
func (a *App) validateModel(ctx context.Context, model string) (string, bool, error) {
	v := a.CLIVersion()
	if v != "" {
		if c, ok := a.st.Capability("model:"+model, v, 7*24*time.Hour); ok {
			return c, true, nil
		}
	}
	res, err := a.cli.ProbeModel(ctx, model, a.cfg.Claude.SettingSources)
	if err != nil {
		return "", false, err
	}
	if !res.OK {
		return "", false, errors.New(res.Reason)
	}
	if v != "" {
		_ = a.st.SetCapability("model:"+model, res.Canonical, v)
	}
	return res.Canonical, false, nil
}

func userSettingsModel() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		return ""
	}
	var v struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(b, &v)
	return v.Model
}

func (a *App) cmdModel(m *models.Message, args []string) {
	args, sessRef, hasSess := takeFlag(args, "--session")
	args, _, isDefault := takeFlag(args, "--default")
	if len(args) == 0 {
		var sb strings.Builder
		s, _ := a.sess.Active()
		def := a.sess.DefaultModel()
		if s != nil {
			src := "chat default"
			mo := def
			if s.Model != "" {
				src, mo = "session override", s.Model
			}
			canon := ""
			if c, ok := a.expectedModel(mo); ok {
				canon = " → " + Esc(c)
			}
			fmt.Fprintf(&sb, "Model for %s: <b>%s</b>%s (%s)\n", s.Label(), Esc(mo), canon, src)
		}
		cli := userSettingsModel()
		if cli == "" {
			cli = "unset"
		}
		fmt.Fprintf(&sb, "Chat default: %s · Claude Code user setting: %s (used only if the bot passed no --model; it always passes one)\n", Esc(def), Esc(cli))
		fmt.Fprintf(&sb, "Built-in aliases (Claude Code docs, 2026-10-16): %s\n", Esc(strings.Join(BuiltinModelAliases, ", ")))
		if al, _ := a.st.ListAliases(); len(al) > 0 {
			var xs []string
			for _, x := range al {
				xs = append(xs, x.Name+" → "+x.Model)
			}
			fmt.Fprintf(&sb, "Custom aliases: %s\n", Esc(strings.Join(xs, ", ")))
		}
		sb.WriteString("Set: /model NAME · chat default: /model NAME --default · clear the session override: /model reset\n")
		sb.WriteString("<i>This is the primary model. The separate --advisor model is never changed by the bot.</i>")
		a.replyTo(m, sb.String())
		return
	}
	if len(args) != 1 {
		a.replyTo(m, "Usage: /model [NAME|reset] [--default] [--session S-ID]")
		return
	}
	var target *store.Session
	if hasSess {
		s, err := a.sess.Resolve(sessRef)
		if err != nil {
			a.replyTo(m, "No such session: "+Esc(sessRef))
			return
		}
		target = s
	} else if !isDefault {
		s, _, err := a.sess.EnsureActive()
		if err != nil {
			a.replyTo(m, "⚠️ No session available.")
			return
		}
		target = s
	}
	if strings.EqualFold(args[0], "reset") {
		if target == nil {
			a.replyTo(m, "reset clears a session override; it has no meaning with --default.")
			return
		}
		_ = a.st.SetSessionField(target.ID, "model", "")
		a.replyTo(m, fmt.Sprintf("Model override for %s cleared; it now uses the chat default %s.", target.Label(), Esc(a.sess.DefaultModel())))
		return
	}
	model, via, err := a.resolveModelName(args[0])
	if err != nil {
		a.replyTo(m, "❌ Not changed. "+Esc(err.Error()))
		return
	}
	scope := "chat default for new sessions"
	if target != nil {
		scope = target.Label()
	}
	ack := a.replyTo(m, fmt.Sprintf("Validating %s…", Esc(model)))
	a.go_(func() {
		ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
		defer cancel()
		canon, cached, err := a.validateModel(ctx, model)
		edit := func(html string) {
			if ack != 0 {
				_ = a.send.Edit(a.ctx, m.Chat.ID, ack, html, nil)
			} else {
				a.replyTo(m, html)
			}
		}
		if err != nil {
			prefix := "❌ Not changed. "
			if errors.Is(err, agent.ErrProbeInfra) {
				edit(prefix + "The validation call could not run: " + Esc(observability.Truncate(observability.Redact(err.Error()), 200)) + ". See /doctor.")
				return
			}
			edit(prefix + Esc(model) + " was rejected: " + Esc(observability.Truncate(err.Error(), 200)))
			return
		}
		if target != nil {
			err = a.st.SetSessionField(target.ID, "model", model)
		} else {
			err = a.sess.SetDefaultModel(model)
		}
		if err != nil {
			edit("❌ Not changed: could not save the setting.")
			return
		}
		src := "verified now"
		if cached {
			src = "verified earlier"
		}
		extra := ""
		if via != "" {
			extra = " (" + Esc(via) + ")"
		}
		a.st.Audit("model", "set", scope, model, 0)
		edit(fmt.Sprintf("✅ Model for %s set to <b>%s</b> → %s%s (primary model; %s on CLI %s). Applies to the next message.", Esc(scope), Esc(model), Esc(canon), extra, src, Esc(a.CLIVersion())))
	})
}

func (a *App) cmdEffort(m *models.Message, args []string) {
	args, _, isDefault := takeFlag(args, "--default")
	s, _ := a.sess.Active()
	if len(args) == 0 {
		var sb strings.Builder
		def := a.sess.DefaultEffort()
		if s != nil {
			src, e := "chat default", def
			if s.Effort != "" {
				src, e = "session", s.Effort
			}
			fmt.Fprintf(&sb, "Effort for %s: <b>%s</b> (%s)\n", s.Label(), Esc(e), src)
		}
		fmt.Fprintf(&sb, "Chat default: %s\n", Esc(def))
		sb.WriteString("Levels accepted by the installed CLI: low, medium, high, xhigh, max. <b>auto</b> = pass no --effort (Claude Code settings or the model's default decide).\n")
		sb.WriteString("Set: /effort LEVEL · chat default: /effort LEVEL --default")
		a.replyTo(m, sb.String())
		return
	}
	lvl := strings.ToLower(args[0])
	if len(args) != 1 || !config.IsEffort(lvl) {
		a.replyTo(m, "Usage: /effort [low|medium|high|xhigh|max|auto] [--default]")
		return
	}
	if isDefault {
		_ = a.sess.SetDefaultEffort(lvl)
		a.replyTo(m, fmt.Sprintf("✅ Chat default effort set to %s.", lvl))
		return
	}
	if s == nil {
		var err error
		if s, _, err = a.sess.EnsureActive(); err != nil {
			a.replyTo(m, "⚠️ No session available.")
			return
		}
	}
	_ = a.st.SetSessionField(s.ID, "effort", lvl)
	note := ""
	model, _ := a.sess.Effective(s)
	if canon, ok := a.expectedModel(model); ok && lvl == "xhigh" && lacksXHigh[canon] {
		note = fmt.Sprintf(" Note: %s does not support xhigh; Claude Code runs it as high.", Esc(canon))
	}
	a.st.Audit("effort", "set", s.Label(), lvl, 0)
	a.replyTo(m, fmt.Sprintf("✅ Effort for %s set to <b>%s</b>. Applies to the next message.%s", s.Label(), lvl, note))
}

func (a *App) cmdAliases(m *models.Message) {
	al, err := a.st.ListAliases()
	if err != nil {
		a.replyTo(m, "⚠️ Could not read aliases.")
		return
	}
	if len(al) == 0 {
		a.replyTo(m, "No custom aliases. Create one: /alias set fast haiku")
		return
	}
	var sb strings.Builder
	sb.WriteString("<b>Custom model aliases</b>\n")
	for _, x := range al {
		fmt.Fprintf(&sb, "• %s → %s → %s (verified %s)\n", Esc(x.Name), Esc(x.Model), Esc(x.Canonical), x.CreatedAt.Format("Jan 2"))
	}
	a.replyTo(m, sb.String())
}

func (a *App) cmdAlias(m *models.Message, args []string) {
	if len(args) == 2 && args[0] == "rm" {
		if err := a.st.DeleteAlias(strings.ToLower(args[1])); err != nil {
			a.replyTo(m, "No such alias.")
			return
		}
		a.replyTo(m, "Alias removed.")
		return
	}
	if len(args) != 3 || args[0] != "set" {
		a.replyTo(m, "Usage: /alias set NAME MODEL · /alias rm NAME")
		return
	}
	name := strings.ToLower(args[1])
	if !aliasNameRe.MatchString(name) || config.IsEffort(name) || name == "reset" {
		a.replyTo(m, "Alias names must match [a-z0-9_-]{1,24} and must not be an effort level or 'reset'.")
		return
	}
	for _, b := range BuiltinModelAliases {
		if name == b {
			a.replyTo(m, "That name is a built-in alias; pick another.")
			return
		}
	}
	model := strings.ToLower(args[2])
	if _, err := a.st.GetAlias(model); err == nil {
		a.replyTo(m, "An alias cannot point to another alias.")
		return
	}
	builtin := false
	for _, b := range BuiltinModelAliases {
		builtin = builtin || model == b
	}
	if !builtin && !fullModelRe.MatchString(model) {
		a.replyTo(m, "The target must be a built-in alias or a claude-* model ID.")
		return
	}
	ack := a.replyTo(m, fmt.Sprintf("Validating %s…", Esc(model)))
	a.go_(func() {
		ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
		defer cancel()
		canon, _, err := a.validateModel(ctx, model)
		if err != nil {
			_ = a.send.Edit(a.ctx, m.Chat.ID, ack, "❌ Alias not created: "+Esc(observability.Truncate(observability.Redact(err.Error()), 200)), nil)
			return
		}
		_ = a.st.SetAlias(store.Alias{Name: name, Model: model, Canonical: canon})
		_ = a.send.Edit(a.ctx, m.Chat.ID, ack, fmt.Sprintf("✅ Alias %s → %s → %s. Use /model %s.", Esc(name), Esc(model), Esc(canon), Esc(name)), nil)
	})
}

// ---- sessions ----

func (a *App) cmdSessions(m *models.Message, args []string) {
	if len(args) == 2 && args[0] == "restore" {
		x, err := a.sess.Restore(args[1])
		if err != nil {
			a.replyTo(m, "⚠️ "+Esc(err.Error()))
			return
		}
		a.sessionsChanged("restore")
		a.replyTo(m, fmt.Sprintf("%s restored. /switch %s makes it active.", x.Label(), x.Label()))
		return
	}
	if len(args) == 2 && args[0] == "archive" {
		s, err := a.sess.Resolve(args[1])
		if err != nil {
			a.replyTo(m, "No such session.")
			return
		}
		for _, x := range a.jobs.ActiveJobs() {
			if x.SessionID == s.ID {
				a.replyTo(m, fmt.Sprintf("%s has active jobs; stop them first (/stop %s).", s.Label(), s.Label()))
				return
			}
		}
		if err := a.sess.Archive(s); err != nil {
			a.replyTo(m, "⚠️ Could not archive.")
			return
		}
		a.replyTo(m, fmt.Sprintf("%s archived. Its Claude transcript is untouched.", s.Label()))
		return
	}
	all := len(args) == 1 && args[0] == "all"
	list, err := a.st.ListSessions(all, 20)
	if err != nil {
		a.replyTo(m, "⚠️ Could not read sessions.")
		return
	}
	if len(list) == 0 {
		a.replyTo(m, "No sessions yet. Send a message or use /new.")
		return
	}
	active, _ := a.sess.Active()
	var sb strings.Builder
	sb.WriteString("<b>Sessions</b>\n")
	for _, s := range list {
		star := ""
		if active != nil && active.ID == s.ID {
			star = "*"
		}
		name := ""
		if s.Name != "" {
			name = " " + Esc(s.Name)
		}
		status := ""
		if s.Status != "active" {
			status = " · " + s.Status
		}
		mo, ef := a.sess.Effective(&s)
		fmt.Fprintf(&sb, "%s%s%s · ws %s · %s · %s · %d turns · %s · <code>%s</code>%s\n", s.Label(), star, name, Esc(s.WorkspaceName), s.Mode, Esc(modelEffort(mo, ef)), s.Turns, ago(s.LastUsedAt), shortID(s.ClaudeSessionID), status)
	}
	sb.WriteString("* = active · /switch S-ID · /new [name] · /newf · /clear · /forget S-ID")
	a.replyTo(m, sb.String())
}

func (a *App) cmdNew(m *models.Message, args []string) {
	args, wsName, hasWS := takeFlag(args, "--ws")
	var ws *config.Workspace
	if hasWS {
		w, ok := a.cfg.Workspace(wsName)
		if !ok {
			a.replyTo(m, "Unknown workspace. /cwd lists them.")
			return
		}
		ws = w
	}
	name := ""
	if len(args) > 1 {
		a.replyTo(m, "Usage: /new [name] [--ws workspace]")
		return
	}
	if len(args) == 1 {
		name = args[0]
		if !sessions.ValidName(name) {
			a.replyTo(m, "Session names may use letters, digits, '_' and '-' (max 32).")
			return
		}
	}
	s, err := a.sess.Create(name, ws)
	if err != nil {
		a.replyTo(m, "⚠️ "+Esc(err.Error()))
		return
	}
	mo, ef := a.sess.Effective(s)
	a.replyTo(m, fmt.Sprintf("🆕 Session <b>%s</b>%s created and active · ws <code>%s</code> · %s · %s", s.Label(), func() string {
		if name == "" {
			return ""
		}
		return " “" + Esc(name) + "”"
	}(), Esc(s.WorkspaceName), s.Mode, Esc(modelEffort(mo, ef))))
}

func (a *App) cmdSwitch(m *models.Message, args []string) {
	if len(args) != 1 {
		a.replyTo(m, "Usage: /switch S-ID|name")
		return
	}
	if _, err := a.sess.Switch(args[0]); err != nil {
		if errors.Is(err, sessions.ErrNoSession) {
			a.replyTo(m, "No such session. /sessions lists them. (Attaching external Claude sessions is not enabled yet.)")
			return
		}
		a.replyTo(m, "⚠️ "+Esc(err.Error()))
		return
	}
	a.replyTo(m, "Switched. "+a.activeLine())
}

func (a *App) cmdCwd(m *models.Message, args []string) {
	if len(args) == 1 {
		if err := a.sess.SetDefaultWorkspace(args[0]); err != nil {
			a.replyTo(m, "Unknown workspace. Only configured workspaces are allowed; /cwd lists them.")
			return
		}
		a.replyTo(m, fmt.Sprintf("New sessions will use <code>%s</code>. Existing sessions keep their workspace; /new starts one here.", Esc(args[0])))
		return
	}
	def := a.sess.DefaultWorkspace()
	var sb strings.Builder
	sb.WriteString("<b>Workspaces</b> (configured roots only)\n")
	for _, w := range a.cfg.Workspaces {
		star := ""
		if w.Name == def.Name {
			star = "*"
		}
		fmt.Fprintf(&sb, "• %s%s → <code>%s</code> · default tier %s\n", Esc(w.Name), star, Esc(w.Canonical), w.DefaultMode)
	}
	if s, _ := a.sess.Active(); s != nil {
		fmt.Fprintf(&sb, "Active session %s works in <code>%s</code>.\n", s.Label(), Esc(s.WorkspacePath))
	}
	sb.WriteString("* = default for /new · change: /cwd NAME")
	a.replyTo(m, sb.String())
}

// modeHelp says what a tier allows, in the words of this OS.
func (a *App) modeHelp(mode string) string {
	switch mode {
	case config.ModeObserve:
		return "read files in the workspace and run Claude Code's built-in read-only commands; edits, other commands and web access are denied"
	case config.ModeAssisted:
		return "also edit files inside the workspace and run the workspace command allowlist; deny rules (rm, sudo, git push, network tools, secret paths) still apply"
	case config.ModeAutomation:
		return "run shell commands on " + a.words.ThisComputer + " (" + a.words.AutomationExamples + ") without asking whenever Claude Code's safety classifier approves them; " + a.words.DeniedSummary + " and secret files stay blocked"
	}
	return ""
}

func (a *App) cmdMode(m *models.Message, args []string) {
	if len(args) == 0 {
		s, _ := a.sess.Active()
		if s == nil {
			ws := a.sess.DefaultWorkspace()
			a.replyTo(m, fmt.Sprintf("No active session. New sessions in <code>%s</code> start in <b>%s</b> — %s.", Esc(ws.Name), ws.DefaultMode, a.modeHelp(ws.DefaultMode)))
			return
		}
		ws, _ := a.cfg.Workspace(s.WorkspaceName)
		allow := "none"
		if ws != nil && len(ws.AllowedCommands) > 0 {
			allow = strings.Join(ws.AllowedCommands, ", ")
		}
		a.replyTo(m, fmt.Sprintf("Tier for %s: <b>%s</b> — %s.\nAssisted command allowlist: <code>%s</code>\nChange: /mode observe | /mode assisted. Bypass mode is never used.", s.Label(), s.Mode, a.modeHelp(s.Mode), Esc(allow)))
		return
	}
	mode := strings.ToLower(args[0])
	switch mode {
	case config.ModeObserve, config.ModeAssisted:
		s, _, err := a.sess.EnsureActive()
		if err != nil {
			a.replyTo(m, "⚠️ No session available.")
			return
		}
		_ = a.st.SetSessionField(s.ID, "mode", mode)
		a.st.Audit("mode", "set", s.Label(), mode, 0)
		a.replyTo(m, fmt.Sprintf("✅ %s tier set to <b>%s</b>: %s. Applies to the next message.", s.Label(), mode, a.modeHelp(mode)))
	case config.ModeAutomation:
		if !a.cfg.Modes.AutomationEnabled {
			a.replyTo(m, "The automation tier is not enabled. Turn it on in the clawdyX app on "+a.words.ThisComputer+": Settings → Features → Automation tier available, then restart the worker.")
			return
		}
		s, _, err := a.sess.EnsureActive()
		if err != nil {
			a.replyTo(m, "⚠️ No session available.")
			return
		}
		if s.Mode == config.ModeAutomation {
			a.replyTo(m, fmt.Sprintf("%s is already in automation.", s.Label()))
			return
		}
		tok := a.confirms.New(m.From.ID, "mode_automation", strconv.FormatInt(s.ID, 10))
		extra := ""
		if a.cfg.Desktop.ScreenControl {
			extra = " It will also see your screen and use the mouse and keyboard."
		}
		a.askConfirm(tgt(m), fmt.Sprintf("Switch %s to automation? Claude will %s.%s Switch back any time with /mode observe.", s.Label(), a.modeHelp(config.ModeAutomation), extra), tok, "Enable automation", "Keep current tier")
	default:
		a.replyTo(m, "Usage: /mode [observe|assisted|automation]")
	}
}

// ---- confirmations ----

func (a *App) askConfirm(t target, question, tok, yes, no string) {
	markup := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
		{Text: yes, CallbackData: "y:" + tok}, {Text: no, CallbackData: "n:" + tok},
	}}}
	_, err := a.send.Send(a.ctx, Out{HTML: Esc(question) + fmt.Sprintf("\n<i>Expires in %ds. Text fallback: /confirm %s</i>", int(ConfirmTTL.Seconds()), tok), ChatID: t.chat, ReplyTo: t.msg, Markup: markup})
	if err != nil {
		a.log.Warn("confirmation prompt failed")
	}
}

// confirm executes a confirmed action. editID is the prompt message (0 if text).
func (a *App) confirm(t target, editID int, tok string) {
	action, args, err := a.confirms.Take(tok, "", t.uid)
	if err != nil {
		a.st.Audit("confirm", "reject", "", err.Error(), 0)
		a.say(t, "That confirmation is unknown, expired, or already used. Nothing was done.")
		return
	}
	a.st.Audit("confirm", action, args, "accepted", 0)
	if editID != 0 {
		_ = a.send.Edit(a.ctx, t.chat, editID, "Confirmed.", noMarkup())
	}
	if a.confirmLifecycle(t, action, args) {
		return
	}
	switch action {
	case "mode_automation":
		id, _ := strconv.ParseInt(args, 10, 64)
		s, err := a.st.GetSession(id)
		if err != nil || !a.cfg.Modes.AutomationEnabled {
			a.say(t, "Not changed: the session is gone or automation was disabled.")
			return
		}
		_ = a.st.SetSessionField(s.ID, "mode", config.ModeAutomation)
		a.st.Audit("mode", "set", s.Label(), config.ModeAutomation, 0)
		a.say(t, fmt.Sprintf("🤖 %s is now in <b>automation</b>. Ask for things like \"open YouTube in Firefox\". /mode observe switches back.", s.Label()))
	case "stop_all":
		a.stopJobs(t, strings.Split(args, ","))
	case "shutdown":
		a.say(t, "Shutting down gracefully. "+a.words.StartAgainHint)
		a.shutdownOnce.Do(func() { close(a.ShutdownRequested) })
		go func() {
			time.Sleep(time.Second)
			a.Stop()
		}()
	}
}

func (a *App) onCallback(ctx context.Context, cq *models.CallbackQuery, role string) {
	data := cq.Data
	msgID := 0
	var chat int64
	if cq.Message.Message != nil {
		msgID, chat = cq.Message.Message.ID, cq.Message.Message.Chat.ID
	}
	t := target{chat: chat, msg: msgID, uid: cq.From.ID}
	switch {
	case strings.HasPrefix(data, "c:") && role != config.RoleOwner && !a.mayControlJob(cq.From.ID, role, func() int64 {
		parts := strings.Split(data, ":")
		if len(parts) == 3 {
			id, _ := strconv.ParseInt(parts[1], 10, 64)
			return id
		}
		return 0
	}()):
		a.send.Answer(ctx, cq.ID, "You can only cancel your own jobs")
	case strings.HasPrefix(data, "c:"):
		parts := strings.Split(data, ":")
		if len(parts) != 3 {
			a.send.Answer(ctx, cq.ID, "Invalid button")
			return
		}
		id, err := strconv.ParseInt(parts[1], 10, 64)
		st := a.streams.get(id)
		if err != nil || st == nil || st.nonce != parts[2] {
			a.send.Answer(ctx, cq.ID, "This job is no longer active")
			return
		}
		if _, err := a.jobs.Cancel(id); err != nil {
			a.send.Answer(ctx, cq.ID, "Already finished")
			return
		}
		a.st.Audit("job", "cancel_button", fmt.Sprintf("j-%d", id), "ok", id)
		a.send.Answer(ctx, cq.ID, fmt.Sprintf("Cancelling j-%d", id))
	case strings.HasPrefix(data, "y:"):
		a.send.Answer(ctx, cq.ID, "")
		a.confirm(t, msgID, strings.TrimPrefix(data, "y:"))
	case strings.HasPrefix(data, "n:"):
		a.send.Answer(ctx, cq.ID, "")
		if a.confirms.Deny(strings.TrimPrefix(data, "n:"), cq.From.ID) {
			_ = a.send.Edit(ctx, chat, msgID, "OK, nothing was done.", noMarkup())
		}
	default:
		a.send.Answer(ctx, cq.ID, "Unknown button")
	}
}

func (a *App) cmdShutdown(m *models.Message) {
	if !a.cfg.ShutdownCommandEnabled {
		a.replyTo(m, "/shutdown is disabled (shutdown_command_enabled is false). "+a.words.StopServiceHint)
		return
	}
	tok := a.confirms.New(m.From.ID, "shutdown", "")
	a.askConfirm(tgt(m), "Shut the bot down? Running jobs will be stopped. "+a.words.StartNeedsHint, tok, "Shut down", "Cancel")
}

// ---- /doctor ----

func (a *App) cmdDoctor(m *models.Message) {
	ack := a.replyTo(m, "🩺 Running read-only diagnostics (about 30–45 s; claude auth status and claude agents are slow)…")
	a.go_(func() {
		ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
		defer cancel()
		var (
			wg                    sync.WaitGroup
			ver, doc              string
			verErr, docErr, agErr error
			auth                  agent.AuthInfo
			authErr               error
			agents                []agent.AgentEntry
			authDur, agDur        time.Duration
		)
		wg.Add(4)
		go func() { defer wg.Done(); ver, verErr = a.cli.Version(ctx) }()
		go func() { defer wg.Done(); doc, docErr = a.cli.Doctor(ctx) }()
		go func() {
			defer wg.Done()
			t := time.Now()
			auth, authErr = a.cli.AuthStatus(ctx)
			authDur = time.Since(t)
		}()
		go func() {
			defer wg.Done()
			t := time.Now()
			agents, agErr = a.cli.Agents(ctx)
			agDur = time.Since(t)
		}()
		wg.Wait()

		ok := func(b bool) string {
			if b {
				return "✅"
			}
			return "❌"
		}
		var sb strings.Builder
		sb.WriteString("<b>Doctor</b> (read-only)\n")
		fmt.Fprintf(&sb, "%s CLI binary <code>%s</code>", ok(verErr == nil), Esc(a.cfg.Claude.Binary))
		if verErr == nil {
			fmt.Fprintf(&sb, " · version %s\n", Esc(ver))
		} else {
			fmt.Fprintf(&sb, " · %s\n", Esc(observability.Redact(verErr.Error())))
		}
		if authErr == nil {
			fmt.Fprintf(&sb, "%s Auth: loggedIn=%v · method %s · provider %s · subscription %s (%s)\n", ok(auth.LoggedIn), auth.LoggedIn, Esc(auth.AuthMethod), Esc(auth.APIProvider), Esc(auth.SubscriptionType), dur(authDur))
			if !auth.LoggedIn {
				sb.WriteString("   → In " + Esc(a.words.Terminal) + " run: claude auth login (the bot never logs in or uses API keys)\n")
			}
		} else {
			fmt.Fprintf(&sb, "❌ Auth check failed: %s\n", Esc(observability.Redact(authErr.Error())))
		}
		if agErr == nil {
			fmt.Fprintf(&sb, "✅ claude agents --json: %d session(s) (%s)\n", len(agents), dur(agDur))
		} else {
			fmt.Fprintf(&sb, "❌ claude agents --json: %s\n", Esc(observability.Redact(agErr.Error())))
		}
		dbErr := a.st.QuickCheck()
		fmt.Fprintf(&sb, "%s State database integrity\n", ok(dbErr == nil))
		probe := filepath.Join(a.cfg.StateDir, ".write-test")
		werr := os.WriteFile(probe, []byte("x"), 0o600)
		_ = os.Remove(probe)
		fmt.Fprintf(&sb, "%s State directory writable\n", ok(werr == nil))
		_, meErr := a.b.GetMe(ctx)
		fmt.Fprintf(&sb, "%s Telegram getMe · last update %s · poll errors %d · 409 conflicts %d\n", ok(meErr == nil), ago(a.lastUpdateTime()), a.counters.PollErrors.Load(), a.counters.Conflicts409.Load())
		if info, err := a.b.GetWebhookInfo(ctx); err == nil {
			fmt.Fprintf(&sb, "%s Webhook: %s (long polling requires none)\n", ok(info.URL == ""), map[bool]string{true: "none", false: "SET"}[info.URL == ""])
		}
		var names []string
		for _, kv := range a.cli.Env {
			k, _, _ := strings.Cut(kv, "=")
			names = append(names, k)
		}
		fmt.Fprintf(&sb, "Child environment (names only): %s\n", Esc(strings.Join(names, ", ")))
		for _, c := range a.platformChecks(ctx) {
			fmt.Fprintf(&sb, "%s %s: %s\n", ok(c.OK), Esc(c.Label), Esc(c.Detail))
			if !c.OK && c.Fix != "" {
				sb.WriteString("   → " + Esc(c.Fix) + "\n")
			}
		}
		fmt.Fprintf(&sb, "Settings sources for jobs: %s · strict MCP: %v · bypass/auto disabled via --settings\n", Esc(a.cfg.Claude.SettingSources), a.cfg.Claude.StrictMCPConfig)
		if a.cfg.Desktop.ScreenControl {
			for _, c := range []control.DiagnosticCheck{a.screenCheck(ctx), a.inputCheck(a.desktop.Trusted(ctx))} {
				fmt.Fprintf(&sb, "%s %s: %s\n", ok(c.OK || c.Warn), Esc(c.Label), Esc(c.Detail))
				if !c.OK && c.Fix != "" {
					sb.WriteString("   → " + Esc(c.Fix) + "\n")
				}
			}
		}
		if a.cfg.Desktop.Camera {
			acc, _ := a.cams.Check(ctx)
			fmt.Fprintf(&sb, "%s Camera permission: %s · camera %s\n", ok(acc != "denied"), acc, map[bool]string{true: "on", false: "switched off"}[a.CameraOn()])
		}
		if docErr == nil {
			sb.WriteString("\n<b>claude doctor</b>\n<pre>" + Esc(observability.Truncate(observability.Redact(strings.TrimSpace(doc)), 1500)) + "</pre>")
		} else {
			sb.WriteString("\n❌ claude doctor: " + Esc(observability.Redact(docErr.Error())))
		}
		if ack != 0 {
			_ = a.send.Edit(a.ctx, m.Chat.ID, ack, "🩺 Diagnostics finished.", nil)
		}
		a.replyLongTo(m, sb.String())
	})
}

func (a *App) lastUpdateTime() time.Time {
	a.errMu.Lock()
	defer a.errMu.Unlock()
	return a.lastUpdate
}

var _ jobs.Notifier = (*App)(nil)
