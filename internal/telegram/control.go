package telegram

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/agent"
	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/control"
	"github.com/Publicthrone547/clawdyX/internal/observability"
	"github.com/Publicthrone547/clawdyX/internal/processes"
	"github.com/Publicthrone547/clawdyX/internal/sessions"
	"github.com/Publicthrone547/clawdyX/internal/store"
)

func decode[T any](raw json.RawMessage) (T, error) {
	var v T
	if len(raw) == 0 || string(raw) == "null" {
		return v, nil
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, control.InvalidParams("invalid params: %v", err)
	}
	return v, nil
}

func typed[T any](f func(ctx context.Context, p T) (any, error)) control.Handler {
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		p, err := decode[T](raw)
		if err != nil {
			return nil, err
		}
		return f(ctx, p)
	}
}

type none struct{}

// RegisterControl exposes the worker over the local control channel. All
// methods are typed; none runs shell input. Events are published through
// srv.Publish.
func (a *App) RegisterControl(srv *control.Server) {
	a.SetEventSink(srv.Publish)
	h := func(name string, timeout time.Duration, f control.Handler) { srv.Handle(name, timeout, f) }

	h("status.get", 5*time.Second, typed(func(context.Context, none) (any, error) { return a.Status(), nil }))
	h("bot.start", 30*time.Second, typed(func(context.Context, none) (any, error) {
		st, err := a.StartPolling(true)
		if err != nil {
			return nil, control.Errorf("%s", err.Error())
		}
		return st, nil
	}))
	h("bot.stop", 60*time.Second, typed(func(_ context.Context, p control.BotStopParams) (any, error) {
		st := a.StopPolling(true)
		res := control.BotStopResult{Bot: st, Cancelled: []string{}, Running: []string{}}
		for _, x := range a.jobs.ActiveJobs() {
			if p.CancelJobs {
				if prev, err := a.jobs.Cancel(x.JobID); err == nil {
					res.Cancelled = append(res.Cancelled, fmt.Sprintf("j-%d (%s)", x.JobID, prev))
				}
			} else {
				res.Running = append(res.Running, fmt.Sprintf("j-%d (%s)", x.JobID, x.State))
			}
		}
		a.st.Audit("bot", "stop", "", fmt.Sprintf("cancelled=%d", len(res.Cancelled)), 0)
		return res, nil
	}))
	h("bot.pause", 5*time.Second, typed(func(context.Context, none) (any, error) {
		a.jobs.SetPaused(true)
		a.emitBot()
		return a.BotState(), nil
	}))
	h("bot.resume", 5*time.Second, typed(func(context.Context, none) (any, error) {
		a.jobs.SetPaused(false)
		a.emitBot()
		return a.BotState(), nil
	}))

	h("sessions.list", 5*time.Second, typed(func(_ context.Context, p control.SessionsListParams) (any, error) {
		return a.listSessions(p)
	}))
	h("sessions.create", 10*time.Second, typed(func(_ context.Context, p control.SessionCreateParams) (any, error) {
		var ws *config.Workspace
		if p.Workspace != "" {
			w, ok := a.cfg.Workspace(p.Workspace)
			if !ok {
				return nil, control.InvalidParams("unknown workspace %q", p.Workspace)
			}
			ws = w
		}
		s, err := a.sess.Create(p.Name, ws)
		if err != nil {
			return nil, control.Errorf("%s", err.Error())
		}
		a.sessionsChanged("create")
		return control.SessionResult{Session: a.sessionInfo(s), Message: s.Label() + " created and active."}, nil
	}))
	h("sessions.switch", 5*time.Second, typed(func(_ context.Context, p control.SessionRefParams) (any, error) {
		s, err := a.sess.Switch(p.Ref)
		if err != nil {
			return nil, sessionErr(err)
		}
		a.sessionsChanged("switch")
		return control.SessionResult{Session: a.sessionInfo(s), Message: "Switched to " + s.Label() + "."}, nil
	}))
	h("sessions.archive", 5*time.Second, typed(func(_ context.Context, p control.SessionRefParams) (any, error) {
		s, err := a.sess.Resolve(p.Ref)
		if err != nil {
			return nil, sessionErr(err)
		}
		if a.sessionBusy(s.ID) {
			return nil, control.Errorf("%s has active jobs; stop them first", s.Label())
		}
		if err := a.sess.Archive(s); err != nil {
			return nil, control.Errorf("could not archive: %v", err)
		}
		a.sessionsChanged("archive")
		return control.SessionResult{Message: s.Label() + " archived. Its Claude Code history is kept."}, nil
	}))
	h("sessions.restore", 5*time.Second, typed(func(_ context.Context, p control.SessionRefParams) (any, error) {
		s, err := a.sess.Restore(p.Ref)
		if err != nil {
			return nil, sessionErr(err)
		}
		a.sessionsChanged("restore")
		return control.SessionResult{Session: a.sessionInfo(s), Message: s.Label() + " restored."}, nil
	}))

	h("sessions.newForget", 30*time.Second, typed(func(_ context.Context, p control.SessionNewForgetParams) (any, error) {
		old, _ := a.sess.Active()
		if p.DeleteHistory && old != nil {
			if h := sessions.FindHistory(a.projectsDir(), old); !h.Deletable {
				return nil, control.Errorf("the history of %s cannot be deleted: %s", old.Label(), h.Reason)
			}
		}
		x, old, err := a.sess.CreateAndForget(p.Name, a.busy())
		if err != nil {
			return nil, control.Errorf("%s", err.Error())
		}
		a.sessionsChanged("newf")
		res := control.SessionResult{Session: a.sessionInfo(x), Message: x.Label() + " created and active."}
		if old != nil {
			res.Previous = a.sessionInfo(old)
			res.Message += " " + old.Label() + " forgotten from clawdyX; its Claude Code history is kept."
			if p.DeleteHistory {
				removed, derr := sessions.DeleteHistory(a.projectsDir(), old)
				res.Deleted = homeRels(removed)
				if derr != nil {
					res.Message = x.Label() + " created and active. " + old.Label() + " forgotten, but its history was NOT deleted: " + derr.Error()
				} else {
					_ = a.st.MarkHistoryDeleted(old.ID)
					a.st.Audit("session", "delete_history", old.Label(), fmt.Sprintf("%d paths", len(removed)), 0)
					res.Message = fmt.Sprintf("%s created and active. %s forgotten and its Claude Code history deleted (%d item(s)).", x.Label(), old.Label(), len(removed))
				}
			}
		}
		return res, nil
	}))
	h("sessions.fresh", 10*time.Second, typed(func(context.Context, none) (any, error) {
		x, from, err := a.sess.Fresh(a.busy())
		if err != nil {
			return nil, control.Errorf("%s", err.Error())
		}
		a.sessionsChanged("fresh")
		res := control.SessionResult{Session: a.sessionInfo(x), Message: "Fresh context: " + x.Label() + "."}
		if from != nil {
			res.Previous = a.sessionInfo(from)
			res.Message += " " + from.Label() + " is kept; nothing was deleted."
		}
		return res, nil
	}))
	h("sessions.forget", 30*time.Second, typed(func(_ context.Context, p control.SessionForgetParams) (any, error) {
		x, err := a.sess.Resolve(p.Ref)
		if err != nil {
			return nil, sessionErr(err)
		}
		if p.DeleteHistory {
			if h := sessions.FindHistory(a.projectsDir(), x); !h.Deletable {
				return nil, control.Errorf("the history of %s cannot be deleted: %s", x.Label(), h.Reason)
			}
		}
		msg, removed, err := a.forgetSession(x, p.DeleteHistory)
		if err != nil {
			return nil, control.Errorf("%s", err.Error())
		}
		return control.SessionResult{Message: msg, Deleted: homeRels(removed)}, nil
	}))
	h("sessions.rename", 5*time.Second, typed(func(_ context.Context, p control.SessionRenameParams) (any, error) {
		x, err := a.sess.Resolve(p.Ref)
		if err != nil {
			return nil, sessionErr(err)
		}
		if err := a.sess.Rename(x, p.Name); err != nil {
			return nil, control.Errorf("%s", err.Error())
		}
		a.sessionsChanged("rename")
		x, _ = a.st.GetSession(x.ID)
		return control.SessionResult{Session: a.sessionInfo(x), Message: "Renamed. Only the label changed."}, nil
	}))
	h("sessions.history", 10*time.Second, typed(func(_ context.Context, p control.SessionRefParams) (any, error) {
		x, err := a.sess.Resolve(p.Ref)
		if err != nil {
			return nil, sessionErr(err)
		}
		return a.historyInfo(x), nil
	}))
	h("jobs.list", 5*time.Second, typed(func(_ context.Context, p control.JobsListParams) (any, error) {
		n := p.Limit
		if n <= 0 || n > 200 {
			n = 30
		}
		list, err := a.st.ListJobs(n)
		if err != nil {
			return nil, control.Errorf("could not read jobs")
		}
		active := map[int64]bool{}
		for _, x := range a.jobs.ActiveJobs() {
			active[x.JobID] = true
		}
		out := make([]control.JobInfo, 0, len(list))
		for i := range list {
			out = append(out, jobInfo(&list[i], active[list[i].ID]))
		}
		return out, nil
	}))
	h("jobs.cancel", 30*time.Second, typed(func(_ context.Context, p control.JobRefParams) (any, error) {
		prev, err := a.jobs.Cancel(p.ID)
		if err != nil {
			return nil, control.Errorf("j-%d is not queued or running", p.ID)
		}
		a.st.Audit("job", "cancel_gui", fmt.Sprintf("j-%d", p.ID), "ok", p.ID)
		return control.CancelResult{Previous: prev}, nil
	}))
	h("jobs.events", 5*time.Second, typed(func(_ context.Context, p control.JobEventsParams) (any, error) {
		n := p.Limit
		if n <= 0 || n > 500 {
			n = 100
		}
		evs, err := a.st.JobEvents(p.ID, n)
		if err != nil {
			return nil, control.Errorf("could not read job events")
		}
		out := make([]control.JobEvent, 0, len(evs))
		for _, e := range evs {
			out = append(out, control.JobEvent{TS: e.TS.Unix(), Type: e.Type, Payload: observability.Redact(e.Payload)})
		}
		return out, nil
	}))
	h("processes.list", 15*time.Second, typed(func(ctx context.Context, _ none) (any, error) { return a.processesInfo(ctx), nil }))

	h("claude.status", 5*time.Second, typed(func(context.Context, none) (any, error) { return a.claudeStatus(), nil }))
	h("claude.models", 5*time.Second, typed(func(context.Context, none) (any, error) { return a.modelsInfo(), nil }))
	h("model.set", 150*time.Second, typed(func(ctx context.Context, p control.SetModelParams) (any, error) { return a.setModel(ctx, p) }))
	h("effort.set", 5*time.Second, typed(func(_ context.Context, p control.SetEffortParams) (any, error) { return a.setEffort(p) }))
	h("mode.set", 5*time.Second, typed(func(_ context.Context, p control.SetModeParams) (any, error) { return a.setMode(p) }))

	h("log.tail", 5*time.Second, typed(func(_ context.Context, p control.LogTailParams) (any, error) {
		return tailLog(filepath.Join(a.cfg.LogDir, "bot.log"), p)
	}))
	h("diagnostics.run", 150*time.Second, typed(func(ctx context.Context, _ none) (any, error) { return a.diagnostics(ctx), nil }))
	a.registerScreenControl(srv)
	a.registerTelegramControl(srv)
	a.registerSettingsControl(srv)
}

