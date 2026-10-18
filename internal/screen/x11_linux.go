package screen

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/png"
	"sort"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/randr"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

// X11 is the Linux implementation for an X11 session: RandR for the
// outputs, GetImage on the root window for captures and XTest for the
// mouse and keyboard, in root-window pixels. It talks the X protocol in Go
// (no cgo). DISPLAY selects the server; a worker started by systemd sees it
// only when the desktop imported it into the user manager.
type X11 struct{}

func xconn() (*xgb.Conn, xproto.Window, error) {
	c, err := xgb.NewConn()
	if err != nil {
		return nil, 0, fmt.Errorf("the X server is not reachable (%v)", err)
	}
	return c, xproto.Setup(c).DefaultScreen(c).Root, nil
}

// Displays lists the active outputs (nothing captured).
func (X11) Displays(context.Context) ([]Display, error) {
	c, root, err := xconn()
	if err != nil {
		return nil, err
	}
	defer c.Close()
	ds := randrDisplays(c, root)
	if len(ds) == 0 {
		// No RandR (or a nested server): the whole root window is one display.
		s := xproto.Setup(c).DefaultScreen(c)
		ds = []Display{{ID: "root", Key: "x11-root", Name: "Screen", W: float64(s.WidthInPixels), H: float64(s.HeightInPixels),
			PixelW: int(s.WidthInPixels), PixelH: int(s.HeightInPixels), Scale: 1, Main: true}}
	}
	sort.SliceStable(ds, func(i, j int) bool {
		if ds[i].Main != ds[j].Main {
			return ds[i].Main
		}
		if ds[i].X != ds[j].X {
			return ds[i].X < ds[j].X
		}
		return ds[i].Y < ds[j].Y
	})
	for i := range ds {
		ds[i].Index = i + 1
	}
	return ds, nil
}

func randrDisplays(c *xgb.Conn, root xproto.Window) []Display {
	if randr.Init(c) != nil {
		return nil
	}
	res, err := randr.GetScreenResourcesCurrent(c, root).Reply()
	if err != nil {
		return nil
	}
	var primary randr.Output
	if p, err := randr.GetOutputPrimary(c, root).Reply(); err == nil {
		primary = p.Output
	}
	edidAtom := xproto.Atom(0)
	if a, err := xproto.InternAtom(c, true, uint16(len("EDID")), "EDID").Reply(); err == nil {
		edidAtom = a.Atom
	}
	var ds []Display
	for _, o := range res.Outputs {
		info, err := randr.GetOutputInfo(c, o, res.ConfigTimestamp).Reply()
		if err != nil || info.Crtc == 0 || info.Connection != randr.ConnectionConnected {
			continue
		}
		crtc, err := randr.GetCrtcInfo(c, info.Crtc, res.ConfigTimestamp).Reply()
		if err != nil || crtc.Width == 0 {
			continue
		}
		output := string(info.Name)
		name, keySrc := output, output
		if edidAtom != 0 {
			if p, err := randr.GetOutputProperty(c, o, edidAtom, xproto.AtomAny, 0, 64, false, false).Reply(); err == nil && len(p.Data) >= 128 {
				if n := ParseEDIDName(p.Data); n != "" {
					name = output + " · " + n
				}
				keySrc = hex.EncodeToString(p.Data[:128])
			}
		}
		sum := sha256.Sum256([]byte(keySrc))
		ds = append(ds, Display{
			ID: output, Key: "x11-" + hex.EncodeToString(sum[:8]), Name: name,
			X: float64(crtc.X), Y: float64(crtc.Y), W: float64(crtc.Width), H: float64(crtc.Height),
			PixelW: int(crtc.Width), PixelH: int(crtc.Height), Scale: 1, Main: o == primary,
			Builtin: len(output) >= 3 && (output[:3] == "eDP" || output[:3] == "LVD" || output[:3] == "DSI"),
		})
	}
	if len(ds) > 0 && primary == 0 {
		ds[0].Main = true
	}
	return ds
}

