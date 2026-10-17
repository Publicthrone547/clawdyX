package agent

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/policy"
)

var fakeBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fakeclaude")
	if err != nil {
		panic(err)
	}
	// The executable must be named "claude" so ownership checks recognise it.
	fakeBin = filepath.Join(dir, "claude")
	out, err := exec.Command("go", "build", "-o", fakeBin, "./testdata/fakeclaude").CombinedOutput()
	if err != nil {
		panic(string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// FakeBinary builds path for other packages' tests.
func spec(t *testing.T, prompt string) Spec {
	t.Helper()
	return Spec{Binary: fakeBin, Dir: t.TempDir(), Prompt: prompt, ClaudeSessionID: "11111111-1111-4111-8111-111111111111",
		Model: "sonnet", Effort: "high", Flags: policy.Flags{PermissionMode: "dontAsk", Disallowed: []string{"Edit", "Bash(rm *)"}, SettingsJSON: `{"x":1}`},
		SettingSources: "user", StrictMCP: true, MaxTurns: 5, Env: ChildEnv("/usr/bin:/bin", 1000)}
}

func collect(p *Process) []Event {
	var evs []Event
	for e := range p.Events() {
		evs = append(evs, e)
	}
	<-p.Done()
	return evs
}

func TestBuildArgsNoPromptAndFlags(t *testing.T) {
	s := spec(t, "secret prompt text")
	args := BuildArgs(s)
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "secret prompt") {
		t.Fatal("prompt leaked into argv")
	}
	for _, want := range []string{"-p", "--session-id " + s.ClaudeSessionID, "--model sonnet", "--effort high", "--permission-mode dontAsk", "--permission-prompts none", "--strict-mcp-config", "--setting-sources user"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %s", want, joined)
		}
	}
	if strings.Contains(joined, "dangerously") || strings.Contains(joined, "bypass") || strings.Contains(joined, "--bare") || strings.Contains(joined, "--advisor") {
		t.Fatal("forbidden flag present")
	}
	if args[len(args)-1] != "Bash(rm *)" {
		t.Fatal("variadic flags must be last")
	}
	s.Resume, s.Effort = true, "auto"
	j2 := strings.Join(BuildArgs(s), " ")
	if !strings.Contains(j2, "--resume "+s.ClaudeSessionID) || strings.Contains(j2, "--effort") || strings.Contains(j2, "--session-id") {
		t.Fatalf("resume args wrong: %s", j2)
	}
}

func TestChildEnvAllowlist(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "123:abc")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-x")
	t.Setenv("CLAUDECODE", "1")
	env := strings.Join(ChildEnv("/usr/bin", 5), "\n")
	for _, bad := range []string{"TELEGRAM_BOT_TOKEN", "ANTHROPIC_API_KEY", "CLAUDECODE"} {
		if strings.Contains(env, bad) {
			t.Fatalf("%s forwarded", bad)
		}
	}
	if !strings.Contains(env, "PATH=/usr/bin") || !strings.Contains(env, "HOME=") {
		t.Fatal("required vars missing")
	}
	if strings.Contains(env, "CLAUDE_CODE_SKIP_PROMPT_HISTORY") {
		t.Fatal("CLAUDE_CODE_SKIP_PROMPT_HISTORY disables transcripts and breaks --resume")
	}
}