func homeRels(ps []string) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, homeRel(p, 200))
	}
	return out
}

func sessionErr(err error) error {
	if errors.Is(err, sessions.ErrNoSession) {
		return control.Errorf("no such session")
	}
	return control.Errorf("%s", err.Error())
}

func (a *App) sessionsChanged(what string) {
	a.emit("sessions.changed", map[string]string{"action": what})
}

func (a *App) sessionBusy(id int64) bool {
	for _, x := range a.jobs.ActiveJobs() {
		if x.SessionID == id {
			return true
		}
	}
	return false
}

// Status builds the one-screen overview.
func (a *App) Status() control.Status {
	a.errMu.Lock()
	lastUpd, lastErr, lastErrAt := a.lastUpdate, a.lastErr, a.lastErrAt
	a.errMu.Unlock()
	st := control.Status{
		Version: Version, PID: os.Getpid(), StartedAt: a.started.Unix(), Bot: a.BotState(),
		Telegram: control.TelegramStatus{
			BotUsername: a.BotUsername(), LastUpdate: unixTime(lastUpd), PollErrors: a.counters.PollErrors.Load(),
			SendErrors: a.counters.SendErrors.Load(), Conflicts409: a.counters.Conflicts409.Load(), LastConflict: a.counters.LastConflict.Load(),
			Unauthorized: a.counters.Unauthorized.Load(), UpdatesHandled: a.counters.UpdatesHandled.Load(), Outbox: a.outbox.Len(),
			AllowedUsers: a.allowedCount(), TokenStore: a.cfg.TokenSource, TokenHint: tokenHint(a.cfg.Token), WebhookRemoved: a.webhookGone,
		},
		Claude:      a.claudeStatus(),
		LastError:   lastErr,
		LastErrorAt: unixTime(lastErrAt),
		Jobs:        control.JobCounts{MaxConcurrent: a.cfg.Claude.MaxConcurrentJobs},
		Screen:      control.ScreenStatus(a.screenStatus()),
	}
	for _, x := range a.jobs.ActiveJobs() {
		if x.State == store.JobQueued {
			st.Jobs.Queued++
		} else {
			st.Jobs.Running++
		}
	}
	if s, err := a.sess.Active(); err == nil && s != nil {
		st.Active = a.sessionInfo(s)
	}
	return st
}

