// Package jobs runs prompts as Claude child processes with per-session
// serialization (the CLI does not lock sessions), a global concurrency cap,
// bounded queues, timeouts, an RSS watchdog, cancellation, and restart
// recovery that never re-runs a prompt automatically.
package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/agent"
	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/observability"
	"github.com/Publicthrone547/clawdyX/internal/platform"
	"github.com/Publicthrone547/clawdyX/internal/policy"
	"github.com/Publicthrone547/clawdyX/internal/processes"
	"github.com/Publicthrone547/clawdyX/internal/sessions"
	"github.com/Publicthrone547/clawdyX/internal/store"
)

// Errors returned by Submit.
var (
	ErrQueueFull = errors.New("queue full")
	ErrPaused    = errors.New("paused")
	ErrShutdown  = errors.New("shutting down")
	ErrNotActive = errors.New("job is not queued or running")
)

// KeyRateLimit stores the last observed rate_limit_event.
const KeyRateLimit = "last_rate_limit"

// Progress is a streaming snapshot.
type Progress struct {
	Text     string // accumulated assistant text (bounded)
	LastTool string // e.g. "Read internal/x.go"
	Denials  int
	Tools    int
	Elapsed  time.Duration
}

// Outcome is the final result of a job.
type Outcome struct {
	State          string
	Text           string
	Truncated      bool
	Denials        []string
	Warnings       []string
	ErrorMessage   string
	SessionMissing bool
	Usage          agent.Usage
	Duration       time.Duration
	ResolvedModel  string
	Effort         string
	NumTurns       int
}

// Notifier receives job lifecycle callbacks. Calls are made from job
// goroutines; implementations must be safe for concurrent use and must not block long.
type Notifier interface {
	JobQueued(j *store.Job, s *store.Session, position int)
	JobStarted(j *store.Job, s *store.Session)
	JobProgress(j *store.Job, p Progress)
	JobFinished(j *store.Job, s *store.Session, o Outcome)
}

// StartFunc launches a process (replaceable in tests).
type StartFunc func(agent.Spec) (*agent.Process, error)

// Manager owns all jobs.
type Manager struct {
	cfg      *config.Config
	st       *store.Store
	sess     *sessions.Service
	log      *slog.Logger
	notify   Notifier
	start    StartFunc
	home     string
	childEnv []string

	// Grace periods for the cancellation ladder (shortened in tests).
	IntGrace, TermGrace time.Duration

	// ScreenCommand is the executable that serves `mcp-screen` (the bot
	// binary itself). Empty disables screen control.
	ScreenCommand string
	// Words are the OS words used in prompts to Claude (platform.W).
	Words platform.Words
	// ScreenEnabled reports the runtime screen switch (nil = on).
	ScreenEnabled func() bool
	// ControlSocket is passed to the screen server so it can ask the worker.
	ControlSocket string

	sem chan struct{}

	mu         sync.Mutex
	queues     map[int64][]int64       // session -> queued job IDs (not yet picked by the worker)
	waiting    map[int64]*waitState    // picked by the worker, waiting for a global slot
	starting   map[int64]*waitState    // holds a slot, process not started yet
	ready      map[int64]chan struct{} // closed once the queued notification was delivered
	prompts    map[int64]string        // prompts live in memory only, never in the database
	images     map[int64][]agent.Image // likewise for attached screen captures
	workers    map[int64]bool
	running    map[int64]*runState
	paused     bool
	shutdown   bool
	shutdownCh chan struct{}
	wg         sync.WaitGroup

	rlMu      sync.Mutex
	rateLimit *agent.RateLimit
	rlAt      time.Time
}

type waitState struct {
	sessionID int64
	cancel    chan struct{}
	once      sync.Once
}

type runState struct {
	jobID     int64
	sessionID int64
	proc      *agent.Process
	reason    string // "cancel", "timeout", "rss", "shutdown"
	stopped   chan struct{}
	once      sync.Once
	started   time.Time
}

