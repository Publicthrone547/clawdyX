package screen

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// ErrNoWaylandInput: synthetic input on Wayland needs the RemoteDesktop
// portal with PipeWire, which clawdyX does not have yet.
var ErrNoWaylandInput = errors.New("mouse and keyboard tools are not available on Wayland yet; use an X11 session for screen control (screenshots still work)")

// Wayland is the Linux implementation for a Wayland session: screenshots
// only, through the desktop portal (org.freedesktop.portal.Screenshot) or
// grim on wlroots compositors. The portal may ask the person at the
// computer; it cannot list monitors, so the whole screen is one display.
type Wayland struct{}

var waylandDisplay = Display{Index: 1, ID: "wayland", Key: "wayland-screen", Name: "Wayland screen", Scale: 1, Main: true}

// Displays reports the whole screen as one display.
func (Wayland) Displays(context.Context) ([]Display, error) { return []Display{waylandDisplay}, nil }

// CapturePNG captures the whole screen.
func (Wayland) CapturePNG(ctx context.Context, _ Display) ([]byte, error) {
	desktop := strings.ToUpper(os.Getenv("XDG_CURRENT_DESKTOP"))
	if grim, err := exec.LookPath("grim"); err == nil && !strings.Contains(desktop, "GNOME") && !strings.Contains(desktop, "KDE") {
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		out, err := exec.CommandContext(cctx, grim, "-t", "png", "-").Output()
		if err == nil && len(out) > 0 {
			return out, nil
		}
	}
	return portalScreenshot(ctx)
}

// Screenshot captures the screen, scaled for the model (the size is known
// only after the capture).
func (w Wayland) Screenshot(ctx context.Context, _ string) (Shot, error) {
	raw, err := w.CapturePNG(ctx, waylandDisplay)
	if err != nil {
		return Shot{}, err
	}
	src, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return Shot{}, fmt.Errorf("decode capture: %w", err)
	}
	b := src.Bounds()
	d := waylandDisplay
	d.W, d.H, d.PixelW, d.PixelH = float64(b.Dx()), float64(b.Dy()), b.Dx(), b.Dy()
	scale := math.Min(1, MaxImageEdge/math.Max(d.W, d.H))
	iw, ih := int(math.Round(d.W*scale)), int(math.Round(d.H*scale))
	var img image.Image = src
	enc, err := EncodeJPEGSize(img, iw, ih, 70)
	if err != nil {
		return Shot{}, err
	}
	return Shot{JPEG: enc.JPEG, Width: iw, Height: ih, ScreenW: d.W, ScreenH: d.H, Scale: scale, Display: d}, nil
}

func portalScreenshot(ctx context.Context) ([]byte, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("no session bus for the screenshot portal (%v)", err)
	}
	defer conn.Close()
	tok := make([]byte, 6)
	_, _ = rand.Read(tok)
	token := "clawdyx" + hex.EncodeToString(tok)
	names := conn.Names()
	if len(names) == 0 {
		return nil, errors.New("the session bus gave no name")
	}
	sender := strings.ReplaceAll(strings.TrimPrefix(names[0], ":"), ".", "_")
	path := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/" + sender + "/" + token)
	if err := conn.AddMatchSignal(dbus.WithMatchObjectPath(path), dbus.WithMatchInterface("org.freedesktop.portal.Request"), dbus.WithMatchMember("Response")); err != nil {
		return nil, err
	}
	signals := make(chan *dbus.Signal, 4)
	conn.Signal(signals)
	obj := conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")
	opts := map[string]dbus.Variant{"handle_token": dbus.MakeVariant(token), "interactive": dbus.MakeVariant(false)}
	if call := obj.CallWithContext(ctx, "org.freedesktop.portal.Screenshot.Screenshot", 0, "", opts); call.Err != nil {
		return nil, fmt.Errorf("the screenshot portal is not available (%v)", call.Err)
	}
	wait, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for {
		select {
		case <-wait.Done():
			return nil, Denied("the desktop asked for permission on the computer; answer it there and try again")
		case s := <-signals:
			if s == nil || s.Path != path || len(s.Body) < 2 {
				continue
			}
			code, _ := s.Body[0].(uint32)
			if code != 0 {
				return nil, Denied("screenshots were refused by the desktop portal on the computer")
			}
			results, _ := s.Body[1].(map[string]dbus.Variant)
			uri, _ := results["uri"].Value().(string)
			return readPortalFile(uri)
		}
	}
}

// readPortalFile reads the portal's screenshot file and deletes it.
func readPortalFile(uri string) ([]byte, error) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" || u.Path == "" {
		return nil, fmt.Errorf("the portal returned no file (%q)", uri)
	}
	defer os.Remove(u.Path)
	b, err := os.ReadFile(u.Path)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return nil, errors.New("the capture produced an empty image")
	}
	return b, nil
}

// Click implements Desktop.
func (Wayland) Click(context.Context, float64, float64, string, int) error { return ErrNoWaylandInput }

// Move implements Desktop.
func (Wayland) Move(context.Context, float64, float64) error { return ErrNoWaylandInput }

// Scroll implements Desktop.
func (Wayland) Scroll(context.Context, float64, float64, int, int) error { return ErrNoWaylandInput }

// TypeText implements Desktop.
func (Wayland) TypeText(context.Context, string) error { return ErrNoWaylandInput }

// PressKey implements Desktop.
func (Wayland) PressKey(context.Context, string) error { return ErrNoWaylandInput }

// Trusted implements Desktop: input is not available.
func (Wayland) Trusted(context.Context) bool { return false }

// InputError implements InputReporter.
func (Wayland) InputError() error { return ErrNoWaylandInput }