func (a *App) allowedCount() int { return len(a.Allowlist()) }

// tokenHint is a masked fingerprint: the last four characters only.
func tokenHint(tok string) string {
	if len(tok) < 8 {
		return ""
	}
	return "…" + tok[len(tok)-4:]
}

func (a *App) sessionInfo(s *store.Session) *control.SessionInfo {
	if s == nil {
		return nil
	}
	mo, ef := a.sess.Effective(s)
	active, _ := a.sess.Active()
	n := 0
	for _, x := range a.jobs.ActiveJobs() {
		if x.SessionID == s.ID {
			n++
		}
	}
	return &control.SessionInfo{ID: s.ID, Label: s.Label(), Name: s.Name, Workspace: s.WorkspaceName, Mode: s.Mode,
		Model: mo, Effort: ef, ModelSet: s.Model != "", EffortSet: s.Effort != "", Status: s.Status, Origin: s.Origin,
		ClaudeShort: shortID(s.ClaudeSessionID), Started: s.Started, Turns: s.Turns, CreatedAt: unixTime(s.CreatedAt), LastUsedAt: unixTime(s.LastUsedAt),
		ForgottenAt: unixTime(s.ForgottenAt), DeletedAt: unixTime(s.DeletedAt),
		Active: active != nil && active.ID == s.ID, ActiveJobs: n}
}