// New creates a manager.
func New(cfg *config.Config, st *store.Store, sess *sessions.Service, log *slog.Logger, n Notifier, start StartFunc) *Manager {
	if start == nil {
		start = agent.Start
	}
	home, _ := os.UserHomeDir()
	m := &Manager{cfg: cfg, st: st, sess: sess, log: log, notify: n, start: start, home: home,
		childEnv: agent.ChildEnv(cfg.Claude.ChildPath, cfg.Claude.BgWaitCeilingMS, cfg.Claude.ChildEnvExtra()...),
		IntGrace: 10 * time.Second, TermGrace: 5 * time.Second,
		sem:    make(chan struct{}, cfg.Claude.MaxConcurrentJobs),
		queues: map[int64][]int64{}, waiting: map[int64]*waitState{}, starting: map[int64]*waitState{}, ready: map[int64]chan struct{}{}, prompts: map[int64]string{}, images: map[int64][]agent.Image{},
		workers: map[int64]bool{}, running: map[int64]*runState{}, shutdownCh: make(chan struct{})}
	m.Words = platform.W
	m.paused = st.Setting("paused", "false") == "true"
	if raw := st.Setting(KeyRateLimit, ""); raw != "" {
		var w struct {
			At int64
			RL agent.RateLimit
		}
		if json.Unmarshal([]byte(raw), &w) == nil {
			m.rateLimit, m.rlAt = &w.RL, time.Unix(w.At, 0)
		}
	}
	return m
}

// SetChildEnv overrides the child environment (tests).
func (m *Manager) SetChildEnv(env []string) { m.childEnv = env }

// SetPaused stops or resumes accepting new jobs.
func (m *Manager) SetPaused(p bool) {
	m.mu.Lock()
	m.paused = p
	m.mu.Unlock()
	_ = m.st.SetSetting("paused", fmt.Sprint(p))
}

// Paused reports whether new jobs are refused.
func (m *Manager) Paused() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.paused
}

// Request is a prompt submission.
type Request struct {
	Session   *store.Session
	Prompt    string
	UpdateID  int64
	MessageID int
	ChatID    int64         // Telegram chat that asked (0 = the owner / the app)
	Images    []agent.Image // optional screen captures sent with the prompt
	Kind      string        // "prompt" (default) or "look"
	// Registered runs with the new job before any notification is sent.
	Registered func(*store.Job)
}

// Submit queues a prompt. It is idempotent on UpdateID (ErrDuplicate from
// the store). The queued notification is delivered before the job can start.
func (m *Manager) Submit(r Request) (*store.Job, int, error) {
	m.mu.Lock()
	if m.shutdown {
		m.mu.Unlock()
		return nil, 0, ErrShutdown
	}
	if m.paused {
		m.mu.Unlock()
		return nil, 0, ErrPaused
	}
	if len(m.queues[r.Session.ID]) >= m.cfg.Claude.MaxQueuePerSession {
		m.mu.Unlock()
		return nil, 0, ErrQueueFull
	}
	sum := sha256.Sum256([]byte(r.Prompt))
	model, effort := m.sess.Effective(r.Session)
	j, err := m.st.InsertJob(store.Job{SessionID: r.Session.ID, Kind: r.Kind, TgUpdateID: r.UpdateID, TgMessageID: r.MessageID, TgChatID: r.ChatID,
		PromptSHA256: hex.EncodeToString(sum[:]), Model: model, Effort: effort, PermMode: r.Session.Mode})
	if err != nil {
		m.mu.Unlock()
		return nil, 0, err
	}
	position := len(m.queues[r.Session.ID])
	if m.sessionBusyLocked(r.Session.ID) {
		position++
	}
	m.queues[r.Session.ID] = append(m.queues[r.Session.ID], j.ID)
	m.prompts[j.ID] = r.Prompt
	if len(r.Images) > 0 {
		m.images[j.ID] = r.Images
	}
	ready := make(chan struct{})
	m.ready[j.ID] = ready
	if !m.workers[r.Session.ID] {
		m.workers[r.Session.ID] = true
		m.wg.Add(1)
		go m.worker(r.Session.ID)
	}
	m.mu.Unlock()

	jc := *j
	if r.Registered != nil {
		r.Registered(&jc)
	}
	m.notify.JobQueued(&jc, r.Session, position)
	close(ready)
	return j, position, nil
}

func (m *Manager) takePrompt(id int64) (string, []agent.Image, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.prompts[id]
	imgs := m.images[id]
	delete(m.prompts, id)
	delete(m.images, id)
	return p, imgs, ok
}

