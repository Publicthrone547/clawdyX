// Package telegram is the Telegram front end: long polling, the allowlist
// authorization gate, command routing, job streaming and rate-limited sends.
package telegram

import (
	"github.com/go-telegram/bot/models"
)

// AllowedUpdates is the only set of update types requested from Telegram.
var AllowedUpdates = []string{"message", "callback_query", "stopped_message_generation"}

// RoleFunc returns the role of a Telegram user ID, or "" when not allowed.
type RoleFunc func(id int64) string

// Authorize is the single, centralized identity gate. It accepts an update
// only when the sender is on the allowlist (numeric ID), the chat is a
// private chat, and the chat is the sender's own chat. It returns the
// sender and role. Everything else is rejected silently by the caller,
// before any lookup or side effect.
func Authorize(u *models.Update, role RoleFunc) (int64, string, bool) {
	if u == nil || role == nil {
		return 0, "", false
	}
	switch {
	case u.Message != nil:
		m := u.Message
		if m.From == nil || m.From.IsBot || m.SenderChat != nil || m.From.ID <= 0 {
			return 0, "", false
		}
		r := role(m.From.ID)
		if r == "" || m.Chat.Type != models.ChatTypePrivate || m.Chat.ID != m.From.ID {
			return 0, "", false
		}
		return m.From.ID, r, true
	case u.CallbackQuery != nil:
		cq := u.CallbackQuery
		if cq.From.IsBot || cq.InlineMessageID != "" || cq.From.ID <= 0 {
			return 0, "", false
		}
		r := role(cq.From.ID)
		m := cq.Message.Message
		if r == "" || m == nil || m.Chat.Type != models.ChatTypePrivate || m.Chat.ID != cq.From.ID {
			return 0, "", false
		}
		return cq.From.ID, r, true
	case u.StoppedMessageGeneration != nil:
		c := u.StoppedMessageGeneration.Chat
		if c.Type != models.ChatTypePrivate || c.ID <= 0 {
			return 0, "", false
		}
		r := role(c.ID)
		if r == "" {
			return 0, "", false
		}
		return c.ID, r, true
	}
	return 0, "", false
}
