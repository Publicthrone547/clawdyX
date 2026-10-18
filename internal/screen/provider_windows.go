package screen

import "context"

// Win is the Windows implementation: GDI captures of the monitors and
// SendInput for the mouse and keyboard, in physical pixels of the virtual
// screen (the process is per-monitor DPI aware). No permission is needed;
// input cannot reach windows that run as administrator (UIPI), the secure
// desktop or the lock screen.
type Win struct{}

// NewDesktop returns this OS's Desktop, which is also its Provider.
func NewDesktop(string) Desktop { return Win{} }

// Screenshot captures one display, scaled for the model.
func (w Win) Screenshot(ctx context.Context, display string) (Shot, error) {
	return screenshot(ctx, w, display)
}

// Click clicks at (x, y) in virtual-screen pixels.
func (Win) Click(_ context.Context, x, y float64, button string, count int) error {
	return clickAt(x, y, button, count)
}

// Move moves the pointer.
func (Win) Move(_ context.Context, x, y float64) error {
	InitDPI()
	return send(moveTo(x, y))
}

// Scroll scrolls at (x, y).
func (Win) Scroll(_ context.Context, x, y float64, dx, dy int) error { return scrollAt(x, y, dx, dy) }

// TypeText types text into the focused window.
func (Win) TypeText(_ context.Context, text string) error { return typeText(text) }

// PressKey presses a key or shortcut ("ctrl+s", "alt+tab", "return").
func (Win) PressKey(_ context.Context, combo string) error { return pressCombo(combo) }

// Trusted reports whether input can reach the desktop that has the keyboard.
func (Win) Trusted(context.Context) bool { return inputDesktop() }
