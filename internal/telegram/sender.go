package telegram

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/Publicthrone547/clawdyX/internal/observability"
	"github.com/Publicthrone547/clawdyX/internal/render"
)

// Sender wraps the Bot API with a per-chat token bucket (1 msg/s, burst 3),
// 429 handling and a plain-text fallback when HTML is rejected.
type Sender struct {
	b        *bot.Bot
	chatID   int64
	log      *slog.Logger
	counters *observability.Counters

	mu     sync.Mutex
	tokens float64
	last   time.Time
}

// chat returns c, or the owner's chat when c is 0.
func (s *Sender) chat(c int64) int64 {
	if c != 0 {
		return c
	}
	return s.chatID
}

// NewSender returns a sender whose default chat is the owner's.
func NewSender(b *bot.Bot, chatID int64, log *slog.Logger, c *observability.Counters) *Sender {
	return &Sender{b: b, chatID: chatID, log: log, counters: c, tokens: 3, last: time.Now()}
}

func (s *Sender) refill() {
	now := time.Now()
	s.tokens += now.Sub(s.last).Seconds()
	if s.tokens > 3 {
		s.tokens = 3
	}
	s.last = now
}

func (s *Sender) wait(ctx context.Context) error {
	for {
		s.mu.Lock()
		s.refill()
		if s.tokens >= 1 {
			s.tokens--
			s.mu.Unlock()
			return nil
		}
		need := time.Duration((1 - s.tokens) * float64(time.Second))
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(need):
		}
	}
}

func (s *Sender) try() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refill()
	if s.tokens >= 1 {
		s.tokens--
		return true
	}
	return false
}

// Out is an outgoing message.
type Out struct {
	ChatID  int64 // 0 = the owner's chat
	HTML    string
	Plain   string // used when Telegram rejects the HTML; defaults to StripHTML(HTML)
	ReplyTo int
	Markup  models.ReplyMarkup
}

func isParseErr(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "can't parse entities")
}

func isNotModified(err error) bool {
	return err != nil && strings.Contains(err.Error(), "message is not modified")
}

// IsAPIError reports whether Telegram answered with an error (as opposed to
// a network failure, which may succeed later).
func IsAPIError(err error) bool {
	for _, e := range []error{bot.ErrorBadRequest, bot.ErrorForbidden, bot.ErrorUnauthorized, bot.ErrorNotFound, bot.ErrorConflict} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// call runs fn with token-bucket pacing and up to 3 retries on 429.
func (s *Sender) call(ctx context.Context, fn func() error) error { return s.callPaced(ctx, fn, false) }

// callPaced is call; haveToken means the caller already took the first token.
func (s *Sender) callPaced(ctx context.Context, fn func() error, haveToken bool) error {
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 || !haveToken {
			if err = s.wait(ctx); err != nil {
				return err
			}
		}
		err = fn()
		var tm *bot.TooManyRequestsError
		if errors.As(err, &tm) {
			d := time.Duration(tm.RetryAfter) * time.Second
			if d <= 0 || d > 60*time.Second {
				d = 5 * time.Second
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
			}
			continue
		}
		break
	}
	if err != nil && !isNotModified(err) {
		s.counters.SendErrors.Add(1)
		s.log.Warn("telegram call failed", "err", observability.Redact(err.Error()))
	}
	return err
}

// Send sends a message and returns its ID.
func (s *Sender) Send(ctx context.Context, o Out) (int, error) {
	chat := o.ChatID
	if chat == 0 {
		chat = s.chatID
	}
	params := &bot.SendMessageParams{ChatID: chat, Text: o.HTML, ParseMode: models.ParseModeHTML, ReplyMarkup: o.Markup,
		LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: bot.True()}}
	if o.ReplyTo != 0 {
		params.ReplyParameters = &models.ReplyParameters{MessageID: o.ReplyTo, AllowSendingWithoutReply: true}
	}
	var msg *models.Message
	err := s.call(ctx, func() error {
		var e error
		msg, e = s.b.SendMessage(ctx, params)
		return e
	})
	if isParseErr(err) {
		plain := o.Plain
		if plain == "" {
			plain = render.StripHTML(o.HTML)
		}
		params.Text, params.ParseMode = plain, ""
		err = s.call(ctx, func() error {
			var e error
			msg, e = s.b.SendMessage(ctx, params)
			return e
		})
	}
	if err != nil {
		return 0, err
	}
	return msg.ID, nil
}

// Edit replaces a message's text and markup. "not modified" is not an error.
func (s *Sender) Edit(ctx context.Context, chat int64, msgID int, html string, markup models.ReplyMarkup) error {
	if msgID == 0 {
		return nil
	}
	params := &bot.EditMessageTextParams{ChatID: s.chat(chat), MessageID: msgID, Text: html, ParseMode: models.ParseModeHTML, ReplyMarkup: markup,
		LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: bot.True()}}
	err := s.call(ctx, func() error { _, e := s.b.EditMessageText(ctx, params); return e })
	if isParseErr(err) {
		params.Text, params.ParseMode = render.StripHTML(html), ""
		err = s.call(ctx, func() error { _, e := s.b.EditMessageText(ctx, params); return e })
	}
	if isNotModified(err) {
		return nil
	}
	return err
}