func (a *App) listSessions(p control.SessionsListParams) ([]control.SessionInfo, error) {
	var list []store.Session
	var err error
	if p.Forgotten {
		list, err = a.st.ListForgotten(30 * 24 * time.Hour)
	} else {
		list, err = a.st.ListSessions(p.All, 500)
	}
	if err != nil {
		return nil, control.Errorf("could not read sessions")
	}
	out := make([]control.SessionInfo, 0, len(list))
	for i := range list {
		out = append(out, *a.sessionInfo(&list[i]))
	}
	return out, nil
}

func jobInfo(j *store.Job, active bool) control.JobInfo {
	return control.JobInfo{ID: j.ID, Label: j.Label(), SessionID: j.SessionID, State: j.State, Kind: j.Kind, PID: j.PID,
		Model: j.Model, ResolvedModel: j.ResolvedModel, Effort: j.Effort, Mode: j.PermMode,
		CreatedAt: unixTime(j.CreatedAt), StartedAt: unixTime(j.StartedAt), FinishedAt: unixTime(j.FinishedAt), NumTurns: j.NumTurns,
		InTokens: j.InTokens, OutTokens: j.OutTokens, ErrorSummary: j.ErrorSummary, ResultPreview: j.ResultPreview, Active: active}
}

func (a *App) processesInfo(ctx context.Context) control.ProcessesInfo {
	var out control.ProcessesInfo
	act := a.jobs.ActiveJobs()
	pids := []int{os.Getpid()}
	for _, x := range act {
		if x.PID > 0 {
			pids = append(pids, x.PID)
		}
	}
	var (
		wg    sync.WaitGroup
		cpu   map[int]float64
		stats map[int]processes.Stat
	)
	wg.Add(1)
	go func() { defer wg.Done(); cpu = processes.CPUPercent(ctx, pids[1:], time.Second) }()
	stats = processes.Sample(ctx, pids)
	out.WorkerRSSKB = stats[os.Getpid()].RSSKB
	own := map[string]string{}
	if list, err := a.st.ListSessions(true, 10000); err == nil {
		for _, s := range list {
			own[s.ClaudeSessionID] = s.Label()
		}
	}
	ownPID := map[int]string{}
	for _, x := range act {
		row := control.ProcessRow{JobID: x.JobID, SessionID: x.SessionID, Session: fmt.Sprintf("S%d", x.SessionID), State: x.State, PID: x.PID, Started: unixTime(x.Started), CPU: -1}
		if s, err := a.st.GetSession(x.SessionID); err == nil {
			mo, ef := a.sess.Effective(s)
			row.Model, row.Mode, row.Workspace = modelEffort(mo, ef), s.Mode, s.WorkspaceName
		}
		if x.PID > 0 {
			ownPID[x.PID] = fmt.Sprintf("j-%d", x.JobID)
			row.RSSKB = stats[x.PID].RSSKB
		}
		out.Owned = append(out.Owned, row)
	}
	entries, at, err := a.agents.Get()
	out.AgentsAt = unixTime(at)
	if err != nil && at.IsZero() {
		out.AgentsError = observability.Truncate(observability.Redact(err.Error()), 160)
	}
	for _, e := range entries {
		r := control.AgentRow{Kind: e.Kind, Name: e.Name, Status: e.Status, PID: e.PID, Dir: homeRel(e.CWD, 60)}
		if r.Name == "" {
			r.Name = e.ID
		}
		if r.Status == "" {
			r.Status = e.State
		}
		if e.StartedAt > 0 {
			r.Started = e.StartedAt / 1000
		}
		if l, ok := own[e.SessionID]; ok {
			r.OwnedBy = l
		} else if j, ok := ownPID[e.PID]; ok {
			r.OwnedBy = j
		}
		out.Agents = append(out.Agents, r)
	}
	out.ClaudeCount = processes.CountClaude(ctx)
	if sys := processes.SystemSummary(ctx); sys.OK {
		out.System = &control.SystemInfo{Load1: sys.Load1, Load5: sys.Load5, Load15: sys.Load15, HasLoad: sys.HasLoad, MemTotal: sys.MemTotal, MemFree: sys.MemFree}
	}
	wg.Wait()
	for i := range out.Owned {
		if v, ok := cpu[out.Owned[i].PID]; ok {
			out.Owned[i].CPU = v
		}
	}
	if out.Owned == nil {
		out.Owned = []control.ProcessRow{}
	}
	if out.Agents == nil {
		out.Agents = []control.AgentRow{}
	}
	return out
}

