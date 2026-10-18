package screen

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ErrNoScreenRecording means macOS refused the screenshot.
var ErrNoScreenRecording = Denied("macOS Screen Recording permission is missing for the app that runs clawdyx (System Settings → Privacy & Security → Screen & System Audio Recording)")

// ErrNoAccessibility means macOS will ignore synthetic input.
var ErrNoAccessibility = errors.New("macOS Accessibility permission is missing for the app that runs clawdyx (System Settings → Privacy & Security → Accessibility). Mouse and keyboard tools cannot work until the owner grants it; screenshots and shell commands such as open -a Firefox URL still work")

// Mac is the macOS implementation (screencapture or the bundled
// ScreenCaptureKit helper, osascript JXA). Geometry is in points.
type Mac struct {
	TmpDir string
	// Helper is the path of clawdyx-screenshot (inside clawdyX.app); empty
	// means screencapture is used.
	Helper string
}

// HelperName is the ScreenCaptureKit helper's file name.
const HelperName = "clawdyx-screenshot"

// FindHelper returns the helper next to exe, or "".
func FindHelper(exe string) string {
	p := filepath.Join(filepath.Dir(exe), HelperName)
	if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() && st.Mode().Perm()&0o111 != 0 {
		return p
	}
	return ""
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

// Screenshot captures one display, scaled for the model.
func (m Mac) Screenshot(ctx context.Context, display string) (Shot, error) {
	return screenshot(ctx, m, display)
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

// PressKey presses one key with modifiers ("cmd+shift+t", "return").
func (m Mac) PressKey(ctx context.Context, combo string) error {
	code, flags, err := ParseKey(combo)
	if err != nil {
		return err
	}
	if err := m.needTrust(ctx); err != nil {
		return err
	}
	_, err = jxa(ctx, jsKey, strconv.Itoa(code), strconv.FormatUint(flags, 10))
	return err
}
