package screen

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/draw"
)

// Display is one active display. Geometry is in points in the global
// coordinate space (origin at the main display's top-left corner).
type Display struct {
	Index   int     `json:"index"` // 1 = main, then left-to-right, top-to-bottom
	ID      uint32  `json:"id"`    // CGDirectDisplayID: stable only until reboot
	Key     string  `json:"key"`   // durable: ColorSync UUID, else hash of vendor/model/serial
	Name    string  `json:"name"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	W       float64 `json:"w"`
	H       float64 `json:"h"`
	PixelW  int     `json:"pixelW"`
	PixelH  int     `json:"pixelH"`
	Scale   float64 `json:"scale"`
	Main    bool    `json:"main"`
	Builtin bool    `json:"builtin"`
}

// Label is a short human description: "2 · BenQ EW2775ZH · 1920×1080".
func (d Display) Label() string {
	s := fmt.Sprintf("%d · %s · %d×%d", d.Index, d.Name, d.PixelW, d.PixelH)
	if d.Main {
		s += " · main"
	}
	return s
}

// ErrNoSuchDisplay means the selected display is not connected.
var ErrNoSuchDisplay = errors.New("no such display")

// jsDisplays lists active displays through AppKit and CoreGraphics. It needs
// no permission and captures nothing.
const jsDisplays = `function run(argv){
 ObjC.import("CoreGraphics"); ObjC.import("AppKit"); ObjC.import("ColorSync");
 var out = [];
 var scr = $.NSScreen.screens;
 for (var i=0;i<scr.count;i++){
   var s = scr.objectAtIndex(i);
   var id = s.deviceDescription.objectForKey("NSScreenNumber").unsignedIntValue;
   var b = $.CGDisplayBounds(id);
   var m = $.CGDisplayCopyDisplayMode(id);
   var u = $.CGDisplayCreateUUIDFromDisplayID(id);
   out.push({id:Number(id), name:String(s.localizedName.js), x:Number(b.origin.x), y:Number(b.origin.y), w:Number(b.size.width), h:Number(b.size.height),
     pw:Number($.CGDisplayModeGetPixelWidth(m)), ph:Number($.CGDisplayModeGetPixelHeight(m)), scale:Number(s.backingScaleFactor),
     main:!!$.CGDisplayIsMain(id), builtin:!!$.CGDisplayIsBuiltin(id),
     vendor:Number($.CGDisplayVendorNumber(id)), model:Number($.CGDisplayModelNumber(id)), serial:Number($.CGDisplaySerialNumber(id)),
     uuid: u ? String(ObjC.castRefToObject($.CFUUIDCreateString(null,u)).js) : ""});
 }
 return JSON.stringify(out);
}`

type rawDisplay struct {
	ID                    uint32
	Name                  string
	X, Y, W, H            float64
	PW, PH                int
	Scale                 float64
	Main, Builtin         bool
	Vendor, Model, Serial uint32
	UUID                  string
}

// ParseDisplays turns the JXA output into ordered displays.
func ParseDisplays(out string) ([]Display, error) {
	var raw []rawDisplay
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("display list: %w", err)
	}
	ds := make([]Display, 0, len(raw))
	for _, r := range raw {
		key := r.UUID
		if key == "" {
			h := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%d", r.Vendor, r.Model, r.Serial)))
			key = "hw-" + hex.EncodeToString(h[:8])
		}
		if r.PW == 0 {
			r.PW, r.PH = int(math.Round(r.W*r.Scale)), int(math.Round(r.H*r.Scale))
		}
		name := strings.TrimSpace(r.Name)
		if name == "" {
			name = "Display " + strconv.FormatUint(uint64(r.ID), 10)
		}
		ds = append(ds, Display{ID: r.ID, Key: key, Name: name, X: r.X, Y: r.Y, W: r.W, H: r.H, PixelW: r.PW, PixelH: r.PH, Scale: r.Scale, Main: r.Main, Builtin: r.Builtin})
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

// Displays lists the active displays (no permission needed, nothing captured).
func (m Mac) Displays(ctx context.Context) ([]Display, error) {
	out, err := jxa(ctx, jsDisplays)
	if err != nil {
		return nil, fmt.Errorf("display list: %w", err)
	}
	return ParseDisplays(out)
}

// Select picks a display by "" or "1"/"2"… (index), a key prefix, or a CG id.
func Select(ds []Display, sel string) (Display, error) {
	sel = strings.TrimSpace(sel)
	if sel == "" || strings.EqualFold(sel, "main") {
		sel = "1"
	}
	if n, err := strconv.Atoi(sel); err == nil {
		for _, d := range ds {
			if d.Index == n {
				return d, nil
			}
		}
		return Display{}, fmt.Errorf("%w: display %d (connected: %d)", ErrNoSuchDisplay, n, len(ds))
	}
	for _, d := range ds {
		if len(sel) >= 6 && strings.HasPrefix(strings.ToLower(d.Key), strings.ToLower(sel)) {
			return d, nil
		}
	}
	return Display{}, fmt.Errorf("%w: %q", ErrNoSuchDisplay, sel)
}

// CapturePNG captures one display at full resolution. screencapture can only
// write files: the PNG goes to a private temporary directory and is removed
// before this returns.
func (m Mac) CapturePNG(ctx context.Context, d Display) ([]byte, error) {
	dir, err := os.MkdirTemp(m.TmpDir, "cxcap")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	_ = os.Chmod(dir, 0o700)
	f := filepath.Join(dir, "d.png")
	if m.Helper != "" {
		out, err := run(ctx, 20*time.Second, m.Helper, "capture", strconv.FormatUint(uint64(d.ID), 10), f)
		var r helperResult
		_ = json.Unmarshal([]byte(lastLine(out)), &r)
		switch {
		case err == nil && r.OK:
		case strings.Contains(r.Error, "permission"):
			return nil, ErrNoScreenRecording
		case ctx.Err() != nil:
			return nil, errors.New("capture timed out (if macOS is showing a screen-access dialog, answer it on the Mac)")
		default:
			return nil, fmt.Errorf("capture helper: %s", firstNonEmpty(r.Error, out))
		}
		return readCapture(f)
	}
	rect := fmt.Sprintf("-R%s,%s,%s,%s", ff0(d.X), ff0(d.Y), ff0(d.W), ff0(d.H))
	if out, err := run(ctx, 20*time.Second, "/usr/sbin/screencapture", "-x", "-t", "png", rect, f); err != nil {
		if strings.Contains(out, "could not create image") {
			return nil, ErrNoScreenRecording
		}
		if ctx.Err() != nil {
			return nil, errors.New("capture timed out (if macOS is showing a screen-access dialog, answer it on the Mac)")
		}
		return nil, fmt.Errorf("screencapture: %v", err)
	}
	return readCapture(f)
}

func readCapture(f string) ([]byte, error) {
	b, err := os.ReadFile(f)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return nil, errors.New("the capture produced an empty image")
	}
	return b, nil
}

type helperResult struct {
	OK      bool   `json:"ok"`
	Granted bool   `json:"granted"`
	Error   string `json:"error"`
}

// Preflight reports whether macOS grants Screen Recording to this process
// without capturing anything. It needs the bundled helper (CGPreflight-
// ScreenCaptureAccess is not reachable from JXA); known is false without it.
func (m Mac) Preflight(ctx context.Context) (granted, known bool) {
	if m.Helper == "" {
		return false, false
	}
	out, err := run(ctx, 10*time.Second, m.Helper, "preflight")
	var r helperResult
	if err != nil || json.Unmarshal([]byte(lastLine(out)), &r) != nil || !r.OK {
		return false, false
	}
	return r.Granted, true
}

// RequestAccess asks macOS to show the Screen Recording prompt (once).
func (m Mac) RequestAccess(ctx context.Context) error {
	if m.Helper == "" {
		return errors.New("needs the clawdyX app's capture helper")
	}
	_, err := run(ctx, 30*time.Second, m.Helper, "request")
	return err
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return strings.TrimSpace(b)
}

func ff0(v float64) string { return strconv.FormatFloat(math.Round(v), 'f', 0, 64) }

// Encoded is a re-encoded image.
type Encoded struct {
	JPEG          []byte
	Width, Height int // of JPEG
	SrcW, SrcH    int // of the source
}

// EncodeJPEG decodes a PNG and re-encodes it as JPEG so that its long edge is
// at most maxEdge pixels (never upscaled). Re-encoding drops all metadata.
func EncodeJPEG(pngData []byte, maxEdge, quality int) (Encoded, error) {
	src, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return Encoded{}, fmt.Errorf("decode capture: %w", err)
	}
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	w, h := sw, sh
	if long := max(sw, sh); maxEdge > 0 && long > maxEdge {
		f := float64(maxEdge) / float64(long)
		w, h = max(1, int(math.Round(float64(sw)*f))), max(1, int(math.Round(float64(sh)*f)))
	}
	return EncodeJPEGSize(src, w, h, quality)
}

// EncodeJPEGSize scales img to exactly w×h and encodes it as JPEG.
func EncodeJPEGSize(img image.Image, w, h, quality int) (Encoded, error) {
	b := img.Bounds()
	var out image.Image = img
	if w != b.Dx() || h != b.Dy() {
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
		out = dst
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: quality}); err != nil {
		return Encoded{}, err
	}
	return Encoded{JPEG: buf.Bytes(), Width: w, Height: h, SrcW: b.Dx(), SrcH: b.Dy()}, nil
}