func TestRunHappyPath(t *testing.T) {
	s := spec(t, "hello")
	p, err := Start(s)
	if err != nil {
		t.Fatal(err)
	}
	evs := collect(p)
	var text string
	var res *Result
	var init, rl, tool bool
	usage := map[string]Usage{}
	for _, e := range evs {
		switch e.Kind {
		case KindInit:
			init = e.Model == "claude-sonnet-5-5" && e.SessionID == s.ClaudeSessionID
		case KindTextDelta:
			text += e.Text
		case KindAssistant:
			usage[e.MessageID] = e.Usage
			if len(e.Tools) == 1 && e.Tools[0].Name == "Read" && e.Tools[0].Input == "/w/main.go" {
				tool = true
			}
		case KindRateLimit:
			rl = e.RateLimit.Windows["five_hour"].Utilization == 0.15
		case KindResult:
			res = e.Result
		}
	}
	if !init || !rl || !tool || text != "Hello world" || res == nil || res.Text() != "Hello world" || res.IsError {
		t.Fatalf("init=%v rl=%v tool=%v text=%q res=%+v", init, rl, tool, text, res)
	}
	if len(usage) != 1 || usage["m1"].OutputTokens != 7 {
		t.Fatalf("usage must dedupe per message id: %+v", usage)
	}
	if p.ExitCode() != 0 {
		t.Fatalf("exit %d", p.ExitCode())
	}
	var argv []string
	b, _ := os.ReadFile(filepath.Join(s.Dir, ".fakeclaude-args"))
	_ = json.Unmarshal(b, &argv)
	if len(argv) == 0 || argv[0] != "-p" {
		t.Fatalf("argv %v", argv)
	}
	envb, _ := os.ReadFile(filepath.Join(s.Dir, ".fakeclaude-env"))
	if strings.Contains(string(envb), "CLAUDECODE") {
		t.Fatal("nested-session marker reached child")
	}
}

func TestDenialAndGarbage(t *testing.T) {
	p, _ := Start(spec(t, "DENY"))
	var denied bool
	var res *Result
	for _, e := range collect(p) {
		if e.Kind == KindDenied && e.Denial.Summary() == "Bash touch x.txt" {
			denied = true
		}
		if e.Kind == KindResult {
			res = e.Result
		}
	}
	if !denied || len(res.PermissionDenials) != 1 {
		t.Fatal("denial not parsed")
	}
	p2, _ := Start(spec(t, "GARBAGE"))
	evs := collect(p2)
	if p2.ParseErrors() != 1 || evs[len(evs)-1].Kind != KindResult {
		t.Fatalf("garbage handling: errs=%d", p2.ParseErrors())
	}
}

func TestStopSIGINTRecordsResult(t *testing.T) {
	p, _ := Start(spec(t, "SLOW"))
	time.Sleep(300 * time.Millisecond)
	go p.Stop(5*time.Second, 2*time.Second)
	var res *Result
	for e := range p.Events() {
		if e.Kind == KindResult {
			res = e.Result
		}
	}
	<-p.Done()
	if res == nil || res.TerminalReason != "aborted_streaming" || p.ExitCode() != 0 {
		t.Fatalf("res=%+v exit=%d", res, p.ExitCode())
	}
}

func TestStopEscalatesToKillAndKillsGroup(t *testing.T) {
	s := spec(t, "IGNORE_INT SPAWN")
	p, _ := Start(s)
	go func() {
		for range p.Events() {
		}
	}()
	var child int
	for i := 0; i < 50 && child == 0; i++ {
		time.Sleep(50 * time.Millisecond)
		b, _ := os.ReadFile(filepath.Join(s.Dir, ".fakeclaude-child"))
		child, _ = strconv.Atoi(string(b))
	}
	if child == 0 {
		t.Fatal("child not spawned")
	}
	start := time.Now()
	p.Stop(200*time.Millisecond, 200*time.Millisecond)
	if p.ExitCode() != 137 {
		t.Fatalf("expected SIGKILL exit 137, got %d", p.ExitCode())
	}
	if time.Since(start) > 4*time.Second {
		t.Fatal("ladder too slow")
	}
	time.Sleep(100 * time.Millisecond)
	// The grandchild was in the same process group and must be gone (or a zombie reaped by launchd/init).
	if err := syscall.Kill(child, 0); err == nil {
		if ProcessComm(child) == "sleep" {
			t.Fatalf("grandchild %d survived", child)
		}
	}
}

func TestOwnership(t *testing.T) {
	p, _ := Start(spec(t, "SLOW"))
	go func() {
		for range p.Events() {
		}
	}()
	time.Sleep(100 * time.Millisecond)
	if p.StartUnix == 0 || !OwnedAlive(p.PID, p.StartUnix) {
		t.Fatal("own process not recognised")
	}
	if OwnedAlive(p.PID, p.StartUnix-100) {
		t.Fatal("start-time mismatch must fail (PID reuse guard)")
	}
	if OwnedAlive(os.Getpid(), ProcessStartUnix(os.Getpid())) {
		t.Fatal("non-claude process accepted")
	}
	p.Stop(2*time.Second, time.Second)
}

