package telegram

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-telegram/bot/models"

	"github.com/Publicthrone547/clawdyX/internal/screen"
)

// cmdScreenshot sends the owner a photo of the main display. Only the owner
// can trigger it; the image goes to the owner's private chat.
func (a *App) cmdScreenshot(m *models.Message) {
	if !a.cfg.Desktop.ScreenControl {
		a.reply(m.ID, "Screen control is off. Set \"desktop\": {\"screen_control\": true} in ~/.config/clawdyx/config.json and restart the bot.")
		return
	}
	a.go_(func() {
		ctx, cancel := context.WithTimeout(a.ctx, 40*time.Second)
		defer cancel()
		shot, err := a.desktop.Screenshot(ctx)
		if err != nil {
			if errors.Is(err, screen.ErrNoScreenRecording) {
				a.reply(m.ID, "❌ "+Esc(err.Error())+". Grant it, then restart the bot.")
				return
			}
			a.reply(m.ID, "❌ Screenshot failed: "+Esc(err.Error()))
			return
		}
		a.st.Audit("desktop", "screenshot", "", "ok", 0)
		if err := a.send.Photo(ctx, shot.JPEG, fmt.Sprintf("Main display %dx%d", shot.Width, shot.Height), m.ID); err != nil {
			a.reply(m.ID, "❌ Could not send the screenshot to Telegram.")
		}
	})
}
