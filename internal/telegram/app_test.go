package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/platform"
	"github.com/Publicthrone547/clawdyX/internal/screen"
	"github.com/Publicthrone547/clawdyX/internal/store"
	"github.com/Publicthrone547/clawdyX/internal/vault"
)

const owner int64 = 111111111

var fakeBin string

func TestMain(m *testing.M) {
	config.TokenVault = vault.NewMemory() // never touch the real Keychain
	dir, _ := os.MkdirTemp("", "fakeclaude")
	fakeBin = filepath.Join(dir, "claude"+platform.ExeSuffix)
	if out, err := exec.Command("go", "build", "-o", fakeBin, "../agent/testdata/fakeclaude").CombinedOutput(); err != nil {
		panic(string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type call struct {
	Method string
	Form   map[string]string
}

// fakeTG is a minimal Bot API server.
type fakeTG struct {
	mu        sync.Mutex
	calls     []call
	updates   []json.RawMessage
	nextMsg   int
	nextUpd   int64
	newUpd    chan struct{}
	failDraft bool
}

func newFakeTG() *fakeTG { return &fakeTG{nextMsg: 1000, nextUpd: 1, newUpd: make(chan struct{}, 100)} }

func (f *fakeTG) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	method := parts[len(parts)-1]
	_ = r.ParseMultipartForm(1 << 20)
	form := map[string]string{}
	for k, v := range r.Form {
		form[k] = v[0]
	}
	ok := func(result any) {
		b, _ := json.Marshal(map[string]any{"ok": true, "result": result})
		w.Write(b)
	}
	if method != "getUpdates" {
		f.mu.Lock()
		f.calls = append(f.calls, call{method, form})
		f.mu.Unlock()
	}
	switch method {
	case "getMe":
		ok(map[string]any{"id": 1, "is_bot": true, "first_name": "t", "username": "tbot"})
	case "getWebhookInfo":
		ok(map[string]any{"url": ""})
	case "getUpdates":
		off, _ := strconv.ParseInt(form["offset"], 10, 64)
		deadline := time.After(300 * time.Millisecond)
		for {
			f.mu.Lock()
			var out []json.RawMessage
			for _, u := range f.updates {
				var h struct {
					ID int64 `json:"update_id"`
				}
				_ = json.Unmarshal(u, &h)
				if h.ID >= off {
					out = append(out, u)
				}
			}
			f.mu.Unlock()
			if len(out) > 0 {
				ok(out)
				return
			}
			select {
			case <-f.newUpd:
			case <-deadline:
				ok([]any{})
				return
			case <-r.Context().Done():
				return
			}
		}
	case "sendMessage", "editMessageText":
		f.mu.Lock()
		f.nextMsg++
		id := f.nextMsg
		f.mu.Unlock()
		if method == "editMessageText" {
			id, _ = strconv.Atoi(form["message_id"])
		}
		ok(map[string]any{"message_id": id, "date": 1, "chat": map[string]any{"id": owner, "type": "private"}, "text": form["text"]})
	case "sendMessageDraft":
		if f.failDraft {
			w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: method not available"}`))
			return
		}
		ok(true)
	default:
		ok(true)
	}
}

func (f *fakeTG) push(u map[string]any) int64 {
	f.mu.Lock()
	id := f.nextUpd
	f.nextUpd++
	u["update_id"] = id
	b, _ := json.Marshal(u)
	f.updates = append(f.updates, b)
	f.mu.Unlock()
	f.newUpd <- struct{}{}
	return id
}

func (f *fakeTG) pushRaw(u map[string]any) {
	b, _ := json.Marshal(u)
	f.mu.Lock()
	f.updates = append(f.updates, b)
	f.mu.Unlock()
	f.newUpd <- struct{}{}
}

var msgSeq = 1

func msg(from int64, chatID int64, chatType, text string) map[string]any {
	msgSeq++
	return map[string]any{"message": map[string]any{"message_id": msgSeq, "date": 1, "text": text,
		"from": map[string]any{"id": from, "is_bot": false, "first_name": "x"},
		"chat": map[string]any{"id": chatID, "type": chatType}}}
}

func callback(from int64, data string, msgID int) map[string]any {
	return map[string]any{"callback_query": map[string]any{"id": fmt.Sprint(time.Now().UnixNano()), "data": data, "chat_instance": "c",
		"from":    map[string]any{"id": from, "is_bot": false, "first_name": "x"},
		"message": map[string]any{"message_id": msgID, "date": 1, "chat": map[string]any{"id": owner, "type": "private"}}}}
}

func (f *fakeTG) sent() []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]call(nil), f.calls...)
}