func (a *App) claudeStatus() control.ClaudeStatus {
	cs := control.ClaudeStatus{Binary: a.cfg.Claude.Binary, Version: a.CLIVersion(), DefaultModel: a.sess.DefaultModel(),
		DefaultEffort: a.sess.DefaultEffort(), AutomationOK: a.cfg.Modes.AutomationEnabled, ScreenControl: a.cfg.Desktop.ScreenControl}
	auth, at, err := a.authSnapshot()
	cs.CheckedAt = unixTime(at)
	if auth != nil {
		li := auth.LoggedIn
		cs.LoggedIn, cs.AuthMethod, cs.Provider, cs.Subscription = &li, auth.AuthMethod, auth.APIProvider, auth.SubscriptionType
	}
	if err != nil {
		cs.AuthError = observability.Truncate(observability.Redact(err.Error()), 200)
	}
	return cs
}

func (a *App) modelsInfo() control.ModelsInfo {
	mi := control.ModelsInfo{Builtin: BuiltinModelAliases, EffortLevels: config.EffortLevels,
		DefaultModel: a.sess.DefaultModel(), DefaultEffort: a.sess.DefaultEffort(), Aliases: []control.AliasInfo{}, Verified: []string{}}
	if al, err := a.st.ListAliases(); err == nil {
		for _, x := range al {
			mi.Aliases = append(mi.Aliases, control.AliasInfo{Name: x.Name, Model: x.Model, Canonical: x.Canonical})
		}
	}
	if v := a.CLIVersion(); v != "" {
		for _, n := range append(append([]string{}, BuiltinModelAliases...), aliasTargets(mi.Aliases)...) {
			if _, ok := a.st.Capability("model:"+n, v, 7*24*time.Hour); ok {
				mi.Verified = append(mi.Verified, n)
			}
		}
	}
	return mi
}

func aliasTargets(al []control.AliasInfo) []string {
	var out []string
	for _, a := range al {
		out = append(out, a.Model)
	}
	return out
}

