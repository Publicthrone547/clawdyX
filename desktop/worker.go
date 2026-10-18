package main

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/control"
)

// ErrWorkerDown is shown when the background worker is not reachable.
var ErrWorkerDown = errors.New("The clawdyX worker is not running. Start it from Overview.")

// WorkerEvent is a worker event forwarded to the frontend ("worker:event").
type WorkerEvent struct {
	Type string          `json:"type"`
	Seq  uint64          `json:"seq"`
	TS   int64           `json:"ts"`
	Data json.RawMessage `json:"data"`
}

// Connection is the state of the link to the worker ("worker:connection").
type Connection struct {
	Connected  bool   `json:"connected"`
	Configured bool   `json:"configured"`
	Error      string `json:"error,omitempty"`
}

// WorkerService connects to the worker's control socket, keeps the
// connection alive, forwards events, and exposes typed calls to the UI.
// It never runs shell commands or touches files for the UI.
type WorkerService struct {
	app *application.App

	mu   sync.Mutex
	c    *control.Client
	conn Connection
}

func (w *WorkerService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	w.app = application.Get()
	go w.loop(ctx)
	return nil
}

func socketPath() (string, error) {
	cfg, err := loadConfig()
	if err != nil {
		return "", err
	}
	return cfg.ControlSocket(), nil
}

func loadConfig() (*config.Config, error) {
	return loadConfigFile(config.DefaultPath())
}

func (w *WorkerService) setConn(c Connection) {
	w.mu.Lock()
	changed := c != w.conn
	w.conn = c
	w.mu.Unlock()
	if changed && w.app != nil {
		w.app.Event.Emit("worker:connection", c)
	}
}

