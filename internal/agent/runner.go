package agent

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/Publicthrone547/clawdyX/internal/policy"
)

// Spec describes one claude invocation.
type Spec struct {
	Binary          string
	Dir             string // canonical workspace directory (cwd of the child)
	Prompt          string // written to stdin
	ClaudeSessionID string // UUID
	Resume          bool   // true: --resume, false: --session-id (new session)
	Model           string // empty: do not pass --model
	Effort          string // empty or "auto": do not pass --effort
	Flags           policy.Flags
	SettingSources  string
	StrictMCP       bool
	MaxTurns        int
	MaxBudgetUSD    float64
	Env             []string // full child environment
	MCPConfig       string   // JSON for --mcp-config (only servers the bot provides)
	AppendSystem    string   // text for --append-system-prompt
}

// BuildArgs returns the argv (without the binary) for a job. The prompt is
// never part of argv.
func BuildArgs(s Spec) []string {
	a := []string{"-p", "--output-format", "stream-json", "--verbose", "--include-partial-messages",
		"--permission-mode", s.Flags.PermissionMode, "--permission-prompts", "none"}
	if s.Resume {
		a = append(a, "--resume", s.ClaudeSessionID)
	} else {
		a = append(a, "--session-id", s.ClaudeSessionID)
	}
	if s.Model != "" {
		a = append(a, "--model", s.Model)
	}
	if s.Effort != "" && s.Effort != "auto" {
		a = append(a, "--effort", s.Effort)
	}
	if s.SettingSources != "" {
		a = append(a, "--setting-sources", s.SettingSources)
	}
	if s.StrictMCP {
		a = append(a, "--strict-mcp-config")
	}
	if s.Flags.SettingsJSON != "" {
		a = append(a, "--settings", s.Flags.SettingsJSON)
	}
	if s.MaxTurns > 0 {
		a = append(a, "--max-turns", strconv.Itoa(s.MaxTurns))
	}
	if s.MaxBudgetUSD > 0 {
		a = append(a, "--max-budget-usd", strconv.FormatFloat(s.MaxBudgetUSD, 'f', 2, 64))
	}
	if s.AppendSystem != "" {
		// Claude Code records the system prompt on a conversation's first
		// request and reuses it on resume. A session that started in another
		// tier would never see this text, so rebuild the prompt every request.
		a = append(a, "--append-system-prompt", s.AppendSystem, "--system-prompt-snapshot", "off")
	}
	if s.MCPConfig != "" {
		a = append(a, "--mcp-config", s.MCPConfig) // variadic; the next flag terminates it
	}
	// Variadic flags go last; each rule is its own argv element. No positional
	// prompt follows, so nothing can be swallowed by the variadic parser.
	if len(s.Flags.Allowed) > 0 {
		a = append(a, "--allowedTools")
		a = append(a, s.Flags.Allowed...)
	}
	if len(s.Flags.Disallowed) > 0 {
		a = append(a, "--disallowedTools")
		a = append(a, s.Flags.Disallowed...)
	}
	return a
}

// ChildEnv builds the allowlisted environment for claude children. Secrets
// (bot token, API keys) and nested-session markers are never forwarded.
func ChildEnv(path string, bgWaitMS int) []string {
	env := []string{"PATH=" + path, "SHELL=/bin/zsh", "TERM=dumb", "LANG=en_US.UTF-8", "DISABLE_AUTOUPDATER=1"}
	for _, k := range []string{"HOME", "USER", "LOGNAME", "TMPDIR"} {
		if v := os.Getenv(k); v != "" {
			env = append(env, k+"="+v)
		}
	}
	if bgWaitMS > 0 {
		env = append(env, "CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS="+strconv.Itoa(bgWaitMS))
	}
	return env
}

// ringBuffer keeps the last n bytes written.
type ringBuffer struct {
	mu  sync.Mutex
	buf []byte
	n   int
}

func (r *ringBuffer) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, p...)
	if len(r.buf) > r.n {
		r.buf = r.buf[len(r.buf)-r.n:]
	}
	return len(p), nil
}

func (r *ringBuffer) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(r.buf)
}

// Process is a running claude child in its own process group.
type Process struct {
	cmd       *exec.Cmd
	events    chan Event
	stderr    *ringBuffer
	PID       int
	StartUnix int64
	done      chan struct{}
	exitCode  int
	waitErr   error
	parseErrs int
	overflow  bool
}

// Events returns the event channel; it is closed when stdout ends.
func (p *Process) Events() <-chan Event { return p.events }

// Stderr returns the last 64 KiB of stderr.
func (p *Process) Stderr() string { return p.stderr.String() }

