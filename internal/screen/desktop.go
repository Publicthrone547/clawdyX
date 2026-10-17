// Package screen gives Claude eyes and hands on the Mac's displays:
// screenshots, mouse clicks, typing, key presses and scrolling. It is served
// to Claude Code as an MCP server (`clawdyx mcp-screen`) and is only
// enabled for automation-tier sessions when desktop.screen_control is true.
//
// macOS permissions are required and can only be granted by the owner in
// System Settings → Privacy & Security: Screen Recording (screenshots) and
// Accessibility (mouse/keyboard), for the app that started the bot.
package screen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ErrNoScreenRecording means macOS refused the screenshot.
var ErrNoScreenRecording = errors.New("macOS Screen Recording permission is missing for the app that runs clawdyx (System Settings → Privacy & Security → Screen & System Audio Recording)")

// ErrNoAccessibility means macOS will ignore synthetic input.
var ErrNoAccessibility = errors.New("macOS Accessibility permission is missing for the app that runs clawdyx (System Settings → Privacy & Security → Accessibility). Mouse and keyboard tools cannot work until the owner grants it; screenshots and shell commands such as open -a Firefox URL still work")

// MaxImageEdge bounds the screenshot's long edge in pixels.
const MaxImageEdge = 1456

// Shot is a screenshot scaled for the model.
type Shot struct {
	JPEG             []byte
	Width, Height    int     // image size in pixels
	ScreenW          float64 // display size in points
	ScreenH          float64
	Scale            float64 // image pixels per screen point
	OriginX, OriginY float64 // display origin in global points (0,0 for the main display)
	Display          Display
}

// Desktop performs the actions. Coordinates are screen points.
type Desktop interface {
	// Screenshot captures one display ("" or "1" = main; see Select).
	Screenshot(ctx context.Context, display string) (Shot, error)
	Click(ctx context.Context, x, y float64, button string, count int) error
	Move(ctx context.Context, x, y float64) error
	Scroll(ctx context.Context, x, y float64, dx, dy int) error
	TypeText(ctx context.Context, text string) error
	PressKey(ctx context.Context, keyCode int, flags uint64) error
	Trusted(ctx context.Context) bool
}

// Mac is the real macOS implementation (screencapture, sips, osascript JXA).
type Mac struct {
	TmpDir string
}

func run(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + os.Getenv("HOME")}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	out, err := cmd.CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		if s == "" {
			s = err.Error()
		}
		return s, errors.New(s)
	}
	return s, nil
}

// jxa runs a fixed JavaScript-for-Automation script; argv is passed as data.
func jxa(ctx context.Context, script string, argv ...string) (string, error) {
	args := append([]string{"-l", "JavaScript", "-e", script, "--"}, argv...)
	return run(ctx, 15*time.Second, "/usr/bin/osascript", args...)
}

const jsTrusted = `function run(argv){ ObjC.import("ApplicationServices"); return String($.AXIsProcessTrusted()) }`

const jsMouse = `function run(argv){
 ObjC.import("CoreGraphics");
 var p={x:parseFloat(argv[1]), y:parseFloat(argv[2])};
 var right = argv[3]==="right";
 var b = right ? $.kCGMouseButtonRight : $.kCGMouseButtonLeft;
 $.CGEventPost($.kCGHIDEventTap, $.CGEventCreateMouseEvent(null, $.kCGEventMouseMoved, p, b));
 if (argv[0]==="move") return "ok";
 if (argv[0]==="scroll") {
   delay(0.05);
   var s=$.CGEventCreateScrollWheelEvent(null, $.kCGScrollEventUnitLine, 2, parseInt(argv[4]), parseInt(argv[5]));
   $.CGEventPost($.kCGHIDEventTap, s); return "ok";
 }
 var n=parseInt(argv[4]);
 var down = right ? $.kCGEventRightMouseDown : $.kCGEventLeftMouseDown;
 var up = right ? $.kCGEventRightMouseUp : $.kCGEventLeftMouseUp;
 delay(0.05);
 for (var i=1;i<=n;i++){
   var d=$.CGEventCreateMouseEvent(null, down, p, b); $.CGEventSetIntegerValueField(d, $.kCGMouseEventClickState, i); $.CGEventPost($.kCGHIDEventTap, d);
   var u=$.CGEventCreateMouseEvent(null, up, p, b); $.CGEventSetIntegerValueField(u, $.kCGMouseEventClickState, i); $.CGEventPost($.kCGHIDEventTap, u);
   delay(0.06);
 }
 return "ok";
}`

const jsKey = `function run(argv){
 ObjC.import("CoreGraphics");
 var code=parseInt(argv[0]), flags=parseInt(argv[1]);
 var d=$.CGEventCreateKeyboardEvent(null, code, true); $.CGEventSetFlags(d, flags); $.CGEventPost($.kCGHIDEventTap, d);
 var u=$.CGEventCreateKeyboardEvent(null, code, false); $.CGEventSetFlags(u, flags); $.CGEventPost($.kCGHIDEventTap, u);
 return "ok";
}`

const jsType = `function run(argv){ var se=Application("System Events"); se.keystroke(argv[0]); return "ok" }`

