// Package screen gives Claude eyes and hands on the computer's displays:
// screenshots, mouse clicks, typing, key presses and scrolling. It is served
// to Claude Code as an MCP server (`clawdyx mcp-screen`) and is only
// enabled for automation-tier sessions when desktop.screen_control is true.
//
// Each OS has its own Desktop (see NewDesktop): the macOS one needs the
// Screen Recording and Accessibility permissions, which only the owner can
// grant; where an OS or a session cannot capture or send input, the
// Desktop returns an error that says why.
package screen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"math"
	"strconv"
)

// MaxImageEdge bounds the screenshot's long edge in pixels.
const MaxImageEdge = 1456

// Shot is a screenshot scaled for the model.
type Shot struct {
	JPEG             []byte
	Width, Height    int     // image size in pixels
	ScreenW          float64 // display size in screen units (points on macOS, pixels elsewhere)
	ScreenH          float64
	Scale            float64 // image pixels per screen unit
	OriginX, OriginY float64 // display origin in global screen units
	Display          Display
}

// Desktop performs the actions. Coordinates are global screen units (see
// Display).
type Desktop interface {
	// Screenshot captures one display ("" or "1" = main; see Select).
	Screenshot(ctx context.Context, display string) (Shot, error)
	Click(ctx context.Context, x, y float64, button string, count int) error
	Move(ctx context.Context, x, y float64) error
	Scroll(ctx context.Context, x, y float64, dx, dy int) error
	TypeText(ctx context.Context, text string) error
	// PressKey presses a key or shortcut such as "return" or "ctrl+s"; the
	// modifier names depend on the OS (KeyHelp).
	PressKey(ctx context.Context, combo string) error
	// Trusted reports whether input actions can work in this session.
	Trusted(ctx context.Context) bool
}

// ErrUnsupported wraps the reason screen features cannot work here.
var ErrUnsupported = errors.New("screen features are not available")

// screenshot captures one display of p and scales it so the long edge is
// at most MaxImageEdge pixels and never above the display's size in screen
// units (so image pixels map back to screen units by Scale).
func screenshot(ctx context.Context, p Provider, display string) (Shot, error) {
	ds, err := p.Displays(ctx)
	if err != nil {
		return Shot{}, err
	}
	d, err := Select(ds, display)
	if err != nil {
		return Shot{}, err
	}
	raw, err := p.CapturePNG(ctx, d)
	if err != nil {
		return Shot{}, err
	}
	src, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return Shot{}, fmt.Errorf("decode capture: %w", err)
	}
	scale := math.Min(1, MaxImageEdge/math.Max(d.W, d.H))
	w, h := int(math.Round(d.W*scale)), int(math.Round(d.H*scale))
	enc, err := EncodeJPEGSize(src, w, h, 70)
	if err != nil {
		return Shot{}, err
	}
	return Shot{JPEG: enc.JPEG, Width: w, Height: h, ScreenW: d.W, ScreenH: d.H, Scale: scale, OriginX: d.X, OriginY: d.Y, Display: d}, nil
}

func ff(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

// Unsupported is the Desktop and Provider of a system or session without
// screen features; every action returns Err.
type Unsupported struct{ Err error }

func (u Unsupported) err() error {
	if u.Err == nil {
		return ErrUnsupported
	}
	return u.Err
}

// Displays implements Provider.
func (u Unsupported) Displays(context.Context) ([]Display, error) { return nil, u.err() }

// CapturePNG implements Provider.
func (u Unsupported) CapturePNG(context.Context, Display) ([]byte, error) { return nil, u.err() }

// Screenshot implements Desktop.
func (u Unsupported) Screenshot(context.Context, string) (Shot, error) { return Shot{}, u.err() }

// Click implements Desktop.
func (u Unsupported) Click(context.Context, float64, float64, string, int) error { return u.err() }

// Move implements Desktop.
func (u Unsupported) Move(context.Context, float64, float64) error { return u.err() }

// Scroll implements Desktop.
func (u Unsupported) Scroll(context.Context, float64, float64, int, int) error { return u.err() }

// TypeText implements Desktop.
func (u Unsupported) TypeText(context.Context, string) error { return u.err() }

// PressKey implements Desktop.
func (u Unsupported) PressKey(context.Context, string) error { return u.err() }

// Trusted implements Desktop.
func (u Unsupported) Trusted(context.Context) bool { return false }

// InputError implements InputReporter.
func (u Unsupported) InputError() error { return u.err() }

// ErrCaptureDenied means the OS or the desktop refused a capture (a
// permission or a portal dialog). Providers return their own text through
// Denied; errors.Is matches it.
var ErrCaptureDenied = errors.New("the screen capture was refused")

type deniedError struct{ msg string }

func (e deniedError) Error() string        { return e.msg }
func (e deniedError) Is(target error) bool { return target == ErrCaptureDenied }

// Denied is an ErrCaptureDenied with an OS-specific explanation.
func Denied(msg string) error { return deniedError{msg} }
