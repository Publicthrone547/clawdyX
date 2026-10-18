package screen

import (
	"bytes"
	"context"
	"image/png"
	"testing"
	"unsafe"
)

func TestNormalizeVirtualScreen(t *testing.T) {
	// Two monitors: one at 1920×1080, another left of it at -1280.
	vs := rect{Left: -1280, Top: 0, Right: 1920, Bottom: 1080}
	if x, y := Normalize(-1280, 0, vs); x != 0 || y != 0 {
		t.Fatal(x, y)
	}
	if x, y := Normalize(1919, 1079, vs); x != 65535 || y != 65535 {
		t.Fatal(x, y)
	}
	if x, _ := Normalize(0, 540, vs); x < 26200 || x > 26230 { // 1280 of 3199 px → ~26219
		t.Fatal(x)
	}
	if x, y := Normalize(99999, -50, vs); x != 65535 || y != 0 {
		t.Fatal("outside points are clamped", x, y)
	}
}

func TestMonitorsToDisplays(t *testing.T) {
	ds := MonitorsToDisplays([]rawMonitor{
		{Rect: rect{-2560, 0, 0, 1440}, Device: `\\.\DISPLAY2`, Name: "DELL U2723QE", DeviceID: `MONITOR\DELA1\{x}\0001`, DPI: 144},
		{Rect: rect{0, 0, 1920, 1080}, Primary: true, Device: `\\.\DISPLAY1`, Name: "Generic PnP Monitor", DPI: 96},
	})
	if ds[0].Index != 1 || !ds[0].Main || ds[0].Name != "DISPLAY1" || ds[0].PixelW != 1920 {
		t.Fatalf("primary first: %+v", ds[0])
	}
	if ds[1].X != -2560 || ds[1].Scale != 1.5 || ds[1].Name != "DELL U2723QE" || ds[1].ID != `\\.\DISPLAY2` {
		t.Fatalf("second: %+v", ds[1])
	}
	if ds[0].Key == ds[1].Key || ds[1].Key[:4] != "mon-" {
		t.Fatal("keys")
	}
}

func TestInputLayout(t *testing.T) {
	if unsafe.Sizeof(input{}) != 40 || unsafe.Sizeof(mouseInput{}) > 32 || unsafe.Sizeof(keybdInput{}) > 32 {
		t.Fatalf("INPUT %d mouse %d keyboard %d", unsafe.Sizeof(input{}), unsafe.Sizeof(mouseInput{}), unsafe.Sizeof(keybdInput{}))
	}
	ks := UnicodeKeys("Привет, 😀")
	if len(ks) != 2*(len([]rune("Привет, "))+2) || ks[0].Scan != 'П' || ks[1].Flags != keyUnicode|keyUp {
		t.Fatalf("%d keys, first %+v", len(ks), ks[0])
	}
	pix := []byte{1, 2, 3, 0}
	BGRAToRGBA(pix)
	if pix[0] != 3 || pix[2] != 1 || pix[3] != 255 {
		t.Fatal(pix)
	}
}

func TestVirtualKeys(t *testing.T) {
	for k, want := range map[string]uint16{"s": 'S', "0": '0', "return": 0x0D, "f12": 0x7B, "left": 0x25} {
		if vk, _, err := VirtualKey(k); err != nil || vk != want {
			t.Errorf("%s: %x %v", k, vk, err)
		}
	}
	if _, ext, _ := VirtualKey("pagedown"); !ext {
		t.Error("navigation keys are extended")
	}
	if _, _, err := SplitCombo("cmd+s"); err == nil {
		t.Error("cmd is a macOS modifier")
	}
}

// TestCaptureTheMonitors needs an interactive desktop (the PC, or the
// windows-latest runner's virtual display). It keeps the images in memory.
func TestCaptureTheMonitors(t *testing.T) {
	ds, err := Win{}.Displays(context.Background())
	if err != nil {
		t.Skipf("no desktop: %v", err)
	}
	for _, d := range ds {
		raw, err := Win{}.CapturePNG(context.Background(), d)
		if err != nil {
			t.Fatalf("display %d: %v", d.Index, err)
		}
		img, err := png.Decode(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		if b := img.Bounds(); b.Dx() != d.PixelW || b.Dy() != d.PixelH {
			t.Fatalf("display %d: image %v for %d×%d", d.Index, b, d.PixelW, d.PixelH)
		}
		t.Logf("display %s: %d×%d at %v,%v scale %.2f", d.Label(), d.PixelW, d.PixelH, d.X, d.Y, d.Scale)
	}
	shot, err := Win{}.Screenshot(context.Background(), "1")
	if err != nil || len(shot.JPEG) == 0 || shot.Width > MaxImageEdge {
		t.Fatalf("screenshot: %v %dx%d", err, shot.Width, shot.Height)
	}
}

// TestMoveToTheSamePoint sends one real mouse move to where the pointer
// already is (nothing visible happens) and checks it lands there, which
// proves the virtual-screen normalisation and SendInput layout.
func TestMoveToTheSamePoint(t *testing.T) {
	InitDPI()
	var p point
	if r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p))); r == 0 {
		t.Skip("no cursor (no interactive desktop)")
	}
	if !(Win{}).Trusted(context.Background()) {
		t.Skip("the input desktop is not reachable (locked?)")
	}
	if err := (Win{}).Move(context.Background(), float64(p.X), float64(p.Y)); err != nil {
		t.Fatal(err)
	}
	var q point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&q)))
	if d := q.X - p.X; d < -1 || d > 1 || q.Y-p.Y < -1 || q.Y-p.Y > 1 {
		t.Fatalf("pointer moved from %v to %v", p, q)
	}
}
