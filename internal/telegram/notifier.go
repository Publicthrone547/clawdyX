package telegram

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot/models"

	"github.com/Publicthrone547/clawdyX/internal/jobs"
	"github.com/Publicthrone547/clawdyX/internal/observability"
	"github.com/Publicthrone547/clawdyX/internal/render"
	"github.com/Publicthrone547/clawdyX/internal/store"
)

type stream struct {
	mu        sync.Mutex
	jobID     int64
	ackID     int
	replyTo   int
	nonce     string
	header    string
	draftOK   bool
	inflight  bool
	lastEdit  time.Time
	typingEnd chan struct{}
	done      bool
}

type streams struct {
	mu sync.Mutex
	m  map[int64]*stream
}

func newStreams() *streams { return &streams{m: map[int64]*stream{}} }

func (s *streams) get(id int64) *stream {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[id]
}

func (s *streams) put(st *stream) {
	s.mu.Lock()
	s.m[st.jobID] = st
	s.mu.Unlock()
}

func (s *streams) drop(id int64) {
	s.mu.Lock()
	delete(s.m, id)
	s.mu.Unlock()
}

func nonce() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func cancelMarkup(st *stream) models.ReplyMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
		{Text: "⛔ Cancel", CallbackData: fmt.Sprintf("c:%d:%s", st.jobID, st.nonce)},
	}}}
}

func noMarkup() models.ReplyMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{}}
}

func header(j *store.Job, s *store.Session) string {
	name := ""
	if s.Name != "" {
		name = " " + Esc(s.Name)
	}
	return fmt.Sprintf("%s%s · %s · %s", s.Label(), name, Esc(modelEffort(j.Model, j.Effort)), s.Mode)
}

// JobQueued implements jobs.Notifier: the acknowledgement message.
func (a *App) JobQueued(j *store.Job, s *store.Session, position int) {
	st := &stream{jobID: j.ID, replyTo: j.TgMessageID, nonce: nonce(), header: header(j, s), draftOK: true}
	a.streams.put(st)
	state := "queued"
	if position > 0 {
		state = fmt.Sprintf("queued (position %d)", position)
	}
	text := fmt.Sprintf("⏳ <b>%s</b> %s · %s", j.Label(), state, st.header)
	id, err := a.send.Send(a.ctx, Out{HTML: text, ReplyTo: j.TgMessageID, Markup: cancelMarkup(st)})
	if err == nil {
		st.mu.Lock()
		st.ackID = id
		st.mu.Unlock()
		_ = a.st.SetJobAck(j.ID, id)
	}
}

// JobStarted implements jobs.Notifier.
func (a *App) JobStarted(j *store.Job, s *store.Session) {
	st := a.streams.get(j.ID)
	if st == nil {
		return
	}
	st.mu.Lock()
	st.header = header(j, s)
	st.typingEnd = make(chan struct{})
	ack, end := st.ackID, st.typingEnd
	st.mu.Unlock()
	_ = a.send.Edit(a.ctx, ack, fmt.Sprintf("▶️ <b>%s</b> running · %s", j.Label(), st.header), cancelMarkup(st))
	go func() {
		a.send.Typing(a.ctx)
		t := time.NewTicker(4 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-end:
				return
			case <-a.ctx.Done():
				return
			case <-t.C:
				a.send.Typing(a.ctx)
			}
		}
	}()
}

// JobProgress implements jobs.Notifier. It never blocks the job: updates
// are skipped while a previous one is in flight or no rate token is free.
func (a *App) JobProgress(j *store.Job, p jobs.Progress) {
	st := a.streams.get(j.ID)
	if st == nil {
		return
	}
	st.mu.Lock()
	if st.inflight || st.done {
		st.mu.Unlock()
		return
	}
	st.inflight = true
	draft := st.draftOK
	st.mu.Unlock()

	go func() {
		defer func() {
			st.mu.Lock()
			st.inflight = false
			st.mu.Unlock()
		}()
		tool := ""
		if p.LastTool != "" {
			tool = "🔧 " + observability.Truncate(p.LastTool, 100)
		}
		if p.Denials > 0 {
			tool += fmt.Sprintf("  🚫 %d denied", p.Denials)
		}
		if draft && p.Text != "" {
			txt := render.Tail(p.Text, 3500)
			if tool != "" {
				txt += "\n\n" + tool
			}
			sent, err := a.send.TryDraft(a.ctx, j.ID, txt)
			if sent && err != nil {
				a.log.Info("draft streaming unavailable; falling back to edits", "err", observability.Truncate(observability.Redact(err.Error()), 160))
				st.mu.Lock()
				st.draftOK = false
				st.mu.Unlock()
			}
			if err == nil {
				return
			}
		}
		st.mu.Lock()
		if time.Since(st.lastEdit) < 3*time.Second {
			st.mu.Unlock()
			return
		}
		st.lastEdit = time.Now()
		ack, hdr := st.ackID, st.header
		st.mu.Unlock()
		body := fmt.Sprintf("▶️ <b>%s</b> running %s · %s", j.Label(), dur(p.Elapsed), hdr)
		if tool != "" {
			body += "\n" + Esc(tool)
		}
		if p.Text != "" {
			body += "\n\n" + Esc(render.Tail(p.Text, 2500))
		}
		_, _ = a.send.TryEdit(a.ctx, ack, body, cancelMarkup(st))
	}()
}

var stateIcon = map[string]string{
	store.JobCompleted: "✅", store.JobFailed: "❌", store.JobTimeout: "⏱", store.JobCancelled: "⛔", store.JobInterrupted: "♻️",
}