func (a *App) targetSession(ref string) (*store.Session, error) {
	if ref != "" {
		s, err := a.sess.Resolve(ref)
		if err != nil {
			return nil, sessionErr(err)
		}
		return s, nil
	}
	s, _, err := a.sess.EnsureActive()
	if err != nil {
		return nil, control.Errorf("no session available")
	}
	return s, nil
}

func (a *App) setModel(ctx context.Context, p control.SetModelParams) (any, error) {
	if strings.EqualFold(p.Model, "reset") {
		if p.Default {
			return nil, control.InvalidParams("reset clears a session override; it has no meaning for the default")
		}
		s, err := a.targetSession(p.Session)
		if err != nil {
			return nil, err
		}
		_ = a.st.SetSessionField(s.ID, "model", "")
		a.sessionsChanged("model")
		return control.SettingResult{Message: fmt.Sprintf("Model override for %s cleared; it uses the default %s.", s.Label(), a.sess.DefaultModel())}, nil
	}
	model, via, err := a.resolveModelName(p.Model)
	if err != nil {
		return nil, control.InvalidParams("%s", err.Error())
	}
	var target *store.Session
	if !p.Default {
		if target, err = a.targetSession(p.Session); err != nil {
			return nil, err
		}
	}
	canon, _, err := a.validateModel(ctx, model)
	if err != nil {
		if errors.Is(err, agent.ErrProbeInfra) {
			return nil, control.Errorf("not changed: the validation call could not run (%s)", observability.Truncate(observability.Redact(err.Error()), 200))
		}
		return nil, control.Errorf("not changed: %s was rejected: %s", model, observability.Truncate(err.Error(), 200))
	}
	scope := "the default for new sessions"
	if target != nil {
		err, scope = a.st.SetSessionField(target.ID, "model", model), target.Label()
	} else {
		err = a.sess.SetDefaultModel(model)
	}
	if err != nil {
		return nil, control.Errorf("not changed: could not save the setting")
	}
	a.st.Audit("model", "set", scope, model, 0)
	a.sessionsChanged("model")
	msg := fmt.Sprintf("Model for %s set to %s → %s (primary model). Applies to the next message.", scope, model, canon)
	if via != "" {
		msg += " (" + via + ")"
	}
	return control.SettingResult{Message: msg, Canonical: canon}, nil
}

func (a *App) setEffort(p control.SetEffortParams) (any, error) {
	lvl := strings.ToLower(strings.TrimSpace(p.Effort))
	if !config.IsEffort(lvl) {
		return nil, control.InvalidParams("effort must be one of %v", config.EffortLevels)
	}
	if p.Default {
		_ = a.sess.SetDefaultEffort(lvl)
		a.sessionsChanged("effort")
		return control.SettingResult{Message: "Default effort set to " + lvl + "."}, nil
	}
	s, err := a.targetSession(p.Session)
	if err != nil {
		return nil, err
	}
	_ = a.st.SetSessionField(s.ID, "effort", lvl)
	a.st.Audit("effort", "set", s.Label(), lvl, 0)
	a.sessionsChanged("effort")
	msg := fmt.Sprintf("Effort for %s set to %s. Applies to the next message.", s.Label(), lvl)
	model, _ := a.sess.Effective(s)
	if canon, ok := a.expectedModel(model); ok && lvl == "xhigh" && lacksXHigh[canon] {
		msg += fmt.Sprintf(" %s does not support xhigh; Claude Code runs it as high.", canon)
	}
	return control.SettingResult{Message: msg}, nil
}

// setMode changes a tier. The local desktop user confirmed in the app's own
// dialog; automation still requires modes.automation_enabled in config.json.
func (a *App) setMode(p control.SetModeParams) (any, error) {
	mode := strings.ToLower(p.Mode)
	switch mode {
	case config.ModeObserve, config.ModeAssisted:
	case config.ModeAutomation:
		if !a.cfg.Modes.AutomationEnabled {
			return nil, control.Errorf("the automation tier is not enabled in config.json (modes.automation_enabled)")
		}
	default:
		return nil, control.InvalidParams("mode must be observe, assisted or automation")
	}
	s, err := a.targetSession(p.Session)
	if err != nil {
		return nil, err
	}
	_ = a.st.SetSessionField(s.ID, "mode", mode)
	a.st.Audit("mode", "set_gui", s.Label(), mode, 0)
	a.sessionsChanged("mode")
	return control.SettingResult{Message: fmt.Sprintf("%s tier set to %s: %s.", s.Label(), mode, modeHelp[mode])}, nil
}

