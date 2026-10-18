package telegram

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/platform"
)

// TestRepliesUseTheWordsOfEachOS renders the bot's menu, help, status,
// doctor, screen, camera and tier replies and every Diagnostics line with
// the words of each OS (whatever OS runs the test) and checks them against
// that OS's forbidden list.
func TestRepliesUseTheWordsOfEachOS(t *testing.T) {
	for _, w := range platform.All {
		t.Run(w.OS, func(t *testing.T) {
			h := startOpts(t, func(c *config.Config) {
				c.Desktop.ScreenControl, c.Desktop.Camera = true, true
			}, Options{Words: w, Desktop: fakeDesk{}, Cameras: fakeCamera{shots: &atomic.Int64{}, denied: true}})
			for _, cmd := range []string{"/help", "/start", "/status", "/mode", "/mode automation", "/shutdown", "/screens", "/cameras", "/screen on", "/doctor"} {
				h.tg.push(msg(owner, owner, "private", cmd))
			}
			h.tg.waitFor(t, "Diagnostics finished")
			h.tg.waitFor(t, "<b>Doctor</b>")
			var texts []string
			for _, c := range h.tg.sent() {
				texts = append(texts, c.Form["text"], c.Form["commands"], c.Form["description"], c.Form["short_description"])
			}
			for _, d := range h.app.diagnostics(context.Background()) {
				texts = append(texts, d.Label, d.Detail, d.Fix)
			}
			texts = append(texts, h.app.modeHelp(config.ModeAutomation))
			// Data of the machine that runs the test is not wording: its
			// paths and the checks of its own OS (built in, whatever Words
			// says) are masked when another OS's words are rendered.
			var hostData []string
			if w.OS != platform.W.OS {
				hostData = append(hostData, h.cfg.Claude.Binary)
				for _, c := range h.app.platformChecks(context.Background()) {
					hostData = append(hostData, c.Label, c.Detail, c.Fix)
				}
			}
			for i, s := range texts {
				for _, d := range hostData {
					if d != "" {
						s = strings.ReplaceAll(strings.ReplaceAll(s, Esc(d), "<host>"), d, "<host>")
					}
				}
				texts[i] = s
			}
			for _, s := range texts {
				if bad := platform.ForbiddenWords(w.OS, s); len(bad) > 0 {
					t.Errorf("%s: %v in %q", w.OS, bad, s)
				}
			}
			all := strings.Join(texts, "\n")
			if !strings.Contains(all, w.ThisComputer) {
				t.Errorf("%s: the replies never say %q", w.OS, w.ThisComputer)
			}
		})
	}
}