func (m *Manager) sessionBusyLocked(sid int64) bool {
	for _, rs := range m.running {
		if rs.sessionID == sid {
			return true
		}
	}
	for _, w := range m.waiting {
		if w.sessionID == sid {
			return true
		}
	}
	return false
}

func (m *Manager) worker(sid int64) {
	defer m.wg.Done()
	for {
		m.mu.Lock()
		q := m.queues[sid]
		if len(q) == 0 || m.shutdown {
			delete(m.workers, sid)
			m.mu.Unlock()
			return
		}
		id := q[0]
		m.queues[sid] = q[1:]
		ws := &waitState{sessionID: sid, cancel: make(chan struct{})}
		m.waiting[id] = ws
		ready := m.ready[id]
		m.mu.Unlock()

		if ready != nil {
			<-ready
		}
		acquired := false
		select {
		case m.sem <- struct{}{}:
			acquired = true
		case <-ws.cancel:
		case <-m.shutdownCh:
		}
		cancelled := false
		select {
		case <-ws.cancel:
			cancelled = true
		default:
		}
		m.mu.Lock()
		delete(m.waiting, id)
		delete(m.ready, id)
		shutdown := m.shutdown
		if acquired && !cancelled && !shutdown {
			m.starting[id] = ws // stays cancellable until the process is registered
		}
		m.mu.Unlock()
		switch {
		case cancelled:
			m.finishUnstarted(id, store.JobCancelled, "cancelled before start")
		case shutdown:
			m.finishUnstarted(id, store.JobInterrupted, "the bot was shutting down")
		default:
			m.run(id)
		}
		if acquired {
			<-m.sem
		}
	}
}

func (m *Manager) finishUnstarted(id int64, state, msg string) {
	m.takePrompt(id)
	m.mu.Lock()
	delete(m.starting, id)
	m.mu.Unlock()
	j, err := m.st.GetJob(id)
	if err != nil {
		return
	}
	j.State, j.ErrorSummary = state, msg
	_ = m.st.FinishJob(*j)
	s, _ := m.st.GetSession(j.SessionID)
	m.notify.JobFinished(j, s, Outcome{State: state, ErrorMessage: msg})
}

func (m *Manager) run(id int64) {
	prompt, images, ok := m.takePrompt(id)
	j, err := m.st.GetJob(id)
	if err != nil {
		return
	}
	if !ok {
		m.finishUnstarted(id, store.JobInterrupted, "prompt no longer available")
		return
	}
	m.mu.Lock()
	sw := m.starting[id]
	m.mu.Unlock()
	if sw != nil {
		select {
		case <-sw.cancel:
			m.finishUnstarted(id, store.JobCancelled, "cancelled before start")
			return
		default:
		}
	}
	s, err := m.st.GetSession(j.SessionID)
	if err != nil {
		m.finishUnstarted(id, store.JobFailed, "session not found")
		return
	}
	ws, err := m.sess.Workspace(s)
	var dir string
	if err == nil {
		dir, err = policy.CheckWorkspaceDir(ws)
	}
	if err != nil {
		m.finishUnstarted(id, store.JobFailed, err.Error())
		return
	}
	flags, err := policy.For(s.Mode, ws, m.cfg.Claude, m.home, m.cfg.Modes.AutomationEnabled)
	if err != nil {
		m.finishUnstarted(id, store.JobFailed, err.Error())
		return
	}
	model, effort := m.sess.Effective(s)
	j.Model, j.Effort, j.PermMode = model, effort, s.Mode
	var mcpConfig, appendSystem string
	if s.Mode == config.ModeAutomation && m.cfg.Desktop.ScreenControl && m.ScreenCommand != "" && (m.ScreenEnabled == nil || m.ScreenEnabled()) {
		srv := map[string]any{"type": "stdio", "command": m.ScreenCommand, "args": []string{"mcp-screen"}}
		if m.ControlSocket != "" {
			// The screen server asks the worker before every screenshot, so
			// switching screen features off takes effect mid-job.
			srv["env"] = map[string]string{"CLAWDYX_CONTROL_SOCKET": m.ControlSocket}
		}
		b, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"screen": srv}})
		mcpConfig, appendSystem = string(b), ScreenSystemPrompt(m.Words, m.cfg.Desktop.BrowserApp)
	}
	spec := agent.Spec{Binary: m.cfg.Claude.Binary, Dir: dir, Prompt: prompt, ClaudeSessionID: s.ClaudeSessionID, Resume: s.Started,
		Model: model, Effort: effort, Flags: flags, SettingSources: m.cfg.Claude.SettingSources, StrictMCP: m.cfg.Claude.StrictMCPConfig,
		MaxTurns: m.cfg.Claude.MaxTurns, MaxBudgetUSD: m.cfg.Claude.MaxBudgetUSD, Env: m.childEnv,
		MCPConfig: mcpConfig, AppendSystem: appendSystem, Images: images}

	out := m.execute(j, s, spec)
	// A transcript can exist even though the bot never saw a result (crash
	// before completion). The CLI then rejects --session-id; retry once with --resume.
	if !spec.Resume && out.sessionInUse {
		spec.Resume = true
		out = m.execute(j, s, spec)
	}
	m.complete(j, s, out)
}