func (w *WorkerService) loop(ctx context.Context) {
	for ctx.Err() == nil {
		path, err := socketPath()
		if err != nil {
			w.setConn(Connection{Configured: !isNotExist(err), Error: errText(err)})
			sleep(ctx, 2*time.Second)
			continue
		}
		dctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		c, err := control.Dial(dctx, path)
		cancel()
		if err != nil {
			w.setConn(Connection{Configured: true})
			sleep(ctx, 2*time.Second)
			continue
		}
		if err := c.Subscribe(ctx); err != nil {
			c.Close()
			sleep(ctx, 2*time.Second)
			continue
		}
		w.mu.Lock()
		w.c = c
		w.mu.Unlock()
		w.setConn(Connection{Connected: true, Configured: true})
		for ev := range c.Events() {
			if w.app != nil {
				w.app.Event.Emit("worker:event", WorkerEvent{Type: ev.Type, Seq: ev.Seq, TS: ev.TS, Data: ev.Data})
			}
		}
		w.mu.Lock()
		w.c = nil
		w.mu.Unlock()
		w.setConn(Connection{Configured: true})
		sleep(ctx, 500*time.Millisecond)
	}
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func (w *WorkerService) call(method string, params, out any, timeout time.Duration) error {
	w.mu.Lock()
	c := w.c
	w.mu.Unlock()
	if c == nil {
		return ErrWorkerDown
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	err := c.Call(ctx, method, params, out)
	var e *control.Error
	if errors.As(err, &e) {
		return errors.New(e.Message)
	}
	if errors.Is(err, control.ErrClosed) {
		return ErrWorkerDown
	}
	return err
}

// Connection reports whether the worker is reachable.
func (w *WorkerService) Connection() Connection {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn
}

const short, medium, long = 10 * time.Second, 40 * time.Second, 160 * time.Second

func (w *WorkerService) Status() (control.Status, error) {
	var s control.Status
	return s, w.call("status.get", nil, &s, short)
}
func (w *WorkerService) BotStart() (control.BotState, error) {
	var s control.BotState
	return s, w.call("bot.start", nil, &s, medium)
}
func (w *WorkerService) BotStop(cancelJobs bool) (control.BotStopResult, error) {
	var r control.BotStopResult
	return r, w.call("bot.stop", control.BotStopParams{CancelJobs: cancelJobs}, &r, long)
}
func (w *WorkerService) BotPause() (control.BotState, error) {
	var s control.BotState
	return s, w.call("bot.pause", nil, &s, short)
}
func (w *WorkerService) BotResume() (control.BotState, error) {
	var s control.BotState
	return s, w.call("bot.resume", nil, &s, short)
}
func (w *WorkerService) Sessions(all bool) ([]control.SessionInfo, error) {
	var out []control.SessionInfo
	return out, w.call("sessions.list", control.SessionsListParams{All: all}, &out, short)
}
func (w *WorkerService) ForgottenSessions() ([]control.SessionInfo, error) {
	var out []control.SessionInfo
	return out, w.call("sessions.list", control.SessionsListParams{Forgotten: true}, &out, short)
}
func (w *WorkerService) SessionCreate(name, workspace string) (control.SessionResult, error) {
	var r control.SessionResult
	return r, w.call("sessions.create", control.SessionCreateParams{Name: name, Workspace: workspace}, &r, short)
}
func (w *WorkerService) SessionSwitch(ref string) (control.SessionResult, error) {
	var r control.SessionResult
	return r, w.call("sessions.switch", control.SessionRefParams{Ref: ref}, &r, short)
}
func (w *WorkerService) SessionArchive(ref string) (control.SessionResult, error) {
	var r control.SessionResult
	return r, w.call("sessions.archive", control.SessionRefParams{Ref: ref}, &r, short)
}
func (w *WorkerService) SessionRestore(ref string) (control.SessionResult, error) {
	var r control.SessionResult
	return r, w.call("sessions.restore", control.SessionRefParams{Ref: ref}, &r, short)
}
func (w *WorkerService) SessionNewForget(name string, deleteHistory bool) (control.SessionResult, error) {
	var r control.SessionResult
	return r, w.call("sessions.newForget", control.SessionNewForgetParams{Name: name, DeleteHistory: deleteHistory}, &r, medium)
}
func (w *WorkerService) SessionFresh() (control.SessionResult, error) {
	var r control.SessionResult
	return r, w.call("sessions.fresh", nil, &r, short)
}
func (w *WorkerService) SessionForget(ref string, deleteHistory bool) (control.SessionResult, error) {
	var r control.SessionResult
	return r, w.call("sessions.forget", control.SessionForgetParams{Ref: ref, DeleteHistory: deleteHistory}, &r, medium)
}
func (w *WorkerService) SessionRename(ref, name string) (control.SessionResult, error) {
	var r control.SessionResult
	return r, w.call("sessions.rename", control.SessionRenameParams{Ref: ref, Name: name}, &r, short)
}
func (w *WorkerService) SessionHistory(ref string) (control.HistoryInfo, error) {
	var h control.HistoryInfo
	return h, w.call("sessions.history", control.SessionRefParams{Ref: ref}, &h, short)
}
func (w *WorkerService) Jobs(limit int) ([]control.JobInfo, error) {
	var out []control.JobInfo
	return out, w.call("jobs.list", control.JobsListParams{Limit: limit}, &out, short)
}
func (w *WorkerService) JobCancel(id int64) (control.CancelResult, error) {
	var r control.CancelResult
	return r, w.call("jobs.cancel", control.JobRefParams{ID: id}, &r, medium)
}
func (w *WorkerService) JobEvents(id int64, limit int) ([]control.JobEvent, error) {
	var out []control.JobEvent
	return out, w.call("jobs.events", control.JobEventsParams{ID: id, Limit: limit}, &out, short)
}
func (w *WorkerService) Submit(session, text string) (control.SubmitResult, error) {
	var r control.SubmitResult
	return r, w.call("jobs.submit", control.SubmitParams{Session: session, Text: text}, &r, short)
}
func (w *WorkerService) Processes() (control.ProcessesInfo, error) {
	var p control.ProcessesInfo
	return p, w.call("processes.list", nil, &p, medium)
}
func (w *WorkerService) Models() (control.ModelsInfo, error) {
	var m control.ModelsInfo
	return m, w.call("claude.models", nil, &m, short)
}
func (w *WorkerService) SetModel(session, model string, asDefault bool) (control.SettingResult, error) {
	var r control.SettingResult
	return r, w.call("model.set", control.SetModelParams{Session: session, Model: model, Default: asDefault}, &r, long)
}
func (w *WorkerService) SetEffort(session, effort string, asDefault bool) (control.SettingResult, error) {
	var r control.SettingResult
	return r, w.call("effort.set", control.SetEffortParams{Session: session, Effort: effort, Default: asDefault}, &r, short)
}
func (w *WorkerService) SetMode(session, mode string) (control.SettingResult, error) {
	var r control.SettingResult
	return r, w.call("mode.set", control.SetModeParams{Session: session, Mode: mode}, &r, short)
}
func (w *WorkerService) LogTail(lines int, level string) ([]control.LogLine, error) {
	var out []control.LogLine
	return out, w.call("log.tail", control.LogTailParams{Lines: lines, Level: level}, &out, short)
}
func (w *WorkerService) Diagnostics() ([]control.DiagnosticCheck, error) {
	var out []control.DiagnosticCheck
	return out, w.call("diagnostics.run", nil, &out, long)
}
func (w *WorkerService) Screens() (control.ScreensInfo, error) {
	var s control.ScreensInfo
	return s, w.call("screens.list", nil, &s, medium)
}
func (w *WorkerService) Capture(display string) (control.CaptureResult, error) {
	var r control.CaptureResult
	return r, w.call("screens.capture", control.CaptureParams{Display: display}, &r, 100*time.Second)
}
func (w *WorkerService) CaptureFull(id string) (control.FullImage, error) {
	var f control.FullImage
	return f, w.call("screens.full", control.CaptureRefParams{ID: id}, &f, medium)
}
func (w *WorkerService) CaptureDiscard(id string) error {
	return w.call("screens.discard", control.CaptureRefParams{ID: id}, nil, short)
}
func (w *WorkerService) CaptureUse(id, session, text string) (control.SubmitResult, error) {
	var r control.SubmitResult
	return r, w.call("screens.use", control.ScreensUseParams{ID: id, Session: session, Text: text}, &r, short)
}
func (w *WorkerService) SetScreenEnabled(on bool) (control.ScreenStatus, error) {
	var s control.ScreenStatus
	return s, w.call("screens.enabled.set", control.EnabledParams{Enabled: on}, &s, short)
}
func (w *WorkerService) Allowlist() ([]control.AllowedUserInfo, error) {
	var out []control.AllowedUserInfo
	return out, w.call("telegram.allowlist.get", nil, &out, short)
}
func (w *WorkerService) SetAllowlist(users []control.AllowedUserInfo) ([]control.AllowedUserInfo, error) {
	var out []control.AllowedUserInfo
	return out, w.call("telegram.allowlist.set", control.AllowlistParams{Users: users}, &out, short)
}
func (w *WorkerService) DetectStart(seconds int) (control.DetectResult, error) {
	var r control.DetectResult
	return r, w.call("telegram.detect.start", control.DetectParams{Seconds: seconds}, &r, short)
}
func (w *WorkerService) DetectResult() (control.DetectResult, error) {
	var r control.DetectResult
	return r, w.call("telegram.detect.result", nil, &r, short)
}
func (w *WorkerService) DetectStop() (control.DetectResult, error) {
	var r control.DetectResult
	return r, w.call("telegram.detect.stop", nil, &r, short)
}
func (w *WorkerService) TokenStatus() (control.TokenStatus, error) {
	var s control.TokenStatus
	return s, w.call("telegram.token.status", nil, &s, short)
}
func (w *WorkerService) TokenTest(token string) (control.TokenTestResult, error) {
	var r control.TokenTestResult
	return r, w.call("telegram.token.test", control.TokenParams{Token: token}, &r, medium)
}
func (w *WorkerService) TokenSet(token string) (control.TokenTestResult, error) {
	var r control.TokenTestResult
	return r, w.call("telegram.token.set", control.TokenParams{Token: token}, &r, medium)
}
func (w *WorkerService) Settings() (control.SettingsInfo, error) {
	var s control.SettingsInfo
	return s, w.call("settings.get", nil, &s, short)
}
func (w *WorkerService) SetSettings(p control.SettingsPatch) (control.SettingsInfo, error) {
	var s control.SettingsInfo
	return s, w.call("settings.set", p, &s, short)
}
func (w *WorkerService) SetDefaultWorkspace(name string) (control.SettingsInfo, error) {
	var s control.SettingsInfo
	return s, w.call("workspace.setDefault", control.WorkspaceDefaultParams{Name: name}, &s, short)
}
func (w *WorkerService) RestartWorker() error {
	return w.call("worker.restart", nil, nil, short)
}
func (w *WorkerService) ResetSettings(section string) (control.SettingsInfo, error) {
	var s control.SettingsInfo
	return s, w.call("settings.reset", control.SettingsResetParams{Section: section}, &s, short)
}