var levelRank = map[string]int{"DEBUG": 0, "INFO": 1, "WARN": 2, "ERROR": 3}

// tailLog returns the last lines of the JSON log, redacted again.
func tailLog(path string, p control.LogTailParams) ([]control.LogLine, error) {
	n := p.Lines
	if n <= 0 || n > 2000 {
		n = 200
	}
	minRank := levelRank[strings.ToUpper(p.Level)]
	f, err := os.Open(path)
	if err != nil {
		return []control.LogLine{}, nil
	}
	defer f.Close()
	st, _ := f.Stat()
	const window = 2 << 20
	if st != nil && st.Size() > window {
		_, _ = f.Seek(st.Size()-window, io.SeekStart)
	}
	var lines []control.LogLine
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		var m map[string]any
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		l := control.LogLine{Attrs: map[string]string{}}
		for k, v := range m {
			s := observability.Redact(fmt.Sprint(v))
			switch k {
			case "time":
				l.Time = s
			case "level":
				l.Level = s
			case "msg":
				l.Msg = s
			default:
				l.Attrs[k] = observability.Truncate(s, 300)
			}
		}
		if levelRank[strings.ToUpper(l.Level)] < minRank {
			continue
		}
		lines = append(lines, l)
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	if lines == nil {
		lines = []control.LogLine{}
	}
	return lines, nil
}