type execOut struct {
	o            Outcome
	sawResult    bool
	sawAssistant bool
	sessionInUse bool
	exitCode     int
	cost         float64
}

func (m *Manager) execute(j *store.Job, s *store.Session, spec agent.Spec) execOut {
	var x execOut
	x.o.Effort = spec.Effort
	p, err := m.start(spec)
	if err != nil {
		x.o.State, x.o.ErrorMessage = store.JobFailed, "could not start claude: "+err.Error()
		return x
	}
	rs := &runState{jobID: j.ID, sessionID: j.SessionID, proc: p, stopped: make(chan struct{}), started: time.Now()}
	m.mu.Lock()
	m.running[j.ID] = rs
	shutdown := m.shutdown
	pending := m.starting[j.ID]
	delete(m.starting, j.ID)
	m.mu.Unlock()
	if pending != nil {
		select {
		case <-pending.cancel:
			m.stop(rs, "cancel")
		default:
		}
	}
	_ = m.st.MarkJobStarted(j.ID, p.PID, p.StartUnix)
	j.State, j.PID, j.ProcStartUnix, j.StartedAt = store.JobRunning, p.PID, p.StartUnix, rs.started
	jc := *j
	m.notify.JobStarted(&jc, s)
	if shutdown {
		m.stop(rs, "shutdown")
	}

	timer := time.AfterFunc(m.cfg.Claude.JobTimeout.Duration, func() { m.stop(rs, "timeout") })
	defer timer.Stop()
	watchDone := make(chan struct{})
	defer close(watchDone)
	go m.watchRSS(rs, watchDone)

	var text strings.Builder
	usage := map[string]agent.Usage{}
	var assistantText strings.Builder
	var result *agent.Result
	seq := 0
	lastProgress := time.Time{}
	prog := Progress{}
	maxOut := m.cfg.Claude.MaxOutputBytes
	addEvent := func(typ, payload string) {
		seq++
		_ = m.st.AddJobEvent(j.ID, seq, typ, observability.Redact(payload))
	}
	for ev := range p.Events() {
		switch ev.Kind {
		case agent.KindInit:
			x.o.ResolvedModel = ev.Model
		case agent.KindTextDelta:
			if ev.Sub {
				continue
			}
			if text.Len() < maxOut {
				text.WriteString(ev.Text)
			} else {
				x.o.Truncated = true
			}
		case agent.KindAssistant:
			x.sawAssistant = true
			if ev.MessageID != "" {
				usage[ev.MessageID] = ev.Usage
			}
			if !ev.Sub && ev.Text != "" && assistantText.Len() < maxOut {
				assistantText.WriteString(ev.Text)
			}
			for _, t := range ev.Tools {
				prog.Tools++
				prog.LastTool = strings.TrimSpace(t.Name + " " + t.Input)
				addEvent("tool_use", prog.LastTool)
			}
		case agent.KindDenied:
			if ev.Denial != nil {
				prog.Denials++
				addEvent("denial", ev.Denial.Summary())
			}
		case agent.KindRateLimit:
			if ev.RateLimit != nil {
				m.recordRateLimit(ev.RateLimit)
			}
		case agent.KindRetry:
			addEvent("api_retry", ev.Status)
		case agent.KindResult:
			result = ev.Result
		}
		if ev.Kind == agent.KindTextDelta || ev.Kind == agent.KindAssistant || ev.Kind == agent.KindDenied {
			if time.Since(lastProgress) > 250*time.Millisecond {
				lastProgress = time.Now()
				prog.Text = text.String()
				if prog.Text == "" {
					prog.Text = assistantText.String()
				}
				prog.Elapsed = time.Since(rs.started)
				jc := *j
				m.notify.JobProgress(&jc, prog)
			}
		}
	}
	<-p.Done()
	x.exitCode = p.ExitCode()
	m.mu.Lock()
	delete(m.running, j.ID)
	reason := rs.reason
	m.mu.Unlock()

	stderr := p.Stderr()
	if strings.Contains(stderr, "Unknown --effort value") {
		x.o.Warnings = append(x.o.Warnings, fmt.Sprintf("the CLI ignored effort %q and used its default; effort reset to auto", spec.Effort))
		x.o.Effort = "auto(cli-default)"
		_ = m.st.SetSessionField(s.ID, "effort", "auto")
	}
	if p.ParseErrors() > 0 {
		x.o.Warnings = append(x.o.Warnings, fmt.Sprintf("%d unparseable output line(s) skipped", p.ParseErrors()))
	}
	for _, u := range usage {
		x.o.Usage = x.o.Usage.Add(u)
	}
	x.o.Duration = time.Since(rs.started)
	x.o.Text = text.String()
	if x.o.Text == "" {
		x.o.Text = assistantText.String()
	}
	if result != nil {
		x.sawResult = true
		x.o.NumTurns = result.NumTurns
		x.cost = result.TotalCostUSD
		if rt := result.Text(); rt != "" && !result.IsError {
			if len(rt) > maxOut {
				rt, x.o.Truncated = rt[:maxOut], true
			}
			x.o.Text = rt
		}
		for _, d := range result.PermissionDenials {
			x.o.Denials = append(x.o.Denials, observability.Truncate(d.Summary(), 140))
		}
		if x.o.ResolvedModel == "" && len(result.ModelUsage) == 1 {
			for k := range result.ModelUsage {
				x.o.ResolvedModel = k
			}
		}
	}

	switch {
	case reason == "cancel":
		x.o.State = store.JobCancelled
	case reason == "shutdown":
		x.o.State = store.JobInterrupted
		x.o.ErrorMessage = "the bot was shutting down"
	case reason == "timeout":
		x.o.State = store.JobTimeout
		x.o.ErrorMessage = fmt.Sprintf("timed out after %s", m.cfg.Claude.JobTimeout.Duration)
	case reason == "rss":
		x.o.State = store.JobFailed
		x.o.ErrorMessage = fmt.Sprintf("stopped: memory above %d MB", m.cfg.Claude.MaxChildRSSMB)
	case result == nil:
		x.o.State = store.JobFailed
		line := firstLine(observability.Redact(stderr))
		switch {
		case strings.Contains(stderr, "is already in use"):
			x.sessionInUse = true
			x.o.ErrorMessage = "the Claude session is already in use by another process"
		case strings.Contains(stderr, "No conversation found"):
			x.o.SessionMissing = true
			x.o.ErrorMessage = "the Claude session transcript no longer exists"
		case line != "":
			x.o.ErrorMessage = line
		default:
			x.o.ErrorMessage = fmt.Sprintf("claude exited with code %d and no result", x.exitCode)
		}
	case result.IsError && strings.Contains(result.ErrorText()+stderr, "No conversation found"):
		// CLI 2.1.292 reports a missing transcript as an error result as well.
		x.o.State = store.JobFailed
		x.o.SessionMissing = true
		x.o.ErrorMessage = "the Claude session transcript no longer exists"
	case result.IsError:
		x.o.State = store.JobFailed
		msg := result.Text()
		if msg == "" {
			msg = result.ErrorText()
		}
		if msg == "" {
			msg = strings.TrimSuffix(result.Subtype+" / "+result.TerminalReason, " / ")
		}
		x.o.ErrorMessage = observability.Truncate(observability.Redact(msg), 400)
	default:
		x.o.State = store.JobCompleted
	}
	return x
}