// Screenshot captures one display and scales it so the long edge is at most
// MaxImageEdge pixels and never above the display's size in points.
func (m Mac) Screenshot(ctx context.Context, display string) (Shot, error) {
	ds, err := m.Displays(ctx)
	if err != nil {
		return Shot{}, err
	}
	d, err := Select(ds, display)
	if err != nil {
		return Shot{}, err
	}
	raw, err := m.CapturePNG(ctx, d)
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

// Trusted reports whether macOS accepts synthetic input from this process.
func (m Mac) Trusted(ctx context.Context) bool {
	out, err := jxa(ctx, jsTrusted)
	return err == nil && out == "true"
}

func (m Mac) needTrust(ctx context.Context) error {
	if !m.Trusted(ctx) {
		return ErrNoAccessibility
	}
	return nil
}

func ff(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

// Click clicks at screen point (x, y).
func (m Mac) Click(ctx context.Context, x, y float64, button string, count int) error {
	if err := m.needTrust(ctx); err != nil {
		return err
	}
	_, err := jxa(ctx, jsMouse, "click", ff(x), ff(y), button, strconv.Itoa(count))
	return err
}

// Move moves the pointer.
func (m Mac) Move(ctx context.Context, x, y float64) error {
	if err := m.needTrust(ctx); err != nil {
		return err
	}
	_, err := jxa(ctx, jsMouse, "move", ff(x), ff(y), "left")
	return err
}

// Scroll scrolls at (x, y); dy > 0 scrolls up, dx > 0 scrolls left (wheel semantics).
func (m Mac) Scroll(ctx context.Context, x, y float64, dx, dy int) error {
	if err := m.needTrust(ctx); err != nil {
		return err
	}
	_, err := jxa(ctx, jsMouse, "scroll", ff(x), ff(y), "left", strconv.Itoa(dy), strconv.Itoa(dx))
	return err
}

// TypeText types text into the focused field (System Events keystroke).
func (m Mac) TypeText(ctx context.Context, text string) error {
	if err := m.needTrust(ctx); err != nil {
		return err
	}
	_, err := jxa(ctx, jsType, text)
	if err != nil && strings.Contains(err.Error(), "-1743") {
		return errors.New("macOS Automation permission for System Events is missing (System Settings → Privacy & Security → Automation)")
	}
	return err
}

// PressKey presses one key with modifier flags.
func (m Mac) PressKey(ctx context.Context, keyCode int, flags uint64) error {
	if err := m.needTrust(ctx); err != nil {
		return err
	}
	_, err := jxa(ctx, jsKey, strconv.Itoa(keyCode), strconv.FormatUint(flags, 10))
	return err
}

// Modifier flag masks (CGEventFlags).
var Modifiers = map[string]uint64{
	"cmd": 0x100000, "command": 0x100000, "shift": 0x20000, "option": 0x80000, "alt": 0x80000, "ctrl": 0x40000, "control": 0x40000,
}

// KeyCodes maps key names to macOS virtual key codes (ANSI layout).
var KeyCodes = map[string]int{
	"return": 36, "enter": 36, "tab": 48, "space": 49, "delete": 51, "backspace": 51, "escape": 53, "esc": 53,
	"forwarddelete": 117, "home": 115, "end": 119, "pageup": 116, "pagedown": 121,
	"left": 123, "right": 124, "down": 125, "up": 126,
	"a": 0, "s": 1, "d": 2, "f": 3, "h": 4, "g": 5, "z": 6, "x": 7, "c": 8, "v": 9, "b": 11, "q": 12, "w": 13, "e": 14, "r": 15,
	"y": 16, "t": 17, "1": 18, "2": 19, "3": 20, "4": 21, "6": 22, "5": 23, "=": 24, "9": 25, "7": 26, "-": 27, "8": 28, "0": 29,
	"]": 30, "o": 31, "u": 32, "[": 33, "i": 34, "p": 35, "l": 37, "j": 38, "'": 39, "k": 40, ";": 41, "\\": 42, ",": 43, "/": 44,
	"n": 45, "m": 46, ".": 47, "`": 50,
	"f1": 122, "f2": 120, "f3": 99, "f4": 118, "f5": 96, "f6": 97, "f7": 98, "f8": 100, "f9": 101, "f10": 109, "f11": 103, "f12": 111,
}

// ParseKey parses "cmd+shift+t", "return", "ctrl+c".
func ParseKey(combo string) (int, uint64, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(combo)), "+")
	if len(parts) == 0 || len(parts) > 5 {
		return 0, 0, fmt.Errorf("invalid key %q", combo)
	}
	var flags uint64
	for _, p := range parts[:len(parts)-1] {
		f, ok := Modifiers[strings.TrimSpace(p)]
		if !ok {
			return 0, 0, fmt.Errorf("unknown modifier %q (use cmd, shift, option, ctrl)", p)
		}
		flags |= f
	}
	k := strings.TrimSpace(parts[len(parts)-1])
	code, ok := KeyCodes[k]
	if !ok {
		return 0, 0, fmt.Errorf("unknown key %q", k)
	}
	return code, flags, nil
}