// TryEdit edits only if a rate token is available right now.
func (s *Sender) TryEdit(ctx context.Context, chat int64, msgID int, html string, markup models.ReplyMarkup) (bool, error) {
	if msgID == 0 || !s.try() {
		return false, nil
	}
	params := &bot.EditMessageTextParams{ChatID: s.chat(chat), MessageID: msgID, Text: html, ParseMode: models.ParseModeHTML, ReplyMarkup: markup,
		LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: bot.True()}}
	err := s.callPaced(ctx, func() error { _, e := s.b.EditMessageText(ctx, params); return e }, true)
	if isNotModified(err) {
		err = nil
	}
	return true, err
}

// TryDraft streams a draft preview (plain text) if a token is available.
func (s *Sender) TryDraft(ctx context.Context, chat int64, draftID int64, text string) (bool, error) {
	if !s.try() {
		return false, nil
	}
	_, err := s.b.SendMessageDraft(ctx, &bot.SendMessageDraftParams{ChatID: s.chat(chat), DraftID: strconv.FormatInt(draftID, 10), Text: text, CanStop: true})
	return true, err
}

// Typing shows the typing indicator (lasts about 5 s). It bypasses the
// message bucket because chat actions are not messages.
func (s *Sender) Typing(ctx context.Context, chat int64) {
	_, _ = s.b.SendChatAction(ctx, &bot.SendChatActionParams{ChatID: s.chat(chat), Action: models.ChatActionTyping})
}

// Photo sends a JPEG photo with an optional caption.
func (s *Sender) Photo(ctx context.Context, chat int64, jpeg []byte, caption string, replyTo int) error {
	params := &bot.SendPhotoParams{ChatID: s.chat(chat), Photo: &models.InputFileUpload{Filename: "screen.jpg", Data: bytes.NewReader(jpeg)}, Caption: caption}
	if replyTo != 0 {
		params.ReplyParameters = &models.ReplyParameters{MessageID: replyTo, AllowSendingWithoutReply: true}
	}
	return s.call(ctx, func() error { _, e := s.b.SendPhoto(ctx, params); return e })
}

// Video uploads an MP4 clip.
func (s *Sender) Video(ctx context.Context, chat int64, mp4 []byte, seconds int, caption string, replyTo int) error {
	params := &bot.SendVideoParams{ChatID: s.chat(chat), Video: &models.InputFileUpload{Filename: "camera.mp4", Data: bytes.NewReader(mp4)},
		Duration: seconds, SupportsStreaming: true, Caption: caption}
	if replyTo != 0 {
		params.ReplyParameters = &models.ReplyParameters{MessageID: replyTo, AllowSendingWithoutReply: true}
	}
	return s.call(ctx, func() error { _, e := s.b.SendVideo(ctx, params); return e })
}

// Answer acknowledges a callback query.
func (s *Sender) Answer(ctx context.Context, id, text string) {
	_, _ = s.b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: id, Text: text})
}

// Outbox delivers messages that must not be lost (final job results) in
// order, retrying network failures with backoff while Telegram is unreachable.
type Outbox struct {
	s      *Sender
	log    *slog.Logger
	mu     sync.Mutex
	queue  []Out
	notify chan struct{}
	max    int
}

// NewOutbox creates an outbox holding at most max pending messages.
func NewOutbox(s *Sender, log *slog.Logger, max int) *Outbox {
	return &Outbox{s: s, log: log, notify: make(chan struct{}, 1), max: max}
}

// Push enqueues a message (dropping the oldest when full).
func (o *Outbox) Push(m Out) {
	o.mu.Lock()
	if len(o.queue) >= o.max {
		o.queue = o.queue[1:]
		o.log.Warn("outbox full; dropped oldest message")
	}
	o.queue = append(o.queue, m)
	o.mu.Unlock()
	select {
	case o.notify <- struct{}{}:
	default:
	}
}

// Len returns the number of pending messages.
func (o *Outbox) Len() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.queue)
}

// Run delivers until ctx ends, then makes a final bounded attempt (flush).
func (o *Outbox) Run(ctx context.Context) {
	backoff := time.Second
	for {
		o.mu.Lock()
		var m *Out
		if len(o.queue) > 0 {
			m = &o.queue[0]
		}
		o.mu.Unlock()
		if m == nil {
			select {
			case <-ctx.Done():
				return
			case <-o.notify:
				continue
			}
		}
		_, err := o.s.Send(ctx, *m)
		if err != nil && !IsAPIError(err) && ctx.Err() == nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		o.mu.Lock()
		if len(o.queue) > 0 {
			o.queue = o.queue[1:]
		}
		o.mu.Unlock()
		if ctx.Err() != nil {
			return
		}
	}
}

// Flush tries to send whatever is pending within the deadline.
func (o *Outbox) Flush(ctx context.Context) {
	for {
		o.mu.Lock()
		if len(o.queue) == 0 {
			o.mu.Unlock()
			return
		}
		m := o.queue[0]
		o.queue = o.queue[1:]
		o.mu.Unlock()
		if _, err := o.s.Send(ctx, m); err != nil && ctx.Err() != nil {
			return
		}
	}
}
