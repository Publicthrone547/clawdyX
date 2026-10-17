package screen

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"
)

type fakeProv struct {
	ds    []Display
	calls int
}

func (f *fakeProv) Displays(context.Context) ([]Display, error) { return f.ds, nil }
func (f *fakeProv) CapturePNG(_ context.Context, d Display) ([]byte, error) {
	f.calls++
	img := image.NewRGBA(image.Rect(0, 0, d.PixelW, d.PixelH))
	img.Set(1, 1, color.RGBA{255, 0, 0, 255})
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes(), nil
}

func twoDisplays() *fakeProv {
	return &fakeProv{ds: []Display{
		{Index: 1, ID: 4, Key: "6614AAAA-1", Name: "Main", W: 1720, H: 720, PixelW: 1720, PixelH: 720, Scale: 1, Main: true},
		{Index: 2, ID: 5, Key: "ACCDBBBB-2", Name: "Side", X: -320, Y: -240, W: 320, H: 240, PixelW: 320, PixelH: 240, Scale: 1},
	}}
}

func TestBrokerSwitchAuditAndLimits(t *testing.T) {
	p := twoDisplays()
	off := true
	var audits []AuditRecord
	b := NewBroker(p, func() error {
		if off {
			return ErrScreenOff
		}
		return nil
	}, func(r AuditRecord) { audits = append(audits, r) })
	if _, err := b.Capture(context.Background(), "1", "telegram", "owner"); !errors.Is(err, ErrScreenOff) {
		t.Fatalf("switched off: %v", err)
	}
	if p.calls != 0 {
		t.Fatal("captured while off")
	}
	off = false
	cs, err := b.Capture(context.Background(), "all", "preview", "gui")
	if err != nil || len(cs) != 2 {
		t.Fatalf("all: %v %d", err, len(cs))
	}
	if cs[0].Preview.Width != 1568 || cs[0].Preview.SrcW != 1720 || cs[1].Preview.Width != 320 {
		t.Fatalf("preview sizes %+v %+v", cs[0].Preview, cs[1].Preview)
	}
	if len(audits) != 2 || audits[1].Destination != "preview" || audits[1].Bytes == 0 {
		t.Fatalf("audits %+v", audits)
	}
	if _, err := b.Capture(context.Background(), "3", "preview", "gui"); !errors.Is(err, ErrNoSuchDisplay) {
		t.Fatalf("missing display: %v", err)
	}
	if _, err := b.Capture(context.Background(), "accdbb", "preview", "gui"); err != nil {
		t.Fatalf("key prefix: %v", err)
	}
	for i := 0; i < 10; i++ {
		_, _ = b.Capture(context.Background(), "2", "preview", "gui")
	}
	if b.Held() != HoldMax {
		t.Fatalf("held %d want %d", b.Held(), HoldMax)
	}
	now := time.Now()
	b.now = func() time.Time { return now.Add(HoldTTL + time.Second) }
	if b.Held() != 0 {
		t.Fatal("TTL not enforced")
	}
	b.now = time.Now
	cs, _ = b.Capture(context.Background(), "", "preview", "gui")
	if _, ok := b.Get(cs[0].ID); !ok {
		t.Fatal("capture not held")
	}
	b.Discard(cs[0].ID)
	if _, ok := b.Get(cs[0].ID); ok {
		t.Fatal("discard failed")
	}
}

func TestParseDisplaysOrderAndKeys(t *testing.T) {
	out := `[{"id":5,"name":"BenQ","x":-1920,"y":-1080,"w":1920,"h":1080,"pw":1920,"ph":1080,"scale":1,"main":false,"vendor":2513,"model":31044,"serial":21573,"uuid":""},
	{"id":4,"name":"VG34","x":0,"y":0,"w":3440,"h":1440,"pw":3440,"ph":1440,"scale":1,"main":true,"uuid":"661406D5-EFCE"},
	{"id":1,"name":"","x":3440,"y":0,"w":1512,"h":982,"pw":3024,"ph":1964,"scale":2,"builtin":true,"uuid":"X"}]`
	ds, err := ParseDisplays(out)
	if err != nil {
		t.Fatal(err)
	}
	if ds[0].ID != 4 || ds[0].Index != 1 || ds[1].ID != 5 || ds[2].ID != 1 || ds[2].Name != "Display 1" {
		t.Fatalf("order %+v", ds)
	}
	if ds[1].Key == "" || ds[1].Key[:3] != "hw-" {
		t.Fatalf("fallback key %q", ds[1].Key)
	}
	if ds[2].PixelW != 3024 {
		t.Fatal("retina pixel size lost")
	}
}

func TestEncodeJPEGNeverUpscales(t *testing.T) {
	p := &fakeProv{}
	raw, _ := p.CapturePNG(context.Background(), Display{PixelW: 800, PixelH: 600})
	e, err := EncodeJPEG(raw, 1568, 70)
	if err != nil || e.Width != 800 || e.Height != 600 {
		t.Fatalf("%v %+v", err, e)
	}
	e, _ = EncodeJPEG(raw, 400, 70)
	if e.Width != 400 || e.Height != 300 {
		t.Fatalf("downscale %+v", e)
	}
}

type deniedProv struct{ fakeProv }

func (d *deniedProv) CapturePNG(context.Context, Display) ([]byte, error) {
	return nil, ErrNoScreenRecording
}

func TestBrokerAccessFromLastCapture(t *testing.T) {
	p := &deniedProv{fakeProv: *twoDisplays()}
	b := NewBroker(p, nil, nil)
	if a, _ := b.Access(); a != "unknown" {
		t.Fatalf("before any capture: %s", a)
	}
	if _, err := b.Capture(context.Background(), "1", "preview", "t"); !errors.Is(err, ErrNoScreenRecording) {
		t.Fatalf("want permission error, got %v", err)
	}
	if a, _ := b.Access(); a != "denied" {
		t.Fatalf("after a refused capture: %s", a)
	}
	b.P = twoDisplays()
	if _, err := b.Capture(context.Background(), "2", "preview", "t"); err != nil {
		t.Fatal(err)
	}
	if a, _ := b.Access(); a != "granted" {
		t.Fatalf("after a capture: %s", a)
	}
}

// BenchmarkEncodeLargeDisplay measures the re-encode step for a 3440×1440
// capture (the largest display on the development Mac) to the 1568-px JPEG
// sent to Claude.
func BenchmarkEncodeLargeDisplay(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 3440, 1440))
	for y := 0; y < 1440; y += 3 {
		for x := 0; x < 3440; x += 7 {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 90, 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	raw := buf.Bytes()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := EncodeJPEG(raw, ClaudeMaxEdge, 75); err != nil {
			b.Fatal(err)
		}
	}
}