func (m *Manager) complete(j *store.Job, s *store.Session, x execOut) {
	o := x.o
	j.State = o.State
	j.ExitCode = x.exitCode
	j.IsError = o.State != store.JobCompleted
	j.NumTurns = o.NumTurns
	j.InTokens, j.OutTokens = o.Usage.InputTokens, o.Usage.OutputTokens
	j.CacheReadTokens, j.CacheCreate = o.Usage.CacheReadInputTokens, o.Usage.CacheCreationInputTokens
	j.CostUSDEst = 0 // result totals are session-cumulative on resume; per-job cost is not derivable
	j.ResolvedModel = o.ResolvedModel
	j.ResultPreview = observability.Truncate(observability.Redact(o.Text), 2000)
	j.ErrorSummary = o.ErrorMessage
	if x.sawResult {
		j.TerminalReason = "result"
	}
	_ = m.st.FinishJob(*j)
	if x.cost > 0 {
		// Store the session-cumulative estimate so /usage can show the latest total per session.
		_ = m.st.SetSetting(fmt.Sprintf("session_cost_%d", s.ID), fmt.Sprintf("%.4f", x.cost))
	}
	started := x.sawResult || x.sawAssistant || s.Started
	turn := 0
	if x.sawResult {
		turn = 1
	}
	_ = m.st.TouchSession(s.ID, j.ID, started, turn)
	if o.SessionMissing {
		_ = m.st.SetSessionField(s.ID, "status", "missing")
	}
	m.st.Audit("job", "finish", j.Label(), o.State, j.ID)
	m.log.Info("job finished", "job_id", j.ID, "session_id", s.ID, "state", o.State, "duration_ms", o.Duration.Milliseconds(), "exit_code", x.exitCode)
	if s2, err := m.st.GetSession(s.ID); err == nil {
		s = s2
	}
	jc := *j
	m.notify.JobFinished(&jc, s, o)
}

