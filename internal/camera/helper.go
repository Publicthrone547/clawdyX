package camera

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// HelperCamera talks to a camera helper program: the AVFoundation helper
// on macOS, the Media Foundation helper on Windows. Both have one command
// line, `list | preflight | photo ID OUT.jpg | video ID SECONDS OUT.mp4`,
// and print one JSON line: {"ok", "error", "status", "cameras"}.
type HelperCamera struct {
	TmpDir string // a private folder under it holds each capture briefly
	Path   string // the helper; "" means there is none (ErrNoHelper)
}

type helperResult struct {
	OK      bool     `json:"ok"`
	Error   string   `json:"error"`
	Status  string   `json:"status"`
	Cameras []Camera `json:"cameras"`
}

func (h HelperCamera) call(ctx context.Context, timeout time.Duration, args ...string) (helperResult, error) {
	var r helperResult
	if h.Path == "" {
		return r, ErrNoHelper
	}
	out, err := run(ctx, timeout, h.Path, args...)
	_ = json.Unmarshal([]byte(lastLine(out)), &r)
	switch {
	case err == nil && r.OK:
		return r, nil
	case strings.Contains(r.Error, "permission"):
		return r, ErrNoCameraAccess
	case r.Error == "no such camera":
		return r, ErrNoSuchCamera
	case ctx.Err() != nil:
		return r, errors.New("the camera timed out " + timeoutHint)
	case r.Error != "":
		return r, errors.New(r.Error)
	case out != "":
		return r, errors.New(out)
	default:
		return r, err
	}
}

// Cameras lists the cameras (no permission needed, nothing captured).
func (h HelperCamera) Cameras(ctx context.Context) ([]Camera, error) {
	r, err := h.call(ctx, 10*time.Second, "list")
	if err != nil {
		return nil, err
	}
	return Order(r.Cameras), nil
}

// Preflight implements Preflighter.
func (h HelperCamera) Preflight(ctx context.Context) (string, error) {
	r, err := h.call(ctx, 10*time.Second, "preflight")
	return r.Status, err
}

// Photo implements Provider. The helper writes into a private temporary
// directory that is removed before this returns.
func (h HelperCamera) Photo(ctx context.Context, c Camera) ([]byte, error) {
	return h.capture(ctx, 60*time.Second, "photo.jpg", func(f string) []string { return []string{"photo", c.ID, f} })
}

// Video implements Provider.
func (h HelperCamera) Video(ctx context.Context, c Camera, d time.Duration) ([]byte, error) {
	secs := int(d / time.Second)
	return h.capture(ctx, d+60*time.Second, "clip.mp4", func(f string) []string { return []string{"video", c.ID, strconv.Itoa(secs), f} })
}

func (h HelperCamera) capture(ctx context.Context, timeout time.Duration, name string, args func(string) []string) ([]byte, error) {
	if h.Path == "" {
		return nil, ErrNoHelper
	}
	dir, err := os.MkdirTemp(h.TmpDir, "cxcam")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	_ = os.Chmod(dir, 0o700)
	f := filepath.Join(dir, name)
	if _, err := h.call(ctx, timeout, args(f)...); err != nil {
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