// Done is closed when the process has exited and output is drained.
func (p *Process) Done() <-chan struct{} { return p.done }

// ExitCode returns the exit code after Done (128+signal if signalled).
func (p *Process) ExitCode() int { return p.exitCode }

// ParseErrors reports how many stdout lines could not be parsed.
func (p *Process) ParseErrors() int { return p.parseErrs }

// Start launches the child. The caller must drain Events until it is closed.
func Start(s Spec) (*Process, error) {
	if s.Dir == "" || s.Binary == "" || s.ClaudeSessionID == "" || s.Flags.PermissionMode == "" {
		return nil, errors.New("agent: incomplete spec")
	}
	cmd := exec.Command(s.Binary, BuildArgs(s)...)
	cmd.Dir = s.Dir
	cmd.Env = s.Env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdin = strings.NewReader(s.Prompt)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	p := &Process{cmd: cmd, events: make(chan Event, 256), stderr: &ringBuffer{n: 64 << 10}, done: make(chan struct{})}
	cmd.Stderr = p.stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("agent: start claude: %w", err)
	}
	p.PID = cmd.Process.Pid
	p.StartUnix = ProcessStartUnix(p.PID)

	go func() {
		p.readLoop(stdout)
		err := cmd.Wait()
		p.waitErr = err
		p.exitCode = exitCode(cmd, err)
		close(p.done)
	}()
	return p, nil
}

func (p *Process) readLoop(r io.Reader) {
	defer close(p.events)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 256<<10), 32<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		ev, err := Parse(line)
		if err != nil {
			p.parseErrs++
			continue
		}
		p.events <- ev
	}
	if sc.Err() != nil {
		p.overflow = true
		_, _ = io.Copy(io.Discard, r)
	}
}

func exitCode(cmd *exec.Cmd, err error) int {
	if cmd.ProcessState == nil {
		return -1
	}
	if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return cmd.ProcessState.ExitCode()
}

// SignalGroup sends sig to the child's whole process group.
func (p *Process) SignalGroup(sig syscall.Signal) error {
	select {
	case <-p.done:
		return nil
	default:
	}
	return syscall.Kill(-p.PID, sig)
}

// Stop runs the cancellation ladder: SIGINT (ends the turn and records it),
// then SIGTERM, then SIGKILL, each to the whole process group. It returns
// when the child has exited.
func (p *Process) Stop(intGrace, termGrace time.Duration) {
	steps := []struct {
		sig   syscall.Signal
		grace time.Duration
	}{{syscall.SIGINT, intGrace}, {syscall.SIGTERM, termGrace}, {syscall.SIGKILL, 5 * time.Second}}
	for _, st := range steps {
		if err := p.SignalGroup(st.sig); err != nil && !errors.Is(err, syscall.ESRCH) {
			_ = p.cmd.Process.Signal(st.sig)
		}
		select {
		case <-p.done:
			return
		case <-time.After(st.grace):
		}
	}
	<-p.done
}

// ProcessStartUnix returns a process's start time (seconds) via sysctl, or 0.
func ProcessStartUnix(pid int) int64 {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || kp == nil || kp.Proc.P_pid != int32(pid) {
		return 0
	}
	return int64(kp.Proc.P_starttime.Sec)
}

// ProcessComm returns the short command name of a pid, or "".
func ProcessComm(pid int) string {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || kp == nil || kp.Proc.P_pid != int32(pid) {
		return ""
	}
	b := kp.Proc.P_comm[:]
	n := 0
	for n < len(b) && b[n] != 0 {
		n++
	}
	return string(b[:n])
}

// OwnedAlive reports whether pid is still the same claude process the bot
// started (guards against PID reuse).
func OwnedAlive(pid int, startUnix int64) bool {
	if pid <= 0 || startUnix == 0 {
		return false
	}
	return ProcessStartUnix(pid) == startUnix && ProcessComm(pid) == "claude"
}

// RunCapture runs a short read-only CLI command with a timeout and returns
// stdout and stderr (bounded).
func RunCapture(ctx context.Context, timeout time.Duration, dir string, env []string, bin string, args ...string) (string, string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	out, errb := &ringBuffer{n: 4 << 20}, &ringBuffer{n: 64 << 10}
	cmd.Stdout, cmd.Stderr = out, errb
	err := cmd.Run()
	code := exitCode(cmd, err)
	if ctx.Err() != nil {
		return out.String(), errb.String(), code, fmt.Errorf("timed out after %s", timeout)
	}
	if _, ok := err.(*exec.ExitError); ok {
		err = nil
	}
	return out.String(), errb.String(), code, err
}