func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return observability.Truncate(l, 300)
		}
	}
	return ""
}

func (m *Manager) watchRSS(rs *runState, done <-chan struct{}) {
	limit := int64(m.cfg.Claude.MaxChildRSSMB) * 1024
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			if processes.RSSKB(rs.proc.PID) > limit {
				m.stop(rs, "rss")
				return
			}
		}
	}
}

// stop runs the cancellation ladder once per job.
func (m *Manager) stop(rs *runState, reason string) {
	rs.once.Do(func() {
		m.mu.Lock()
		rs.reason = reason
		m.mu.Unlock()
		_ = m.st.SetJobState(rs.jobID, store.JobCancelling)
		go func() {
			rs.proc.Stop(m.IntGrace, m.TermGrace)
			close(rs.stopped)
		}()
	})
}

// Cancel cancels a queued or running job. It returns the state the job was
// in ("queued" or "running").
func (m *Manager) Cancel(id int64) (string, error) {
	m.mu.Lock()
	if rs, ok := m.running[id]; ok {
		m.mu.Unlock()
		m.stop(rs, "cancel")
		m.st.Audit("job", "cancel", fmt.Sprintf("j-%d", id), "running", id)
		return "running", nil
	}
	if w, ok := m.waiting[id]; ok {
		m.mu.Unlock()
		w.once.Do(func() { close(w.cancel) })
		m.st.Audit("job", "cancel", fmt.Sprintf("j-%d", id), "queued", id)
		return "queued", nil
	}
	if w, ok := m.starting[id]; ok {
		m.mu.Unlock()
		w.once.Do(func() { close(w.cancel) })
		m.st.Audit("job", "cancel", fmt.Sprintf("j-%d", id), "starting", id)
		return "starting", nil
	}
	for sid, q := range m.queues {
		for i, qid := range q {
			if qid == id {
				m.queues[sid] = append(q[:i:i], q[i+1:]...)
				ready := m.ready[id]
				delete(m.ready, id)
				m.mu.Unlock()
				if ready != nil {
					<-ready
				}
				m.finishUnstarted(id, store.JobCancelled, "cancelled while queued")
				m.st.Audit("job", "cancel", fmt.Sprintf("j-%d", id), "queued", id)
				return "queued", nil
			}
		}
	}
	m.mu.Unlock()
	return "", ErrNotActive
}

// Active describes a queued or running job.
type Active struct {
	JobID     int64
	SessionID int64
	State     string // queued | running | cancelling
	PID       int
	StartUnix int64
	Started   time.Time
}

