package jobs

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/agent"
	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/sessions"
	"github.com/Publicthrone547/clawdyX/internal/store"
)

var fakeBin string

// waitTimeout bounds how long tests wait for a job (live tests raise it).
var waitTimeout = 20 * time.Second

func TestMain(m *testing.M) {
	dir, _ := os.MkdirTemp("", "fakeclaude")
	fakeBin = filepath.Join(dir, "claude")
	if out, err := exec.Command("go", "build", "-o", fakeBin, "../agent/testdata/fakeclaude").CombinedOutput(); err != nil {
		panic(string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type rec struct {
	mu       sync.Mutex
	queued   []int64
	started  []int64
	finished map[int64]Outcome
	order    []string
	done     chan int64
}

func newRec() *rec { return &rec{finished: map[int64]Outcome{}, done: make(chan int64, 100)} }

func (r *rec) JobQueued(j *store.Job, s *store.Session, pos int) {
	r.mu.Lock()
	r.queued = append(r.queued, j.ID)
	r.order = append(r.order, "q"+j.Label())
	r.mu.Unlock()
}
func (r *rec) JobStarted(j *store.Job, s *store.Session) {
	r.mu.Lock()
	r.started = append(r.started, j.ID)
	r.order = append(r.order, "s"+j.Label())
	r.mu.Unlock()
}
func (r *rec) JobProgress(j *store.Job, p Progress) {}
func (r *rec) JobFinished(j *store.Job, s *store.Session, o Outcome) {
	r.mu.Lock()
	r.finished[j.ID] = o
	r.order = append(r.order, "f"+j.Label())
	r.mu.Unlock()
	r.done <- j.ID
}

func (r *rec) wait(t *testing.T, id int64) Outcome {
	t.Helper()
	timeout := time.After(waitTimeout)
	for {
		r.mu.Lock()
		o, ok := r.finished[id]
		r.mu.Unlock()
		if ok {
			return o
		}
		select {
		case <-r.done:
		case <-timeout:
			t.Fatalf("job %d did not finish", id)
		}
	}
}

type env struct {
	m    *Manager
	st   *store.Store
	sess *sessions.Service
	rec  *rec
	ws   string
}

func setup(t *testing.T, mut func(*config.Config)) *env {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	dir := t.TempDir()
	canon, _ := filepath.EvalSymlinks(dir)
	cfg := &config.Config{
		Workspaces:       []config.Workspace{{Name: "w", Path: dir, Canonical: canon, DefaultMode: "observe", DeniedCommands: []string{"Bash(rm *)"}}},
		DefaultWorkspace: "w",
		Claude: config.Claude{Binary: fakeBin, DefaultModel: "fable", DefaultEffort: "auto", JobTimeout: config.Duration{Duration: 20 * time.Second},
			MaxTurns: 5, MaxConcurrentJobs: 2, MaxQueuePerSession: 3, MaxOutputBytes: 1 << 20, MaxChildRSSMB: 4096, ChildPath: "/usr/bin:/bin"},
	}
	if mut != nil {
		mut(cfg)
	}
	ss := sessions.New(st, cfg)
	r := newRec()
	m := New(cfg, st, ss, slog.New(slog.NewTextHandler(io.Discard, nil)), r, nil)
	m.IntGrace, m.TermGrace = 2*time.Second, time.Second
	t.Cleanup(func() { m.Shutdown(t.Context()) })
	return &env{m: m, st: st, sess: ss, rec: r, ws: canon}
}

func (e *env) session(t *testing.T) *store.Session {
	s, _, err := e.sess.EnsureActive()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func readArgs(t *testing.T, dir string) []string {
	var a []string
	b, _ := os.ReadFile(filepath.Join(dir, ".fakeclaude-args"))
	_ = json.Unmarshal(b, &a)
	return a
}

func TestHappyPathThenResume(t *testing.T) {
	e := setup(t, nil)
	s := e.session(t)
	j, pos, err := e.m.Submit(Request{Session: s, Prompt: "hello", UpdateID: 1})
	if err != nil || pos != 0 {
		t.Fatal(err, pos)
	}
	o := e.rec.wait(t, j.ID)
	if o.State != store.JobCompleted || o.Text != "Hello world" || o.Usage.OutputTokens != 7 || o.ResolvedModel != "claude-fable-5-1" {
		t.Fatalf("outcome %+v", o)
	}
	args := strings.Join(readArgs(t, e.ws), " ")
	if !strings.Contains(args, "--session-id "+s.ClaudeSessionID) || !strings.Contains(args, "--model fable") {
		t.Fatalf("first args %s", args)
	}
	s2, _ := e.st.GetSession(s.ID)
	if !s2.Started || s2.Turns != 1 {
		t.Fatalf("session not marked started: %+v", s2)
	}
	j2, _, _ := e.m.Submit(Request{Session: s2, Prompt: "again", UpdateID: 2})
	e.rec.wait(t, j2.ID)
	if args := strings.Join(readArgs(t, e.ws), " "); !strings.Contains(args, "--resume "+s.ClaudeSessionID) {
		t.Fatalf("second job must resume: %s", args)
	}
	if _, _, err := e.m.Submit(Request{Session: s2, Prompt: "dup", UpdateID: 2}); !errors.Is(err, store.ErrDuplicate) {
		t.Fatalf("duplicate update accepted: %v", err)
	}
	got, _ := e.st.GetJob(j.ID)
	if got.PromptPreview != "" || got.PromptSHA256 == "" {
		t.Fatal("prompt text must not be stored")
	}
	e.rec.mu.Lock()
	order := strings.Join(e.rec.order, ",")
	e.rec.mu.Unlock()
	if !strings.HasPrefix(order, "qj-1,sj-1,fj-1") {
		t.Fatalf("notification order %s", order)
	}
}

func TestSerializationCancelAndQueue(t *testing.T) {
	e := setup(t, nil)
	s := e.session(t)
	slow, _, _ := e.m.Submit(Request{Session: s, Prompt: "SLOW", UpdateID: 10})
	next, pos, _ := e.m.Submit(Request{Session: s, Prompt: "hello", UpdateID: 11})
	if pos != 1 {
		t.Fatalf("position %d", pos)
	}
	time.Sleep(400 * time.Millisecond)
	act := e.m.ActiveJobs()
	if len(act) != 2 || act[0].State != store.JobRunning || act[1].State != store.JobQueued {
		t.Fatalf("active %+v", act)
	}
	third, _, _ := e.m.Submit(Request{Session: s, Prompt: "hello", UpdateID: 12})
	if st, err := e.m.Cancel(third.ID); err != nil || st != "queued" {
		t.Fatalf("cancel queued: %v %s", err, st)
	}
	if o := e.rec.wait(t, third.ID); o.State != store.JobCancelled {
		t.Fatalf("third %+v", o)
	}
	if st, err := e.m.Cancel(slow.ID); err != nil || st != "running" {
		t.Fatal(err, st)
	}
	if o := e.rec.wait(t, slow.ID); o.State != store.JobCancelled {
		t.Fatalf("slow %+v", o)
	}
	if o := e.rec.wait(t, next.ID); o.State != store.JobCompleted {
		t.Fatalf("next %+v", o)
	}
	if _, err := e.m.Cancel(slow.ID); !errors.Is(err, ErrNotActive) {
		t.Fatal("cancel of finished job must fail")
	}
	// Queue limit.
	e.m.Submit(Request{Session: s, Prompt: "SLOW", UpdateID: 20})
	time.Sleep(200 * time.Millisecond)
	for i := 0; i < 3; i++ {
		if _, _, err := e.m.Submit(Request{Session: s, Prompt: "hello", UpdateID: int64(21 + i)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := e.m.Submit(Request{Session: s, Prompt: "hello", UpdateID: 30}); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("expected queue full, got %v", err)
	}
}

func TestGlobalConcurrencyAndWaitingCancel(t *testing.T) {
	e := setup(t, func(c *config.Config) { c.Claude.MaxConcurrentJobs = 1 })
	s1 := e.session(t)
	s2, _ := e.sess.Create("b", nil)
	a, _, _ := e.m.Submit(Request{Session: s1, Prompt: "SLOW", UpdateID: 1})
	// Wait until a holds the only slot, so b must wait (either job may win otherwise).
	for i := 0; i < 100 && e.m.RunningCount() == 0; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	b, _, _ := e.m.Submit(Request{Session: s2, Prompt: "hello", UpdateID: 2})
	time.Sleep(300 * time.Millisecond)
	if e.m.RunningCount() != 1 || !e.m.SlotsFull() {
		t.Fatal("concurrency cap not enforced")
	}
	if st, _ := e.m.Cancel(b.ID); st != "queued" {
		t.Fatalf("waiting job cancel state %q", st)
	}
	if o := e.rec.wait(t, b.ID); o.State != store.JobCancelled {
		t.Fatalf("b %+v", o)
	}
	e.m.Cancel(a.ID)
	e.rec.wait(t, a.ID)
}

func TestTimeoutWarningsMissingAndDenials(t *testing.T) {
	e := setup(t, func(c *config.Config) { c.Claude.JobTimeout.Duration = 600 * time.Millisecond })
	s := e.session(t)
	j, _, _ := e.m.Submit(Request{Session: s, Prompt: "SLOW", UpdateID: 1})
	if o := e.rec.wait(t, j.ID); o.State != store.JobTimeout {
		t.Fatalf("timeout %+v", o)
	}
	_ = e.st.SetSessionField(s.ID, "effort", "bogus")
	s, _ = e.st.GetSession(s.ID)
	j2, _, _ := e.m.Submit(Request{Session: s, Prompt: "DENY", UpdateID: 2})
	o := e.rec.wait(t, j2.ID)
	if len(o.Warnings) == 0 || len(o.Denials) != 1 || o.Denials[0] != "Bash touch x.txt" {
		t.Fatalf("warnings/denials %+v", o)
	}
	if s3, _ := e.st.GetSession(s.ID); s3.Effort != "auto" {
		t.Fatalf("effort not reset: %q", s3.Effort)
	}
	evs, _ := e.st.JobEvents(j2.ID, 10)
	if len(evs) < 2 {
		t.Fatalf("events %+v", evs)
	}
	// Missing transcript.
	miss, _ := e.st.CreateSession(store.Session{ClaudeSessionID: "00000000-0000-4000-8000-000000000000", WorkspaceName: "w", WorkspacePath: e.ws, Mode: "observe", Started: true})
	j3, _, _ := e.m.Submit(Request{Session: miss, Prompt: "hi", UpdateID: 3})
	o3 := e.rec.wait(t, j3.ID)
	if !o3.SessionMissing || o3.State != store.JobFailed {
		t.Fatalf("missing %+v", o3)
	}
	if ms, _ := e.st.GetSession(miss.ID); ms.Status != "missing" {
		t.Fatal("session not marked missing")
	}
	j4, _, _ := e.m.Submit(Request{Session: s, Prompt: "FAIL", UpdateID: 4})
	if o := e.rec.wait(t, j4.ID); o.State != store.JobFailed || !strings.Contains(o.ErrorMessage, "overloaded") {
		t.Fatalf("fail %+v", o)
	}
}

func TestPausedAndShutdown(t *testing.T) {
	e := setup(t, nil)
	s := e.session(t)
	e.m.SetPaused(true)
	if _, _, err := e.m.Submit(Request{Session: s, Prompt: "x", UpdateID: 1}); !errors.Is(err, ErrPaused) {
		t.Fatal("paused submit accepted")
	}
	e.m.SetPaused(false)
	j, _, _ := e.m.Submit(Request{Session: s, Prompt: "SLOW", UpdateID: 2})
	q, _, _ := e.m.Submit(Request{Session: s, Prompt: "hello", UpdateID: 3})
	time.Sleep(300 * time.Millisecond)
	e.m.Shutdown(t.Context())
	if o := e.rec.wait(t, j.ID); o.State != store.JobInterrupted {
		t.Fatalf("running at shutdown: %+v", o)
	}
	if o := e.rec.wait(t, q.ID); o.State != store.JobInterrupted {
		t.Fatalf("queued at shutdown: %+v", o)
	}
	if _, _, err := e.m.Submit(Request{Session: s, Prompt: "x", UpdateID: 4}); !errors.Is(err, ErrShutdown) {
		t.Fatal("submit after shutdown accepted")
	}
}

func TestRecoverKillsOwnedOrphan(t *testing.T) {
	e := setup(t, nil)
	s := e.session(t)
	p, err := agent.Start(agent.Spec{Binary: fakeBin, Dir: e.ws, Prompt: "SLOW", ClaudeSessionID: s.ClaudeSessionID, Flags: struct {
		PermissionMode string
		Allowed        []string
		Disallowed     []string
		SettingsJSON   string
	}{PermissionMode: "dontAsk"}, Env: agent.ChildEnv("/usr/bin:/bin", 0)})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for range p.Events() {
		}
	}()
	j, _ := e.st.InsertJob(store.Job{SessionID: s.ID, TgUpdateID: 99})
	_ = e.st.MarkJobStarted(j.ID, p.PID, p.StartUnix)
	q, _ := e.st.InsertJob(store.Job{SessionID: s.ID, TgUpdateID: 100})
	got := e.m.Recover()
	if len(got) != 2 || !got[0].Killed {
		t.Fatalf("recover %+v", got)
	}
	select {
	case <-p.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("orphan still alive")
	}
	for _, id := range []int64{j.ID, q.ID} {
		if jj, _ := e.st.GetJob(id); jj.State != store.JobInterrupted {
			t.Fatalf("job %d state %s", id, jj.State)
		}
	}
}

func TestScreenControlOnlyInAutomation(t *testing.T) {
	e := setup(t, func(c *config.Config) {
		c.Modes.AutomationEnabled = true
		c.Desktop.ScreenControl = true
	})
	e.m.ScreenCommand = "/usr/local/bin/clawdyx"
	s := e.session(t)
	j, _, _ := e.m.Submit(Request{Session: s, Prompt: "hello", UpdateID: 1})
	e.rec.wait(t, j.ID)
	if a := strings.Join(readArgs(t, e.ws), " "); strings.Contains(a, "mcp-screen") {
		t.Fatalf("observe session got screen tools: %s", a)
	}
	_ = e.st.SetSessionField(s.ID, "mode", "automation")
	s, _ = e.st.GetSession(s.ID)
	j2, _, _ := e.m.Submit(Request{Session: s, Prompt: "hello", UpdateID: 2})
	e.rec.wait(t, j2.ID)
	a := strings.Join(readArgs(t, e.ws), " ")
	if !strings.Contains(a, `"command":"/usr/local/bin/clawdyx"`) || !strings.Contains(a, `"args":["mcp-screen"]`) || !strings.Contains(a, "untrusted data") || !strings.Contains(a, "--permission-mode auto") {
		t.Fatalf("automation screen args: %s", a)
	}
}
