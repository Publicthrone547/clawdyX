// Package telegram is the Telegram front end: long polling, the owner-only
// authorization gate, command routing, job streaming and rate-limited sends.
package telegram

import (
	"github.com/go-telegram/bot/models"
)

// AllowedUpdates is the only set of update types requested from Telegram.
var AllowedUpdates = []string{"message", "callback_query", "stopped_message_generation"}

// Authorize is the single, centralized identity gate. It accepts an update
// only when the sender is the owner, the chat is a private chat, and the
// chat is the owner's own chat. Everything else is rejected silently by the
// caller, before any lookup or side effect.
func Authorize(u *models.Update, owner int64) bool {
	if u == nil || owner <= 0 {
		return false
	}
	switch {
	case u.Message != nil:
		m := u.Message
		if m.From == nil || m.From.IsBot || m.SenderChat != nil || m.From.ID != owner {
			return false
		}
		return m.Chat.Type == models.ChatTypePrivate && m.Chat.ID == owner
	case u.CallbackQuery != nil:
		cq := u.CallbackQuery
		if cq.From.ID != owner || cq.From.IsBot || cq.InlineMessageID != "" {
			return false
		}
		m := cq.Message.Message
		if m == nil {
			return false
		}
		return m.Chat.Type == models.ChatTypePrivate && m.Chat.ID == owner
	case u.StoppedMessageGeneration != nil:
		c := u.StoppedMessageGeneration.Chat
		return c.Type == models.ChatTypePrivate && c.ID == owner
	}
	return false
}
