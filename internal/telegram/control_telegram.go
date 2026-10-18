package telegram

import (
	"context"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/control"
	"github.com/Publicthrone547/clawdyX/internal/observability"
)

// detector records unknown private-chat senders only while the owner has
// armed it from the Mac (to find a new user's numeric ID). It keeps at most
// ten entries in memory and forgets them when disarmed. Nothing is logged
// and nobody gets a reply.
type detector struct {
	mu      sync.Mutex
	until   time.Time
	senders []control.DetectedSender
}

func (d *detector) arm(dur time.Duration) {
	d.mu.Lock()
	d.until, d.senders = time.Now().Add(dur), nil
	d.mu.Unlock()
}

func (d *detector) disarm() {
	d.mu.Lock()
	d.until, d.senders = time.Time{}, nil
	d.mu.Unlock()
}

func (d *detector) observe(u *models.Update) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if time.Now().After(d.until) || u.Message == nil || u.Message.From == nil || u.Message.From.IsBot {
		return
	}
	m := u.Message
	if m.Chat.Type != models.ChatTypePrivate || m.Chat.ID != m.From.ID || len(d.senders) >= 10 {
		return
	}
	for _, s := range d.senders {
		if s.ID == m.From.ID {
			return
		}
	}
	d.senders = append(d.senders, control.DetectedSender{ID: m.From.ID, FirstName: observability.Truncate(m.From.FirstName, 40), Username: m.From.Username, SeenAt: time.Now().Unix()})
}

func (d *detector) result() control.DetectResult {
	d.mu.Lock()
	defer d.mu.Unlock()
	armed := time.Now().Before(d.until)
	if !armed && len(d.senders) > 0 && time.Since(d.until) > 5*time.Minute {
		d.senders = nil
	}
	out := control.DetectResult{Armed: armed, Until: unixTime(d.until), Senders: append([]control.DetectedSender{}, d.senders...)}
	return out
}

func (a *App) registerTelegramControl(srv *control.Server) {
	srv.Handle("telegram.allowlist.get", 5*time.Second, typed(func(context.Context, none) (any, error) {
		out := []control.AllowedUserInfo{}
		for _, u := range a.Allowlist() {
			out = append(out, control.AllowedUserInfo{ID: u.ID, Label: u.Label, Role: u.Role})
		}
		return out, nil
	}))
	srv.Handle("telegram.allowlist.set", 10*time.Second, typed(func(_ context.Context, p control.AllowlistParams) (any, error) {
		var users []config.AllowedUser
		for _, u := range p.Users {
			users = append(users, config.AllowedUser{ID: u.ID, Label: u.Label, Role: u.Role})
		}
		n, err := config.NormalizeAllowlist(0, users)
		if err != nil {
			return nil, control.InvalidParams("%s", err.Error())
		}
		if n[0].ID != a.cfg.Telegram.OwnerUserID {
			return nil, control.Errorf("the owner cannot be removed or replaced here; run clawdyx setup to change the owner")
		}
		if a.ConfigPath != "" {
			if _, err := config.Rewrite(a.ConfigPath, func(c *config.Config) error {
				c.Telegram.OwnerUserID, c.Telegram.AllowedUsers = n[0].ID, n
				return nil
			}); err != nil {
				return nil, control.Errorf("could not save the settings: %v", err)
			}
		}
		if err := a.SetAllowlist(n); err != nil {
			return nil, control.Errorf("%s", err.Error())
		}
		a.st.Audit("allowlist", "set", "", "ok", 0)
		a.detect.disarm()
		go a.registerCommands()
		a.emit("telegram.allowlist", map[string]int{"count": len(n)})
		out := []control.AllowedUserInfo{}
		for _, u := range n {
			out = append(out, control.AllowedUserInfo{ID: u.ID, Label: u.Label, Role: u.Role})
		}
		return out, nil
	}))
	srv.Handle("telegram.detect.start", 5*time.Second, typed(func(_ context.Context, p control.DetectParams) (any, error) {
		sec := p.Seconds
		if sec <= 0 || sec > 300 {
			sec = 120
		}
		a.detect.arm(time.Duration(sec) * time.Second)
		return a.detect.result(), nil
	}))
	srv.Handle("telegram.detect.result", 5*time.Second, typed(func(context.Context, none) (any, error) { return a.detect.result(), nil }))
	srv.Handle("telegram.detect.stop", 5*time.Second, typed(func(context.Context, none) (any, error) {
		a.detect.disarm()
		return a.detect.result(), nil
	}))
	srv.Handle("telegram.token.status", 5*time.Second, typed(func(context.Context, none) (any, error) {
		return control.TokenStatus{Source: a.cfg.TokenSource, Hint: tokenHint(a.cfg.Token), BotUsername: a.BotUsername()}, nil
	}))
	srv.Handle("telegram.token.test", 20*time.Second, typed(func(ctx context.Context, p control.TokenParams) (any, error) {
		return testToken(ctx, p.Token, a.serverURL)
	}))
	srv.Handle("telegram.token.set", 30*time.Second, typed(func(ctx context.Context, p control.TokenParams) (any, error) {
		res, err := testToken(ctx, p.Token, a.serverURL)
		if err != nil {
			return nil, err
		}
		if a.ConfigPath == "" {
			return nil, control.Errorf("the worker does not know its config path")
		}
		if err := config.StoreToken(a.ConfigPath, p.Token); err != nil {
			return nil, control.Errorf("could not store the token: %s", observability.Redact(err.Error()))
		}
		a.st.Audit("token", "replace", "", "ok", 0)
		a.log.Info("bot token replaced from the desktop app; restarting the worker to use it")
		a.requestRestart()
		return res, nil
	}))
}

func testToken(ctx context.Context, tok, serverURL string) (control.TokenTestResult, error) {
	if !config.ValidToken(tok) {
		return control.TokenTestResult{}, control.InvalidParams("that does not look like a Telegram bot token (123456789:AAAA…, 35 characters after the colon)")
	}
	opts := []bot.Option{bot.WithSkipGetMe()}
	if serverURL != "" {
		opts = append(opts, bot.WithServerURL(serverURL))
	}
	b, err := bot.New(tok, opts...)
	if err != nil {
		return control.TokenTestResult{}, control.Errorf("%s", observability.Redact(err.Error()))
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	me, err := b.GetMe(cctx)
	if err != nil {
		return control.TokenTestResult{}, control.Errorf("Telegram rejected the token: %s", observability.Truncate(observability.Redact(err.Error()), 160))
	}
	return control.TokenTestResult{BotUsername: me.Username, BotID: me.ID}, nil
}