// CapturePNG reads one output's area of the root window (GetImage).
func (X11) CapturePNG(_ context.Context, d Display) ([]byte, error) {
	c, root, err := xconn()
	if err != nil {
		return nil, err
	}
	defer c.Close()
	w, h := int(d.W), int(d.H)
	img, err := xproto.GetImage(c, xproto.ImageFormatZPixmap, xproto.Drawable(root), int16(d.X), int16(d.Y), uint16(w), uint16(h), 0xffffffff).Reply()
	if err != nil {
		return nil, fmt.Errorf("X11 capture: %v", err)
	}
	if len(img.Data) < w*h*4 {
		return nil, fmt.Errorf("X11 capture: %d bytes for %d×%d (depth %d is not supported)", len(img.Data), w, h, img.Depth)
	}
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	copy(rgba.Pix, img.Data[:w*h*4])
	BGRAToRGBA(rgba.Pix)
	var buf bytes.Buffer
	if err := png.Encode(&buf, rgba); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Screenshot captures one display, scaled for the model.
func (x X11) Screenshot(ctx context.Context, display string) (Shot, error) {
	return screenshot(ctx, x, display)
}

// X event types for XTest FakeInput.
const (
	xKeyPress      = 2
	xKeyRelease    = 3
	xButtonPress   = 4
	xButtonRelease = 5
	xMotionNotify  = 6
)

// xinput is one open connection with XTest and the keyboard mapping.
type xinput struct {
	c        *xgb.Conn
	root     xproto.Window
	min, max xproto.Keycode
	per      int
	syms     []xproto.Keysym
	remapped map[xproto.Keycode][]xproto.Keysym // spare keycodes to restore
}

func openInput() (*xinput, error) {
	c, root, err := xconn()
	if err != nil {
		return nil, err
	}
	if err := xtest.Init(c); err != nil {
		c.Close()
		return nil, errors.New("the X server has no XTEST extension; mouse and keyboard tools cannot work")
	}
	s := xproto.Setup(c)
	in := &xinput{c: c, root: root, min: s.MinKeycode, max: s.MaxKeycode, remapped: map[xproto.Keycode][]xproto.Keysym{}}
	km, err := xproto.GetKeyboardMapping(c, s.MinKeycode, byte(s.MaxKeycode-s.MinKeycode+1)).Reply()
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("keyboard mapping: %v", err)
	}
	in.per, in.syms = int(km.KeysymsPerKeycode), km.Keysyms
	return in, nil
}

func (in *xinput) close() {
	for kc, old := range in.remapped {
		xproto.ChangeKeyboardMapping(in.c, 1, kc, byte(in.per), old)
	}
	in.c.Sync()
	in.c.Close()
}

func (in *xinput) fake(typ, detail byte, x, y int16) error {
	return xtest.FakeInputChecked(in.c, typ, detail, 0, in.root, x, y, 0).Check()
}

// keycode finds a keycode that types keysym (shift says whether the
// keysym sits on the shifted level). Without one, it maps the keysym onto
// a spare keycode for the rest of this connection.
func (in *xinput) keycode(ks uint32) (xproto.Keycode, bool, error) {
	for i := 0; i+in.per <= len(in.syms); i += in.per {
		kc := in.min + xproto.Keycode(i/in.per)
		if uint32(in.syms[i]) == ks {
			return kc, false, nil
		}
		if in.per > 1 && uint32(in.syms[i+1]) == ks {
			return kc, true, nil
		}
	}
	for i := len(in.syms) - in.per; i >= 0; i -= in.per {
		empty := true
		for j := 0; j < in.per; j++ {
			if in.syms[i+j] != 0 {
				empty = false
			}
		}
		if !empty {
			continue
		}
		kc := in.min + xproto.Keycode(i/in.per)
		if _, done := in.remapped[kc]; done {
			continue
		}
		old := append([]xproto.Keysym(nil), in.syms[i:i+in.per]...)
		nw := make([]xproto.Keysym, in.per)
		for j := range nw {
			nw[j] = xproto.Keysym(ks)
		}
		if err := xproto.ChangeKeyboardMappingChecked(in.c, 1, kc, byte(in.per), nw).Check(); err != nil {
			return 0, false, fmt.Errorf("keyboard mapping: %v", err)
		}
		copy(in.syms[i:i+in.per], nw)
		in.remapped[kc] = old
		time.Sleep(20 * time.Millisecond) // let clients see the new mapping
		return kc, false, nil
	}
	return 0, false, errors.New("no spare keycode to type this character")
}

