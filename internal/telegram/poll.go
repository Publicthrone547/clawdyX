package telegram

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/control"
	"github.com/Publicthrone547/clawdyX/internal/observability"
)

// keyBotEnabled persists whether polling should run (the GUI's Start/Stop).
const keyBotEnabled = "bot_enabled"

// poller owns the Telegram polling goroutine. The worker process keeps
// running while polling is stopped, so the desktop app stays connected and
// jobs that are already running can finish and report.
type poller struct {
	mu      sync.Mutex // serialises Start/Stop
	stateMu sync.Mutex // guards the fields below for cheap reads
	state   string
	since   time.Time
	lastErr string
	cancel  context.CancelFunc
	done    chan struct{}
}

func (p *poller) set(state, lastErr string) {
	p.stateMu.Lock()
	p.state, p.since, p.lastErr = state, time.Now(), lastErr
	p.stateMu.Unlock()
}

// BotState reports the polling state.
func (a *App) BotState() control.BotState {
	a.poll.stateMu.Lock()
	st, since, le := a.poll.state, a.poll.since, a.poll.lastErr
	a.poll.stateMu.Unlock()
	if st == "" {
		st = control.BotStopped
	}
	if st == control.BotPolling && a.jobs.Paused() {
		st = control.BotPaused
	}
	return control.BotState{State: st, Since: unixTime(since), Enabled: a.st.Setting(keyBotEnabled, "true") != "false",
		PollStarts: a.counters.PollStarts.Load(), LastError: le}
}

func (a *App) emitBot() { a.emit("bot.state", a.BotState()) }

// StartPolling starts the Telegram poller. It is idempotent: concurrent or
// repeated calls never create a second poller. With validate, the token is
// checked with getMe first and an error is returned when it fails.
func (a *App) StartPolling(validate bool) (control.BotState, error) {
	a.poll.mu.Lock()
	defer a.poll.mu.Unlock()
	if a.ctx == nil || a.ctx.Err() != nil {
		return a.BotState(), errors.New("the worker is shutting down")
	}
	if a.poll.cancel != nil {
		return a.BotState(), nil
	}
	a.poll.set(control.BotStarting, "")
	a.emitBot()
	if validate {
		ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
		me, err := a.b.GetMe(ctx)
		cancel()
		if err != nil {
			msg := "Telegram rejected the bot token or is unreachable: " + observability.Truncate(observability.Redact(err.Error()), 160)
			a.poll.set(control.BotStopped, msg)
			a.setErr(msg)
			a.emitBot()
			return a.BotState(), errors.New(msg)
		}
		a.setBotUsername(me.Username)
	}
	pctx, cancel := context.WithCancel(a.ctx)
	done := make(chan struct{})
	a.poll.cancel, a.poll.done = cancel, done
	a.counters.PollStarts.Add(1)
	go func() {
		defer close(done)
		a.b.Start(pctx)
	}()
	_ = a.st.SetSetting(keyBotEnabled, "true")
	a.poll.set(control.BotPolling, "")
	a.log.Info("telegram polling started")
	a.emitBot()
	return a.BotState(), nil
}

// StopPolling stops the poller and waits for it to exit. persist records the
// stopped state so it survives a worker restart (false on shutdown).
func (a *App) StopPolling(persist bool) control.BotState {
	a.poll.mu.Lock()
	defer a.poll.mu.Unlock()
	if persist {
		_ = a.st.SetSetting(keyBotEnabled, "false")
	}
	if a.poll.cancel == nil {
		a.poll.set(control.BotStopped, "")
		a.emitBot()
		return a.BotState()
	}
	a.poll.set(control.BotStopping, "")
	a.emitBot()
	a.poll.cancel()
	select {
	case <-a.poll.done:
	case <-time.After(10 * time.Second):
		a.log.Warn("telegram poller did not stop within 10 s")
	}
	a.poll.cancel, a.poll.done = nil, nil
	a.poll.set(control.BotStopped, "")
	a.log.Info("telegram polling stopped")
	a.emitBot()
	return a.BotState()
}

func unixTime(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}
