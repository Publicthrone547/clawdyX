// Package camera takes photos and short clips with the computer's cameras
// for the owner's /camera commands. It mirrors the screen package: one
// broker gates every capture (config switch, the runtime switch shared with
// the screens, an audit row per capture) and holds media in memory only, for
// at most HoldTTL. Claude gets no camera tool.
//
// Each OS has its own Provider (NewProvider): on macOS the bundled
// AVFoundation helper (clawdyx-camera, inside clawdyX.app), which asks for
// the Camera permission once.
package camera

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/platform"
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
	ID      string `json:"id"`    // the OS's device id (AVCaptureDevice uniqueID, symbolic link, /dev/videoN)
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
	// ErrCameraDenied means the OS refused camera access; providers return
	// their own explanation through Denied, errors.Is matches it.
	ErrCameraDenied = errors.New("camera access was refused")
	ErrNoSuchCamera = errors.New("no such camera")
	ErrNoHelper     = errors.New("camera capture needs the clawdyX app (its camera helper); the standalone worker cannot use the camera")
)

type deniedError struct{ msg string }

func (e deniedError) Error() string        { return e.msg }
func (e deniedError) Is(target error) bool { return target == ErrCameraDenied }

// Denied is an ErrCameraDenied with an OS-specific explanation.
func Denied(msg string) error { return deniedError{msg} }

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

// HelperName is the camera helper's file name on this OS.
const HelperName = "clawdyx-camera" + platform.ExeSuffix

// FindHelper returns the helper next to exe, or "".
func FindHelper(exe string) string {
	p := filepath.Join(filepath.Dir(exe), HelperName)
	st, err := os.Stat(p)
	if err != nil || !st.Mode().IsRegular() {
		return ""
	}
	if platform.ExeSuffix == "" && st.Mode().Perm()&0o111 == 0 {
		return ""
	}
	return p
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