// diagnostics runs the read-only health checks behind the app's Diagnostics page.
func (a *App) diagnostics(ctx context.Context) []control.DiagnosticCheck {
	var (
		wg            sync.WaitGroup
		ver           string
		verErr, auErr error
		auth          agent.AuthInfo
		meErr         error
		botName       string
		webhookSet    bool
		trusted       bool
	)
	wg.Add(4)
	go func() { defer wg.Done(); ver, verErr = a.cli.Version(ctx) }()
	go func() { defer wg.Done(); auth, auErr = a.cli.AuthStatus(ctx) }()
	go func() {
		defer wg.Done()
		me, err := a.b.GetMe(ctx)
		meErr = err
		if err == nil {
			botName = me.Username
			if info, err := a.b.GetWebhookInfo(ctx); err == nil {
				webhookSet = info.URL != ""
			}
		}
	}()
	go func() {
		defer wg.Done()
		if a.cfg.Desktop.ScreenControl {
			trusted = a.desktop.Trusted(ctx)
		}
	}()
	wg.Wait()
	var out []control.DiagnosticCheck
	add := func(c control.DiagnosticCheck) { out = append(out, c) }
	bs := a.BotState()
	add(control.DiagnosticCheck{ID: "worker", Label: "Background worker", OK: true, Detail: fmt.Sprintf("clawdyx %s, pid %d, up %s", Version, os.Getpid(), dur(time.Since(a.started)))})
	add(control.DiagnosticCheck{ID: "polling", Label: "Telegram polling", OK: bs.State == "polling" || bs.State == "paused", Warn: bs.State == "paused",
		Detail: bs.State, Fix: map[bool]string{true: "", false: "Start the bot from Overview."}[bs.State == "polling" || bs.State == "paused"]})
	if meErr == nil {
		add(control.DiagnosticCheck{ID: "telegram", Label: "Telegram bot token", OK: true, Detail: "connected as @" + botName})
	} else {
		add(control.DiagnosticCheck{ID: "telegram", Label: "Telegram bot token", Detail: observability.Truncate(observability.Redact(meErr.Error()), 160),
			Fix: "Check the network. If Telegram says Unauthorized, the token was revoked: replace it on the Telegram page."})
	}
	add(a.tokenStoreCheck())
	add(control.DiagnosticCheck{ID: "webhook", Label: "No webhook configured", OK: !webhookSet && meErr == nil,
		Detail: map[bool]string{true: "a webhook is set: someone else may have your token", false: "none (long polling)"}[webhookSet],
		Fix:    map[bool]string{true: "Rotate the token in @BotFather, then restart the worker.", false: ""}[webhookSet]})
	if n := a.counters.Conflicts409.Load(); n > 0 {
		add(control.DiagnosticCheck{ID: "conflict", Label: "Single poller", Detail: fmt.Sprintf("%d duplicate-poller conflicts", n), Fix: "Another instance or a leaked token polls this bot. Stop other copies or rotate the token."})
	}
	add(control.DiagnosticCheck{ID: "allowlist", Label: "Allowed Telegram users", OK: a.allowedCount() > 0, Detail: fmt.Sprintf("%d configured", a.allowedCount())})
	if verErr == nil {
		add(control.DiagnosticCheck{ID: "claude", Label: "Claude Code CLI", OK: true, Detail: "version " + ver + " at " + a.cfg.Claude.Binary})
	} else {
		add(control.DiagnosticCheck{ID: "claude", Label: "Claude Code CLI", Detail: observability.Redact(verErr.Error()), Fix: "Install Claude Code (brew install --cask claude-code) or fix claude.binary in config.json."})
	}
	switch {
	case auErr != nil:
		add(control.DiagnosticCheck{ID: "auth", Label: "Claude login", Detail: observability.Truncate(observability.Redact(auErr.Error()), 160)})
	case auth.LoggedIn:
		add(control.DiagnosticCheck{ID: "auth", Label: "Claude login", OK: true, Detail: auth.AuthMethod + " · " + auth.SubscriptionType})
	default:
		add(control.DiagnosticCheck{ID: "auth", Label: "Claude login", Detail: "not logged in", Fix: "Open Terminal, run: claude — and log in. clawdyX never logs in for you."})
	}
	for _, c := range a.platformChecks(ctx) {
		add(c)
	}
	probe := filepath.Join(a.cfg.StateDir, ".write-test")
	werr := os.WriteFile(probe, []byte("x"), 0o600)
	_ = os.Remove(probe)
	add(control.DiagnosticCheck{ID: "state", Label: "State folder writable", OK: werr == nil, Detail: homeRel(a.cfg.StateDir, 80)})
	dbErr := a.st.QuickCheck()
	add(control.DiagnosticCheck{ID: "db", Label: "State database", OK: dbErr == nil, Detail: map[bool]string{true: "integrity ok", false: "integrity check failed"}[dbErr == nil],
		Fix: map[bool]string{true: "", false: "Stop the worker and restore a backup (see the runbook)."}[dbErr == nil]})
	if a.cfg.Desktop.ScreenControl {
		c := control.DiagnosticCheck{ID: "screen", Label: "Screen Recording permission", FixURL: "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture"}
		switch acc, at := a.broker.Check(ctx); acc {
		case "granted":
			c.OK, c.Detail = true, "granted (last capture "+ago(at)+")"
		case "denied":
			c.Detail, c.Fix = "not granted (last attempt "+ago(at)+")", "Grant Screen Recording to the clawdyX worker binary, then restart the worker. macOS ties the grant to the binary, so grant it again after each rebuild unless the binary is signed with a stable identity."
		default:
			c.Warn, c.Detail = true, "unknown until the first capture (checking would need a capture)"
		}
		if !a.ScreenOn() {
			c.Detail += " · screen features switched off"
		}
		add(c)
		add(control.DiagnosticCheck{ID: "accessibility", Label: "Accessibility permission (mouse and keyboard tools)", OK: trusted,
			Detail: map[bool]string{true: "granted", false: "not granted"}[trusted], FixURL: "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility",
			Fix: map[bool]string{true: "", false: "Grant Accessibility to the clawdyX worker, then restart it."}[trusted]})
	} else {
		add(control.DiagnosticCheck{ID: "screen", Label: "Screen features", OK: true, Detail: "off in config.json (desktop.screen_control)"})
	}
	add(a.cameraCheck(ctx))
	if free, err := freeBytes(a.cfg.LogDir); err == nil {
		add(control.DiagnosticCheck{ID: "disk", Label: "Disk space for logs and state", OK: free > 500<<20, Warn: free < 2<<30, Detail: gib(free) + " free"})
	}
	return out
}

// tokenStoreCheck says where the bot token is kept.
func (a *App) tokenStoreCheck() control.DiagnosticCheck {
	c := control.DiagnosticCheck{ID: "token-store", Label: "Token storage", OK: true}
	switch {
	case a.cfg.TokenSource == "keychain":
		c.Detail = "in " + a.words.TokenStore
	case a.cfg.TokenSource == "env":
		c.Warn, c.Detail = true, "from the TELEGRAM_BOT_TOKEN variable (meant for development)"
	case a.cfg.NoKeyring:
		c.Detail = a.words.NoKeyringNote
	default:
		c.Warn, c.Detail = true, "in the private token file"
		c.Fix = "Move it into " + a.words.TokenStore + ": clawdyx setup -migrate-token, then restart the worker."
	}
	return c
}