// waitFor polls until a sendMessage/editMessageText whose text contains s appears.
func (f *fakeTG) waitFor(t *testing.T, s string) call {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		for _, c := range f.sent() {
			if (c.Method == "sendMessage" || c.Method == "editMessageText") && strings.Contains(c.Form["text"], s) {
				return c
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	var texts []string
	for _, c := range f.sent() {
		if c.Method == "sendMessage" || c.Method == "editMessageText" {
			texts = append(texts, c.Form["text"])
		}
	}
	t.Fatalf("no message containing %q; got:\n%s", s, strings.Join(texts, "\n---\n"))
	return call{}
}

type harness struct {
	tg  *fakeTG
	app *App
	st  *store.Store
	ws  string
	cfg *config.Config
}

func start(t *testing.T) *harness { return startWith(t, nil, nil) }

func startWith(t *testing.T, mut func(*config.Config), opener func(ctx context.Context, app, url string) error) *harness {
	return startFull(t, mut, opener, nil)
}

func startFull(t *testing.T, mut func(*config.Config), opener func(ctx context.Context, app, url string) error, desk screen.Desktop) *harness {
	t.Helper()
	return startOpts(t, mut, Options{Opener: opener, Desktop: desk})
}

// startOpts starts the app with opt (ServerURL is filled in).
func startOpts(t *testing.T, mut func(*config.Config), opt Options) *harness {
	t.Helper()
	tg := newFakeTG()
	srv := httptest.NewServer(tg)
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	canon, _ := filepath.EvalSymlinks(dir)
	state := t.TempDir()
	cfg := &config.Config{
		Telegram:         config.Telegram{OwnerUserID: owner, PollTimeoutSeconds: 5},
		Workspaces:       []config.Workspace{{Name: "w", Path: dir, Canonical: canon, DefaultMode: "observe", DeniedCommands: []string{"Bash(rm *)"}}},
		DefaultWorkspace: "w",
		StateDir:         state, LogDir: state,
		Claude: config.Claude{Binary: fakeBin, DefaultModel: "fable", DefaultEffort: "auto", JobTimeout: config.Duration{Duration: 20 * time.Second},
			MaxTurns: 5, MaxConcurrentJobs: 2, MaxQueuePerSession: 3, MaxOutputBytes: 1 << 20, MaxChildRSSMB: 4096, ChildPath: config.DefaultChildPath()},
		Retention: config.Retention{JobsDays: 90, JobEventsDays: 14, ProcessedUpdatesDays: 7, AuditDays: 180},
		Token:     "123456789:" + strings.Repeat("A", 35),
		Desktop:   config.Desktop{BrowserApp: "Firefox"},
	}
	if mut != nil {
		mut(cfg)
	}
	st, err := store.Open(filepath.Join(state, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	opt.ServerURL = srv.URL
	app, err := New(cfg, st, slog.New(slog.NewTextHandler(io.Discard, nil)), opt)
	if err != nil {
		t.Fatal(err)
	}
	app.Jobs().IntGrace, app.Jobs().TermGrace = 2*time.Second, time.Second
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = app.Run(ctx); close(done) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			t.Error("app did not shut down")
		}
	})
	// Wait for the version check so model validation can use the cache.
	for i := 0; i < 100 && app.CLIVersion() == ""; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	return &harness{tg: tg, app: app, st: st, ws: canon, cfg: cfg}
}

func TestUnauthorizedIsSilent(t *testing.T) {
	h := start(t)
	before := len(h.tg.sent())
	h.tg.push(msg(42, 42, "private", "/status"))
	h.tg.push(msg(42, 42, "private", "rm -rf everything"))
	h.tg.push(msg(owner, -100123, "supergroup", "/status"))                                   // owner, but in a group
	h.tg.push(msg(owner, 42, "private", "hello"))                                             // owner id but other chat
	h.tg.push(callback(42, "y:AAAAAA", 5))                                                    // forged callback from another user
	h.tg.push(map[string]any{"edited_message": msg(owner, owner, "private", "x")["message"]}) // not an allowed type
	time.Sleep(1500 * time.Millisecond)
	for _, c := range h.tg.sent()[before:] {
		if c.Method == "sendMessage" || c.Method == "answerCallbackQuery" || c.Method == "editMessageText" || c.Method == "sendChatAction" {
			t.Fatalf("unauthorized update caused %s: %v", c.Method, c.Form)
		}
	}
	if n, _ := h.st.CountSessions(); n != 0 {
		t.Fatal("unauthorized update created a session")
	}
	if js, _ := h.st.ListJobs(10); len(js) != 0 {
		t.Fatal("unauthorized update created a job")
	}
	if h.app.counters.Unauthorized.Load() < 5 {
		t.Fatalf("unauthorized counter %d", h.app.counters.Unauthorized.Load())
	}
	h.tg.push(msg(owner, owner, "private", "/status"))
	c := h.tg.waitFor(t, "clawdyx")
	if c.Form["chat_id"] != fmt.Sprint(owner) {
		t.Fatalf("reply to wrong chat %s", c.Form["chat_id"])
	}
}

func TestPromptStreamsAndAnswers(t *testing.T) {
	h := start(t)
	h.tg.push(msg(owner, owner, "private", "hello"))
	h.tg.waitFor(t, "Started session")
	h.tg.waitFor(t, "j-1")
	final := h.tg.waitFor(t, "Hello world")
	if final.Form["parse_mode"] != "HTML" {
		t.Fatal("final message must use HTML")
	}
	h.tg.waitFor(t, "— j-1")
	h.tg.waitFor(t, "✅ <b>j-1</b> completed")
	j, _ := h.st.GetJob(1)
	if j.State != store.JobCompleted || j.OutTokens != 7 {
		t.Fatalf("job %+v", j)
	}
}

func TestDuplicateUpdateRunsOnce(t *testing.T) {
	h := start(t)
	u := msg(owner, owner, "private", "hello")
	u["update_id"] = 500
	h.tg.pushRaw(u)
	h.tg.waitFor(t, "Hello world")
	// Same update delivered again after a "restart": a fresh poller would re-read it.
	if first, _ := h.st.MarkUpdate(500); first {
		t.Fatal("update not recorded")
	}
	js, _ := h.st.ListJobs(10)
	if len(js) != 1 {
		t.Fatalf("expected one job, got %d", len(js))
	}
}

func TestModelAndEffortCommands(t *testing.T) {
	h := start(t)
	h.tg.push(msg(owner, owner, "private", "/model gpt-5"))
	h.tg.waitFor(t, "Not changed")
	h.tg.push(msg(owner, owner, "private", "/model sonnet"))
	h.tg.waitFor(t, "set to <b>sonnet</b> → claude-sonnet-5-5")
	h.tg.push(msg(owner, owner, "private", "/model fable"))
	h.tg.waitFor(t, "set to <b>fable</b> → claude-fable-5-1 (primary model")
	h.tg.push(msg(owner, owner, "private", "/effort ultra"))
	h.tg.waitFor(t, "Usage: /effort")
	h.tg.push(msg(owner, owner, "private", "/effort max"))
	h.tg.waitFor(t, "set to <b>max</b>")
	h.tg.push(msg(owner, owner, "private", "/status"))
	h.tg.waitFor(t, "fable/max")
	h.tg.push(msg(owner, owner, "private", "hello"))
	h.tg.waitFor(t, "— j-1")
	b, _ := os.ReadFile(filepath.Join(h.ws, ".fakeclaude-args"))
	args := string(b)
	if !strings.Contains(args, `"--model","fable"`) || !strings.Contains(args, `"--effort","max"`) || strings.Contains(args, "advisor") {
		t.Fatalf("job args %s", args)
	}
	h.tg.push(msg(owner, owner, "private", "/alias set fast haiku"))
	h.tg.waitFor(t, "Alias fast → haiku → claude-haiku")
	h.tg.push(msg(owner, owner, "private", "/model fast"))
	h.tg.waitFor(t, "custom alias fast → haiku")
}

func cancelData(t *testing.T, h *harness, jobLabel string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for _, c := range h.tg.sent() {
			if strings.Contains(c.Form["text"], jobLabel) && strings.Contains(c.Form["reply_markup"], "c:") {
				var mk struct {
					K [][]struct {
						D string `json:"callback_data"`
					} `json:"inline_keyboard"`
				}
				_ = json.Unmarshal([]byte(c.Form["reply_markup"]), &mk)
				if len(mk.K) > 0 && len(mk.K[0]) > 0 {
					return mk.K[0][0].D
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("no cancel button")
	return ""
}

func TestCancelButtonAndForgedNonce(t *testing.T) {
	h := start(t)
	h.tg.push(msg(owner, owner, "private", "SLOW"))
	data := cancelData(t, h, "j-1")
	h.tg.waitFor(t, "running")
	parts := strings.Split(data, ":")
	h.tg.push(callback(owner, "c:1:deadbeef", 1001)) // wrong nonce
	time.Sleep(500 * time.Millisecond)
	if j, _ := h.st.GetJob(1); j.State != store.JobRunning {
		t.Fatalf("forged nonce cancelled the job: %s", j.State)
	}
	h.tg.push(callback(owner, "c:1:"+parts[2], 1001))
	h.tg.waitFor(t, "j-1 cancelled")
}

func TestStopAllNeedsConfirmation(t *testing.T) {
	h := start(t)
	h.tg.push(msg(owner, owner, "private", "SLOW"))
	h.tg.waitFor(t, "running")
	h.tg.push(msg(owner, owner, "private", "/new second"))
	h.tg.waitFor(t, "Session <b>S2</b>")
	h.tg.push(msg(owner, owner, "private", "SLOW"))
	h.tg.waitFor(t, "<b>j-2</b> running")
	h.tg.push(msg(owner, owner, "private", "/stop all"))
	c := h.tg.waitFor(t, "Stop 2 jobs")
	var mk struct {
		K [][]struct {
			D string `json:"callback_data"`
		} `json:"inline_keyboard"`
	}
	_ = json.Unmarshal([]byte(c.Form["reply_markup"]), &mk)
	yes := mk.K[0][0].D
	h.tg.push(msg(owner, owner, "private", "/confirm ZZZZZZ"))
	h.tg.waitFor(t, "unknown, expired, or already used")
	if j, _ := h.st.GetJob(1); j.State != store.JobRunning {
		t.Fatal("job stopped without confirmation")
	}
	h.tg.push(callback(owner, yes, 1003))
	h.tg.waitFor(t, "Stopping: j-1")
	h.tg.waitFor(t, "j-2 cancelled")
	h.tg.push(callback(owner, yes, 1003)) // replay
	h.tg.waitFor(t, "already used")
	h.tg.push(msg(owner, owner, "private", "/stop"))
	h.tg.waitFor(t, "Nothing is running in S2")
}

func TestMiscCommands(t *testing.T) {
	h := start(t)
	// Ordered: several of these must run before any session exists.
	for _, c := range [][2]string{
		{"/help", "Claude control center"},
		{"/nonsense", "Unknown command"},
		{"/jobs", "No jobs yet"},
		{"/mode", "No active session"},
		{"/mode automation", "automation tier is not enabled"},
		{"/sessions", "No sessions yet"},
		{"/cwd", "Workspaces"},
		{"/aliases", "No custom aliases"},
		{"/usage", "Rate limits: unavailable"},
		{"/processes", "Bot-owned Claude jobs"},
		{"/shutdown", "disabled"},
		{"/logs j-99", "No job j-99"},
		{"/switch S9", "No such session"},
		{"/stop x", "Usage: /stop"},
	} {
		h.tg.push(msg(owner, owner, "private", c[0]))
		h.tg.waitFor(t, c[1])
	}
	if n, _ := h.st.CountSessions(); n != 0 {
		t.Fatalf("read-only commands created %d session(s)", n)
	}
	h.tg.push(msg(owner, owner, "private", "/mode assisted"))
	h.tg.waitFor(t, "tier set to <b>assisted</b>")
	h.tg.push(msg(owner, owner, "private", "DENY"))
	h.tg.waitFor(t, "denied")
	h.tg.push(msg(owner, owner, "private", "/logs j-1"))
	h.tg.waitFor(t, "touch x.txt")
	h.tg.push(msg(owner, owner, "private", "/pause"))
	h.tg.waitFor(t, "Paused")
	h.tg.push(msg(owner, owner, "private", "hello"))
	h.tg.waitFor(t, "The bot is paused")
	h.tg.push(msg(owner, owner, "private", "/start"))
	h.tg.waitFor(t, "Resumed")
	b, _ := os.ReadFile(filepath.Join(h.ws, ".fakeclaude-args"))
	if !strings.Contains(string(b), `"acceptEdits"`) {
		t.Fatalf("assisted mode not applied: %s", b)
	}
}

func TestDraftFallbackToEdits(t *testing.T) {
	h := start(t)
	h.tg.failDraft = true
	h.tg.push(msg(owner, owner, "private", "SLOW"))
	h.tg.waitFor(t, "tick")
	h.tg.push(msg(owner, owner, "private", "/stop"))
	h.tg.waitFor(t, "cancelled")
}

func TestOpenCommand(t *testing.T) {
	var mu sync.Mutex
	var got []string
	h := startWith(t, nil, func(ctx context.Context, app, url string) error {
		mu.Lock()
		got = append(got, app+" "+url)
		mu.Unlock()
		return nil
	})
	h.tg.push(msg(owner, owner, "private", "/open youtube.com"))
	h.tg.waitFor(t, "Opened https://youtube.com in "+browserName("Firefox"))
	h.tg.push(msg(owner, owner, "private", "/open file:///etc/passwd"))
	h.tg.waitFor(t, "Not opened")
	h.tg.push(msg(42, 42, "private", "/open evil.com")) // stranger: silent, nothing opened
	time.Sleep(500 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0] != "Firefox https://youtube.com" {
		t.Fatalf("opener calls %v", got)
	}
}

func TestAutomationModeNeedsConfigAndConfirmation(t *testing.T) {
	h := startWith(t, func(c *config.Config) { c.Modes.AutomationEnabled = true }, nil)
	h.tg.push(msg(owner, owner, "private", "/mode automation"))
	c := h.tg.waitFor(t, "Switch S1 to automation?")
	var mk struct {
		K [][]struct {
			D string `json:"callback_data"`
		} `json:"inline_keyboard"`
	}
	_ = json.Unmarshal([]byte(c.Form["reply_markup"]), &mk)
	if s, _ := h.st.GetSession(1); s.Mode != "observe" {
		t.Fatal("mode changed before confirmation")
	}
	h.tg.push(callback(owner, mk.K[0][0].D, 1002))
	h.tg.waitFor(t, "is now in <b>automation</b>")
	h.tg.push(msg(owner, owner, "private", "hello"))
	h.tg.waitFor(t, "— j-1")
	b, _ := os.ReadFile(filepath.Join(h.ws, ".fakeclaude-args"))
	args := string(b)
	if !strings.Contains(args, `"--permission-mode","auto"`) || !strings.Contains(args, "Bash(rm *)") || strings.Contains(args, "bypass") {
		t.Fatalf("automation job args %s", args)
	}
	h.tg.push(msg(owner, owner, "private", "/mode observe"))
	h.tg.waitFor(t, "tier set to <b>observe</b>")
}

type fakeDesk struct{}

func (fakeDesk) Screenshot(ctx context.Context, display string) (screen.Shot, error) {
	return screen.Shot{JPEG: []byte{0xff, 0xd8, 0xff, 0xd9}, Width: 10, Height: 5, Scale: 1}, nil
}
func (fakeDesk) Click(context.Context, float64, float64, string, int) error { return nil }
func (fakeDesk) Move(context.Context, float64, float64) error               { return nil }
func (fakeDesk) Scroll(context.Context, float64, float64, int, int) error   { return nil }
func (fakeDesk) TypeText(context.Context, string) error                     { return nil }
func (fakeDesk) PressKey(context.Context, int, uint64) error                { return nil }
func (fakeDesk) Trusted(context.Context) bool                               { return true }

var fakeCaptures atomic.Int64

func (fakeDesk) Displays(context.Context) ([]screen.Display, error) {
	return []screen.Display{
		{Index: 1, ID: 4, Key: "AAAAAAAA-main", Name: "Wide", W: 400, H: 200, PixelW: 400, PixelH: 200, Scale: 1, Main: true},
		{Index: 2, ID: 5, Key: "BBBBBBBB-side", Name: "Side", X: -300, Y: -200, W: 300, H: 200, PixelW: 300, PixelH: 200, Scale: 1},
	}, nil
}

func (fakeDesk) CapturePNG(_ context.Context, d screen.Display) ([]byte, error) {
	fakeCaptures.Add(1)
	img := image.NewRGBA(image.Rect(0, 0, d.PixelW, d.PixelH))
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes(), nil
}
