package telegram

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/camera"
	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/control"
)

// fakeCamera is the camera backend for tests: two cameras, counted captures.
type fakeCamera struct {
	shots  *atomic.Int64
	denied bool
}

func (f fakeCamera) Cameras(context.Context) ([]camera.Camera, error) {
	return camera.Order([]camera.Camera{{ID: "usb-1", Name: "USB Cam"}, {ID: "fhd-0", Name: "FaceTime HD Camera", Builtin: true}}), nil
}

func (f fakeCamera) Photo(context.Context, camera.Camera) ([]byte, error) {
	f.shots.Add(1)
	if f.denied {
		return nil, camera.ErrNoCameraAccess
	}
	return []byte{0xff, 0xd8, 0xff, 0xd9}, nil
}

func (f fakeCamera) Video(_ context.Context, _ camera.Camera, d time.Duration) ([]byte, error) {
	f.shots.Add(1)
	return []byte("mp4:" + d.String()), nil
}

func cameraOn(c *config.Config) { c.Desktop.Camera = true }

func startCam(t *testing.T, mut func(*config.Config), cam fakeCamera) *harness {
	t.Helper()
	return startOpts(t, mut, Options{Desktop: fakeDesk{}, Cameras: cam})
}

func sentMedia(h *harness, method string) []call {
	var out []call
	for _, c := range h.tg.sent() {
		if c.Method == method {
			out = append(out, c)
		}
	}
	return out
}

