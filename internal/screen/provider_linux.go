package screen

import (
	"context"
	"errors"

	"github.com/Publicthrone547/clawdyX/internal/platform"
)

// KeyHelp lists example keys for Claude's press_key tool on this OS.
const KeyHelp = "e.g. return, tab, escape, ctrl+l, ctrl+t, ctrl+w, alt+tab, shift+tab, pagedown, up"

// modifierNames: "cmd" is not a key here; ctrl does what ⌘ does on a Mac.
var modifierNames = map[string]bool{"ctrl": true, "control": true, "shift": true, "alt": true, "win": true, "super": true, "meta": true}

const modifierHint = "use ctrl, shift, alt, win"

// ErrNoGraphicalSession: a server without a desktop has no screen.
var ErrNoGraphicalSession = errors.New("there is no graphical session on this computer, so screen features are off")

// Session picks the Desktop for the current session kind: X11, Wayland
// (screenshots only) or none.
func Session() Desktop {
	switch platform.Detect().Session {
	case platform.SessionX11:
		return X11{}
	case platform.SessionWayland:
		return Wayland{}
	}
	return Unsupported{Err: ErrNoGraphicalSession}
}

// linux re-detects the session at every call (a worker started before the
// desktop may see DISPLAY only later, through a restart).
type linux struct{}

// NewDesktop returns this OS's Desktop, which is also its Provider.
func NewDesktop(string) Desktop { return linux{} }

func provider() Provider { return Session().(Provider) }

func (linux) Displays(ctx context.Context) ([]Display, error) { return provider().Displays(ctx) }
func (linux) CapturePNG(ctx context.Context, d Display) ([]byte, error) {
	return provider().CapturePNG(ctx, d)
}
func (linux) Screenshot(ctx context.Context, display string) (Shot, error) {
	return Session().Screenshot(ctx, display)
}
func (linux) Click(ctx context.Context, x, y float64, b string, n int) error {
	return Session().Click(ctx, x, y, b, n)
}
func (linux) Move(ctx context.Context, x, y float64) error { return Session().Move(ctx, x, y) }
func (linux) Scroll(ctx context.Context, x, y float64, dx, dy int) error {
	return Session().Scroll(ctx, x, y, dx, dy)
}
func (linux) TypeText(ctx context.Context, t string) error { return Session().TypeText(ctx, t) }
func (linux) PressKey(ctx context.Context, c string) error { return Session().PressKey(ctx, c) }
func (linux) Trusted(ctx context.Context) bool             { return Session().Trusted(ctx) }

// InputError implements InputReporter.
func (linux) InputError() error {
	if r, ok := Session().(InputReporter); ok {
		return r.InputError()
	}
	return nil
}
