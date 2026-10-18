// Package camera takes photos and short clips with the Mac's cameras for
// the owner's /camera commands. It mirrors the screen package: one broker
// gates every capture (config switch, the runtime switch shared with the
// screens, an audit row per capture) and holds media in memory only, for
// at most HoldTTL. Claude gets no camera tool.
//
// Captures go through the bundled AVFoundation helper (clawdyx-camera,
// inside clawdyX.app); macOS asks for the Camera permission once.
package camera

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Limits.
const (
	HoldTTL         = 5 * time.Minute // held media is dropped after this
	HoldMax         = 4
	MaxCameras      = 8
	MaxVideoSeconds = 60
	// MaxVideoBytes stays under the Bot API's 50 MB upload limit.
	MaxVideoBytes = 49 << 20
)

// Camera is one video input device.
type Camera struct {
	Index   int    `json:"index"` // 1 = built-in, then by name
	ID      string `json:"id"`    // AVCaptureDevice uniqueID
	Name    string `json:"name"`
	Builtin bool   `json:"builtin"`
}

// Label is a short description: "1 · FaceTime HD Camera · built-in".
func (c Camera) Label() string {
	s := fmt.Sprintf("%d · %s", c.Index, c.Name)
	if c.Builtin {
		s += " · built-in"
	}
	return s
}

// Errors shown to the owner.
var (
	ErrNoCameraAccess = errors.New("macOS Camera permission is missing for clawdyX (System Settings → Privacy & Security → Camera)")
	ErrNoSuchCamera   = errors.New("no such camera")
	ErrNoHelper       = errors.New("camera capture needs the clawdyX app (its camera helper); the standalone worker cannot use the camera")
)

// Provider enumerates cameras and captures from them.
type Provider interface {
	Cameras(ctx context.Context) ([]Camera, error)
	// Photo returns one JPEG.
	Photo(ctx context.Context, c Camera) ([]byte, error)
	// Video records a clip of d without sound and returns it as MP4.
	Video(ctx context.Context, c Camera, d time.Duration) ([]byte, error)
}

// Preflighter reports the permission without capturing:
// authorized | denied | restricted | notDetermined.
type Preflighter interface {
	Preflight(ctx context.Context) (string, error)
}

// HelperName is the camera helper's file name.
const HelperName = "clawdyx-camera"

// FindHelper returns the helper next to exe, or "".
func FindHelper(exe string) string {
	p := filepath.Join(filepath.Dir(exe), HelperName)
	if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() && st.Mode().Perm()&0o111 != 0 {
		return p
	}
	return ""
}

// Mac is the real implementation (the AVFoundation helper).
type Mac struct {
	TmpDir string
	Helper string
}

type helperResult struct {
	OK      bool     `json:"ok"`
	Error   string   `json:"error"`
	Status  string   `json:"status"`
	Cameras []Camera `json:"cameras"`
}

func (m Mac) call(ctx context.Context, timeout time.Duration, args ...string) (helperResult, error) {
	var r helperResult
	if !supported {
		return r, ErrUnsupported
	}
	if m.Helper == "" {
		return r, ErrNoHelper
	}
	out, err := run(ctx, timeout, m.Helper, args...)
	_ = json.Unmarshal([]byte(lastLine(out)), &r)
	switch {
	case err == nil && r.OK:
		return r, nil
	case strings.Contains(r.Error, "permission"):
		return r, ErrNoCameraAccess
	case r.Error == "no such camera":
		return r, ErrNoSuchCamera
	case ctx.Err() != nil:
		return r, errors.New("the camera timed out (if macOS is asking about camera access, answer it on the Mac)")
	case r.Error != "":
		return r, errors.New(r.Error)
	case out != "":
		return r, errors.New(out)
	default:
		return r, err
	}
}

// Cameras lists the cameras (no permission needed, nothing captured).
func (m Mac) Cameras(ctx context.Context) ([]Camera, error) {
	r, err := m.call(ctx, 10*time.Second, "list")
	if err != nil {
		return nil, err
	}
	return Order(r.Cameras), nil
}

// Preflight implements Preflighter.
func (m Mac) Preflight(ctx context.Context) (string, error) {
	r, err := m.call(ctx, 10*time.Second, "preflight")
	return r.Status, err
}

// Photo implements Provider. The helper writes into a private temporary
// directory that is removed before this returns.
func (m Mac) Photo(ctx context.Context, c Camera) ([]byte, error) {
	return m.capture(ctx, 60*time.Second, "photo.jpg", func(f string) []string { return []string{"photo", c.ID, f} })
}

// Video implements Provider.
func (m Mac) Video(ctx context.Context, c Camera, d time.Duration) ([]byte, error) {
	secs := int(d / time.Second)
	return m.capture(ctx, d+60*time.Second, "clip.mp4", func(f string) []string { return []string{"video", c.ID, strconv.Itoa(secs), f} })
}

func (m Mac) capture(ctx context.Context, timeout time.Duration, name string, args func(string) []string) ([]byte, error) {
	if !supported {
		return nil, ErrUnsupported
	}
	if m.Helper == "" {
		return nil, ErrNoHelper
	}
	dir, err := os.MkdirTemp(m.TmpDir, "cxcam")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	_ = os.Chmod(dir, 0o700)
	f := filepath.Join(dir, name)
	if _, err := m.call(ctx, timeout, args(f)...); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(f)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return nil, errors.New("the camera produced an empty file")
	}
	return b, nil
}

// Order numbers cameras: built-in first, then by name.
func Order(cs []Camera) []Camera {
	out := append([]Camera(nil), cs...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Builtin != out[j].Builtin {
			return out[i].Builtin
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > MaxCameras {
		out = out[:MaxCameras]
	}
	for i := range out {
		out[i].Index = i + 1
	}
	return out
}

// Select picks a camera by "" (the first: built-in) or its number.
func Select(cs []Camera, sel string) (Camera, error) {
	sel = strings.TrimSpace(sel)
	if len(cs) == 0 {
		return Camera{}, errors.New("no camera is connected")
	}
	if sel == "" {
		return cs[0], nil
	}
	if n, err := strconv.Atoi(sel); err == nil {
		for _, c := range cs {
			if c.Index == n {
				return c, nil
			}
		}
	}
	return Camera{}, fmt.Errorf("%w: %q", ErrNoSuchCamera, sel)
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