func waitMedia(t *testing.T, h *harness, method string, n int) []call {
	t.Helper()
	for i := 0; i < 200; i++ {
		if p := sentMedia(h, method); len(p) >= n {
			return p
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("wanted %d %s, got %d", n, method, len(sentMedia(h, method)))
	return nil
}

func TestCameraDisabledInConfig(t *testing.T) {
	shots := &atomic.Int64{}
	h := startCam(t, nil, fakeCamera{shots: shots})
	for _, cmd := range []string{"/camera", "/camera video 5", "/cameras"} {
		h.tg.push(msg(owner, owner, "private", cmd))
	}
	h.tg.waitFor(t, "The camera is off")
	time.Sleep(300 * time.Millisecond)
	if shots.Load() != 0 || len(sentMedia(h, "sendPhoto"))+len(sentMedia(h, "sendVideo")) != 0 {
		t.Fatal("captured while disabled in config")
	}
}

func TestCameraPhotoVideoAuditAndSwitch(t *testing.T) {
	shots := &atomic.Int64{}
	h := startCam(t, cameraOn, fakeCamera{shots: shots})
	time.Sleep(300 * time.Millisecond)
	if shots.Load() != 0 {
		t.Fatal("something was captured at startup")
	}
	h.tg.push(msg(42, 42, "private", "/camera")) // stranger
	h.tg.push(msg(owner, owner, "private", "/cameras"))
	h.tg.waitFor(t, "1 · FaceTime HD Camera · built-in")
	h.tg.waitFor(t, "2 · USB Cam")
	if shots.Load() != 0 {
		t.Fatal("/cameras must not capture")
	}

	h.tg.push(msg(owner, owner, "private", "/camera"))
	p := waitMedia(t, h, "sendPhoto", 1)
	if !strings.Contains(p[0].Form["caption"], "Camera 1 · FaceTime HD Camera") || p[0].Form["chat_id"] != fmt.Sprint(owner) {
		t.Fatalf("photo %v", p[0].Form)
	}
	h.tg.push(msg(owner, owner, "private", "/camera 2"))
	p = waitMedia(t, h, "sendPhoto", 2)
	if !strings.Contains(p[1].Form["caption"], "Camera 2 · USB Cam") {
		t.Fatalf("photo 2 %v", p[1].Form)
	}

	h.tg.push(msg(owner, owner, "private", "/camera video 10"))
	h.tg.waitFor(t, "Recording 10 s")
	v := waitMedia(t, h, "sendVideo", 1)
	if v[0].Form["chat_id"] != fmt.Sprint(owner) || v[0].Form["duration"] != "10" || !strings.Contains(v[0].Form["caption"], "10 s") {
		t.Fatalf("video %v", v[0].Form)
	}
	h.tg.push(msg(owner, owner, "private", "/camera 2 video 3"))
	waitMedia(t, h, "sendVideo", 2)

	n := shots.Load()
	h.tg.push(msg(owner, owner, "private", "/camera video 61"))
	h.tg.waitFor(t, "at most 60 seconds (you asked for 61)")
	h.tg.push(msg(owner, owner, "private", "/camera video"))
	h.tg.waitFor(t, "give the clip length in seconds")
	h.tg.push(msg(owner, owner, "private", "/camera 7"))
	h.tg.waitFor(t, "no such camera")
	if shots.Load() != n {
		t.Fatal("a rejected request reached the camera")
	}

	rows, err := h.st.RecentCaptures(10)
	if err != nil || len(rows) != 4 {
		t.Fatalf("audit rows %v %+v", err, rows)
	}
	if r := rows[1]; r.Kind != "camera-video" || r.DurationMS != 10000 || r.DisplayKey != "camera:fhd-0" || r.Destination != "telegram" || r.Requester != "telegram-owner" {
		t.Fatalf("video audit %+v", r)
	}
	if r := rows[3]; r.Kind != "camera-photo" || r.Bytes != 4 {
		t.Fatalf("photo audit %+v", r)
	}
	// Media is dropped once the upload returned (the fake server records
	// the call slightly earlier).
	for i := 0; i < 100 && h.app.cams.Held() != 0; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if h.app.cams.Held() != 0 {
		t.Fatal("media kept in memory after sending")
	}

	// /screen off switches the camera off too; only the Mac switches it back on.
	h.tg.push(msg(owner, owner, "private", "/screen off"))
	h.tg.waitFor(t, "Screen and camera features switched off")
	n = shots.Load()
	h.tg.push(msg(owner, owner, "private", "/camera"))
	h.tg.waitFor(t, "Screen and camera features are switched off (/screen off)")
	if shots.Load() != n {
		t.Fatal("captured while switched off")
	}
	c := withControl(t, h)
	var st control.ScreenStatus
	if err := c.Call(context.Background(), "screens.enabled.set", control.EnabledParams{Enabled: true}, &st); err != nil {
		t.Fatalf("enable with only the camera configured: %v", err)
	}
	h.tg.push(msg(owner, owner, "private", "/camera"))
	waitMedia(t, h, "sendPhoto", 3)
}

func TestCameraPermissionMissingIsExplained(t *testing.T) {
	h := startCam(t, cameraOn, fakeCamera{shots: &atomic.Int64{}, denied: true})
	h.tg.push(msg(owner, owner, "private", "/camera"))
	h.tg.waitFor(t, "macOS Camera permission is missing")
	h.tg.waitFor(t, "Privacy &amp; Security → Camera")
	if len(sentMedia(h, "sendPhoto")) != 0 {
		t.Fatal("photo sent without permission")
	}
}

func TestMemberCannotUseCamera(t *testing.T) {
	shots := &atomic.Int64{}
	h := startCam(t, func(c *config.Config) { twoUsers(c); cameraOn(c) }, fakeCamera{shots: shots})
	for _, cmd := range []string{"/camera", "/camera video 5", "/cameras"} {
		h.tg.push(msg(member, member, "private", cmd))
	}
	for i := 0; i < 40; i++ {
		n := 0
		for _, c := range h.tg.sent() {
			if c.Method == "sendMessage" && c.Form["chat_id"] == fmt.Sprint(member) && strings.Contains(c.Form["text"], "Only the owner can do that") {
				n++
			}
		}
		if n == 3 {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if !sentTo(h, member, "Only the owner can do that") || shots.Load() != 0 {
		t.Fatal("a member used the camera")
	}
}

func TestParseCameraArgs(t *testing.T) {
	for _, tc := range []struct {
		in    string
		sel   string
		video bool
		secs  int
		bad   bool
	}{
		{"", "", false, 0, false}, {"2", "2", false, 0, false}, {"video 10", "", true, 10, false},
		{"video 10s 2", "2", true, 10, false}, {"2 video 5", "2", true, 5, false},
		{"video 61", "", false, 0, true}, {"video 0", "", false, 0, true}, {"video x", "", false, 0, true},
		{"video", "", false, 0, true}, {"selfie", "", false, 0, true}, {"1 2", "", false, 0, true},
	} {
		sel, video, secs, err := parseCameraArgs(strings.Fields(tc.in))
		if tc.bad != (err != nil) || (!tc.bad && (sel != tc.sel || video != tc.video || secs != tc.secs)) {
			t.Errorf("%q → %q %v %d %v", tc.in, sel, video, secs, err)
		}
	}
}

// preflightCamera also reports the permission like the real helper.
type preflightCamera struct {
	fakeCamera
	status string
}

func (p preflightCamera) Preflight(context.Context) (string, error) { return p.status, nil }

func TestCameraDiagnostics(t *testing.T) {
	if !camera.Supported {
		t.Skip("no camera backend on this OS yet")
	}
	ctx := context.Background()
	shots := &atomic.Int64{}
	off := startCam(t, nil, fakeCamera{shots: shots})
	if c := off.app.cameraCheck(ctx); !c.OK || !strings.Contains(c.Detail, "off") {
		t.Fatalf("off: %+v", c)
	}
	for _, tc := range []struct {
		status, want string
		ok, warn     bool
	}{
		{"authorized", "permission granted · 2 found (FaceTime HD Camera, USB Cam)", true, false},
		{"denied", "permission not granted", false, false},
		{"notDetermined", "permission not asked yet", false, true},
	} {
		h := startOpts(t, cameraOn, Options{Desktop: fakeDesk{}, Cameras: preflightCamera{fakeCamera{shots: shots}, tc.status}})
		c := h.app.cameraCheck(ctx)
		if c.OK != tc.ok || c.Warn != tc.warn || !strings.HasPrefix(c.Detail, tc.want) {
			t.Fatalf("%s: %+v", tc.status, c)
		}
		if !tc.ok && c.Fix == "" {
			t.Fatalf("%s: no fix offered", tc.status)
		}
	}
	h := startOpts(t, cameraOn, Options{Desktop: fakeDesk{}, Cameras: preflightCamera{fakeCamera{shots: shots}, "authorized"}})
	h.app.SetScreenEnabled(false)
	if c := h.app.cameraCheck(ctx); !strings.HasSuffix(c.Detail, "switched off (/screen off)") {
		t.Fatalf("switched off: %+v", c)
	}
	if shots.Load() != 0 {
		t.Fatal("the diagnostics check captured")
	}
}