func TestResumeMissing(t *testing.T) {
	s := spec(t, "hi")
	s.Resume, s.ClaudeSessionID = true, "00000000-0000-4000-8000-000000000000"
	p, _ := Start(s)
	evs := collect(p)
	if len(evs) != 0 || p.ExitCode() != 1 || !strings.Contains(p.Stderr(), "No conversation found") {
		t.Fatalf("evs=%d exit=%d stderr=%q", len(evs), p.ExitCode(), p.Stderr())
	}
}

func TestCLIHelpers(t *testing.T) {
	c := CLI{Binary: fakeBin, Env: ChildEnv("/usr/bin:/bin", 0), Dir: t.TempDir()}
	ctx := t.Context()
	v, err := c.Version(ctx)
	if err != nil || v != "2.1.292" {
		t.Fatalf("version %q %v", v, err)
	}
	a, err := c.AuthStatus(ctx)
	if err != nil || !a.LoggedIn || a.SubscriptionType != "max" {
		t.Fatalf("auth %+v %v", a, err)
	}
	ag, err := c.Agents(ctx)
	if err != nil || len(ag) != 1 || ag[0].Kind != "interactive" {
		t.Fatalf("agents %+v %v", ag, err)
	}
	ok, err := c.ProbeModel(ctx, "fable", "user")
	if err != nil || !ok.OK || ok.Canonical != "claude-fable-5-1" {
		t.Fatalf("probe ok %+v %v", ok, err)
	}
	bad, err := c.ProbeModel(ctx, "gpt-5", "user")
	if err != nil || bad.OK || bad.Reason == "" {
		t.Fatalf("probe bad %+v %v", bad, err)
	}
}

func TestBuildArgsScreenMCP(t *testing.T) {
	s := spec(t, "x")
	s.MCPConfig, s.AppendSystem = `{"mcpServers":{"screen":{}}}`, "rules"
	args := BuildArgs(s)
	j := strings.Join(args, "\x00")
	if !strings.Contains(j, "--mcp-config\x00{\"mcpServers\":{\"screen\":{}}}\x00--") || !strings.Contains(j, "--append-system-prompt\x00rules\x00--system-prompt-snapshot\x00off") {
		t.Fatalf("args %q", args)
	}
}

func TestStdinPayloadWithImages(t *testing.T) {
	s := Spec{Prompt: "what is this?"}
	if p, _ := StdinPayload(s); p != "what is this?" {
		t.Fatal("plain prompt must be passed as is")
	}
	s.Images = []Image{{MediaType: "image/jpeg", Data: []byte{1, 2, 3}}}
	p, err := StdinPayload(s)
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Type    string
		Message struct {
			Role    string
			Content []map[string]any
		}
	}
	if err := json.Unmarshal([]byte(p), &m); err != nil || m.Type != "user" || len(m.Message.Content) != 2 {
		t.Fatalf("%v %+v", err, m)
	}
	if m.Message.Content[0]["type"] != "image" || m.Message.Content[1]["text"] != "what is this?" {
		t.Fatalf("content order %+v", m.Message.Content)
	}
	if !strings.HasSuffix(p, "\n") {
		t.Fatal("stream-json line must end with a newline")
	}
	args := strings.Join(BuildArgs(Spec{Flags: s.Flags, Images: s.Images, ClaudeSessionID: "u"}), " ")
	if !strings.Contains(args, "--input-format stream-json") {
		t.Fatal("images need --input-format stream-json")
	}
	if strings.Contains(strings.Join(BuildArgs(Spec{ClaudeSessionID: "u"}), " "), "--input-format") {
		t.Fatal("plain prompts must not switch input format")
	}
	if _, err := StdinPayload(Spec{Images: []Image{{MediaType: "image/gif"}}}); err == nil {
		t.Fatal("unsupported type accepted")
	}
	if _, err := StdinPayload(Spec{Images: []Image{{MediaType: "image/png", Data: make([]byte, 8<<20)}}}); err == nil {
		t.Fatal("oversize input accepted")
	}
}
