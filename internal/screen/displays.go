package screen

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"strconv"
	"strings"

	"golang.org/x/image/draw"
)

// Display is one active display. Geometry is in the global screen space:
// points on macOS (origin at the main display's top-left corner), pixels of
// the virtual screen on Windows and X11 (X and Y may be negative).
type Display struct {
	Index   int     `json:"index"` // 1 = main, then left-to-right, top-to-bottom
	ID      string  `json:"id"`    // OS handle: CGDirectDisplayID, \\.\DISPLAY1, an XRandR output; may change after a reboot
	Key     string  `json:"key"`   // durable: ColorSync UUID, monitor device id or EDID hash
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

// Select picks a display by "" or "1"/"2"… (index) or a key prefix.
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