func (in *xinput) tap(ks uint32) error {
	kc, shift, err := in.keycode(ks)
	if err != nil {
		return err
	}
	var shiftKC xproto.Keycode
	if shift {
		if shiftKC, _, err = in.keycode(x11Modifiers["shift"]); err != nil {
			return err
		}
		in.fake(xKeyPress, byte(shiftKC), 0, 0)
	}
	in.fake(xKeyPress, byte(kc), 0, 0)
	err = in.fake(xKeyRelease, byte(kc), 0, 0)
	if shift {
		in.fake(xKeyRelease, byte(shiftKC), 0, 0)
	}
	return err
}

// Move moves the pointer to (x, y) in root-window pixels.
func (X11) Move(_ context.Context, x, y float64) error {
	in, err := openInput()
	if err != nil {
		return err
	}
	defer in.close()
	return in.fake(xMotionNotify, 0, int16(x), int16(y))
}

// Click clicks at (x, y).
func (X11) Click(_ context.Context, x, y float64, button string, count int) error {
	in, err := openInput()
	if err != nil {
		return err
	}
	defer in.close()
	if err := in.fake(xMotionNotify, 0, int16(x), int16(y)); err != nil {
		return err
	}
	b := byte(1)
	if button == "right" {
		b = 3
	}
	time.Sleep(50 * time.Millisecond)
	for i := 0; i < count; i++ {
		in.fake(xButtonPress, b, 0, 0)
		if err := in.fake(xButtonRelease, b, 0, 0); err != nil {
			return err
		}
		time.Sleep(60 * time.Millisecond)
	}
	return nil
}

// Scroll scrolls at (x, y): buttons 4/5 are wheel up/down, 6/7 left/right.
func (X11) Scroll(_ context.Context, x, y float64, dx, dy int) error {
	in, err := openInput()
	if err != nil {
		return err
	}
	defer in.close()
	in.fake(xMotionNotify, 0, int16(x), int16(y))
	press := func(b byte, n int) {
		for i := 0; i < n; i++ {
			in.fake(xButtonPress, b, 0, 0)
			in.fake(xButtonRelease, b, 0, 0)
		}
	}
	switch {
	case dy > 0:
		press(4, dy)
	case dy < 0:
		press(5, -dy)
	}
	switch {
	case dx > 0:
		press(6, dx)
	case dx < 0:
		press(7, -dx)
	}
	in.c.Sync()
	return nil
}

// TypeText types text; characters missing from the layout are mapped onto
// a spare keycode for the duration and the mapping is restored afterwards.
func (X11) TypeText(_ context.Context, text string) error {
	in, err := openInput()
	if err != nil {
		return err
	}
	defer in.close()
	for _, r := range text {
		if err := in.tap(KeysymForRune(r)); err != nil {
			return err
		}
		time.Sleep(5 * time.Millisecond)
	}
	return nil
}

// PressKey presses a key or shortcut ("ctrl+s").
func (X11) PressKey(_ context.Context, combo string) error {
	mods, k, err := SplitCombo(combo)
	if err != nil {
		return err
	}
	ks := X11KeyKeysym(k)
	if ks == 0 {
		return fmt.Errorf("unknown key %q", k)
	}
	in, err := openInput()
	if err != nil {
		return err
	}
	defer in.close()
	var held []xproto.Keycode
	for _, m := range mods {
		kc, _, err := in.keycode(x11Modifiers[m])
		if err != nil {
			return err
		}
		in.fake(xKeyPress, byte(kc), 0, 0)
		held = append(held, kc)
	}
	kc, _, err := in.keycode(ks)
	if err == nil {
		in.fake(xKeyPress, byte(kc), 0, 0)
		err = in.fake(xKeyRelease, byte(kc), 0, 0)
	}
	for i := len(held) - 1; i >= 0; i-- {
		in.fake(xKeyRelease, byte(held[i]), 0, 0)
	}
	return err
}

// Trusted reports whether the X server takes synthetic input (XTEST).
func (X11) Trusted(context.Context) bool {
	in, err := openInput()
	if err != nil {
		return false
	}
	in.close()
	return true
}