// JobFinished implements jobs.Notifier.
func (a *App) JobFinished(j *store.Job, s *store.Session, o jobs.Outcome) {
	st := a.streams.get(j.ID)
	a.streams.drop(j.ID)
	replyTo := j.TgMessageID
	if st != nil {
		st.mu.Lock()
		st.done = true
		if st.typingEnd != nil {
			close(st.typingEnd)
		}
		ack := st.ackID
		st.mu.Unlock()
		status := fmt.Sprintf("%s <b>%s</b> %s", stateIcon[o.State], j.Label(), o.State)
		if o.Duration > 0 {
			status += " · " + dur(o.Duration)
		}
		if s != nil {
			status += " · " + header(j, s)
		}
		_ = a.send.Edit(a.ctx, ack, status, noMarkup())
	}

	var parts []string
	switch o.State {
	case store.JobCompleted:
		if strings.TrimSpace(o.Text) == "" {
			parts = append(parts, "<i>(Claude returned no text)</i>")
		} else {
			parts = append(parts, render.Chunk(a.hidePaths(o.Text, s), render.Limit)...)
		}
	case store.JobCancelled:
		msg := fmt.Sprintf("⛔ %s cancelled.", j.Label())
		if s != nil && s.Started {
			msg += " The session is still resumable."
		}
		if strings.TrimSpace(o.Text) != "" {
			msg += fmt.Sprintf(" Partial output kept in /logs %s.", j.Label())
		}
		parts = append(parts, msg)
	default:
		msg := fmt.Sprintf("%s %s %s", stateIcon[o.State], j.Label(), o.State)
		if o.ErrorMessage != "" {
			msg += ": " + Esc(o.ErrorMessage)
		}
		if o.SessionMissing {
			msg += "\nThis session cannot be resumed any more. Start a fresh one with /new."
		}
		if o.State == store.JobTimeout && s != nil && s.Started {
			msg += "\nThe session remains resumable."
		}
		parts = append(parts, msg)
		if strings.TrimSpace(o.Text) != "" && o.State != store.JobInterrupted {
			parts = append(parts, render.Chunk(a.hidePaths(o.Text, s), render.Limit)...)
		}
	}
	if o.Truncated {
		parts = append(parts, fmt.Sprintf("<i>… output truncated at %d KB.</i>", a.cfg.Claude.MaxOutputBytes/1024))
	}
	if len(o.Denials) > 0 {
		var sb strings.Builder
		fmt.Fprintf(&sb, "🚫 <b>%d tool call(s) denied</b> by the %s tier:\n", len(o.Denials), s.Mode)
		for i, d := range o.Denials {
			if i == 8 {
				fmt.Fprintf(&sb, "… and %d more (/logs %s)\n", len(o.Denials)-8, j.Label())
				break
			}
			sb.WriteString("• <code>" + Esc(d) + "</code>\n")
		}
		if s.Mode == "observe" {
			sb.WriteString("Use /mode assisted to allow edits and the workspace command allowlist.\n")
		}
		for _, d := range o.Denials {
			if strings.HasPrefix(d, "Bash open ") {
				sb.WriteString("Claude cannot open apps or websites. To open a website on the Mac yourself, send /open ADDRESS (for example /open youtube.com).")
				break
			}
		}
		parts = append(parts, sb.String())
	}
	for _, w := range o.Warnings {
		parts = append(parts, "⚠️ "+Esc(w))
	}
	if exp, ok := a.expectedModel(j.Model); ok && o.ResolvedModel != "" && exp != o.ResolvedModel {
		parts = append(parts, fmt.Sprintf("⚠️ Expected %s but Claude reported %s.", Esc(exp), Esc(o.ResolvedModel)))
	}
	if o.State == store.JobCompleted || o.State == store.JobFailed || o.State == store.JobTimeout {
		model := o.ResolvedModel
		if model == "" {
			model = j.Model
		}
		eff := o.Effort
		if eff == "" {
			eff = j.Effort
		}
		footer := fmt.Sprintf("<i>— %s · %s · %d turn(s) · in %s / out %s · cache r %s · %s</i>", j.Label(), dur(o.Duration), o.NumTurns,
			tokens(o.Usage.InputTokens), tokens(o.Usage.OutputTokens), tokens(o.Usage.CacheReadInputTokens), Esc(modelEffort(model, eff)))
		if last := len(parts) - 1; last >= 0 && len(parts[last])+len(footer)+1 <= render.Limit {
			parts[last] += "\n" + footer
		} else {
			parts = append(parts, footer)
		}
	}
	for i, p := range parts {
		rt := 0
		if i == 0 {
			rt = replyTo
		}
		a.outbox.Push(Out{HTML: p, ReplyTo: rt})
	}
}

// hidePaths replaces absolute paths outside the session's workspace.
func (a *App) hidePaths(text string, s *store.Session) string {
	if s == nil {
		return text
	}
	return pathRe.ReplaceAllStringFunc(text, func(p string) string {
		if strings.HasPrefix(p, s.WorkspacePath) {
			return p
		}
		for _, w := range a.cfg.Workspaces {
			if strings.HasPrefix(p, w.Canonical) {
				return p
			}
		}
		if strings.HasPrefix(p, "/Users/") || strings.HasPrefix(p, "/private/") || strings.HasPrefix(p, "/var/") {
			return "<outside workspace>"
		}
		return p
	})
}

func (a *App) expectedModel(name string) (string, bool) {
	v := a.CLIVersion()
	if v == "" || name == "" {
		return "", false
	}
	return a.st.Capability("model:"+name, v, 7*24*time.Hour)
}