// ActiveJobs lists running and queued jobs.
func (m *Manager) ActiveJobs() []Active {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Active
	for id, rs := range m.running {
		st := store.JobRunning
		if rs.reason != "" {
			st = store.JobCancelling
		}
		out = append(out, Active{JobID: id, SessionID: rs.sessionID, State: st, PID: rs.proc.PID, StartUnix: rs.proc.StartUnix, Started: rs.started})
	}
	for id, w := range m.waiting {
		out = append(out, Active{JobID: id, SessionID: w.sessionID, State: store.JobQueued})
	}
	for id, w := range m.starting {
		out = append(out, Active{JobID: id, SessionID: w.sessionID, State: "starting"})
	}
	for sid, q := range m.queues {
		for _, id := range q {
			out = append(out, Active{JobID: id, SessionID: sid, State: store.JobQueued})
		}
	}
	sortActive(out)
	return out
}

func sortActive(a []Active) {
	for i := 1; i < len(a); i++ {
		for k := i; k > 0 && a[k].JobID < a[k-1].JobID; k-- {
			a[k], a[k-1] = a[k-1], a[k]
		}
	}
}

// RunningCount returns the number of running jobs.
func (m *Manager) RunningCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.running)
}

// SlotsFull reports whether all concurrency slots are taken.
func (m *Manager) SlotsFull() bool { return m.RunningCount() >= cap(m.sem) }

func (m *Manager) recordRateLimit(rl *agent.RateLimit) {
	m.rlMu.Lock()
	m.rateLimit, m.rlAt = rl, time.Now()
	m.rlMu.Unlock()
	b, _ := json.Marshal(struct {
		At int64
		RL agent.RateLimit
	}{time.Now().Unix(), *rl})
	_ = m.st.SetSetting(KeyRateLimit, string(b))
}

// RateLimit returns the last observed rate-limit info and when it was seen.
func (m *Manager) RateLimit() (*agent.RateLimit, time.Time) {
	m.rlMu.Lock()
	defer m.rlMu.Unlock()
	return m.rateLimit, m.rlAt
}

// Interrupted is a job found unfinished at startup.
type Interrupted struct {
	Job    store.Job
	Killed bool
}

// Recover marks jobs left running/queued by a previous process as
// interrupted, terminating any still-alive child that provably belongs to
// the bot. Prompts are never re-run.
func (m *Manager) Recover() []Interrupted {
	js, err := m.st.JobsInStates(store.JobQueued, store.JobRunning, store.JobCancelling)
	if err != nil {
		m.log.Error("recover: list jobs", "err", err)
		return nil
	}
	var out []Interrupted
	for _, j := range js {
		killed := false
		if j.PID > 0 && agent.OwnedAlive(j.PID, j.ProcStartUnix) {
			_ = agent.KillGroup(j.PID, false)
			for i := 0; i < 50 && agent.OwnedAlive(j.PID, j.ProcStartUnix); i++ {
				time.Sleep(100 * time.Millisecond)
			}
			if agent.OwnedAlive(j.PID, j.ProcStartUnix) {
				_ = agent.KillGroup(j.PID, true)
			}
			killed = true
		}
		j.State, j.ErrorSummary = store.JobInterrupted, "bot restarted; not re-run"
		_ = m.st.FinishJob(j)
		m.st.Audit("job", "recover", j.Label(), "interrupted", j.ID)
		out = append(out, Interrupted{Job: j, Killed: killed})
	}
	return out
}

// Shutdown refuses new work, stops running jobs and waits (bounded by ctx).
func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	if !m.shutdown {
		m.shutdown = true
		close(m.shutdownCh)
	}
	var rss []*runState
	for _, rs := range m.running {
		rss = append(rss, rs)
	}
	var queued []int64
	for sid, q := range m.queues {
		queued = append(queued, q...)
		delete(m.queues, sid)
	}
	m.mu.Unlock()
	for _, id := range queued {
		m.mu.Lock()
		ready := m.ready[id]
		delete(m.ready, id)
		m.mu.Unlock()
		if ready != nil {
			<-ready
		}
		m.finishUnstarted(id, store.JobInterrupted, "the bot was shutting down")
	}
	for _, rs := range rss {
		m.stop(rs, "shutdown")
	}
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		m.log.Warn("shutdown: jobs still running at deadline")
	}
}
