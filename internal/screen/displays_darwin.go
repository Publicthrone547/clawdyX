package screen

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

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
		ds = append(ds, Display{ID: strconv.FormatUint(uint64(r.ID), 10), Key: key, Name: name, X: r.X, Y: r.Y, W: r.W, H: r.H, PixelW: r.PW, PixelH: r.PH, Scale: r.Scale, Main: r.Main, Builtin: r.Builtin})
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
		out, err := run(ctx, 20*time.Second, m.Helper, "capture", d.ID, f)
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
