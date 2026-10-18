package camera

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// Supported reports whether this OS has a camera backend in this build.
const Supported = true

// ErrUnsupported is returned where there is no camera backend.
var ErrUnsupported = errors.New("camera capture is not available on this computer")

// ErrNoCameraAccess means the camera device could not be opened.
var ErrNoCameraAccess = Denied("no access to the camera device: add your user to the video group (sudo usermod -aG video $USER), then log in again")

// ErrNoVideoTool means ffmpeg, which records clips on Linux, is missing.
var ErrNoVideoTool = errors.New("clips need ffmpeg: install it (for example sudo apt install ffmpeg), photos work without it")

const timeoutHint = "(another program may be using the camera)"

// NewProvider returns this OS's camera provider: V4L2, with clips in a
// private folder under tmpDir (the state folder) or the system's.
func NewProvider(_ string, tmpDir string) Provider {
	if tmpDir == "" {
		tmpDir = os.TempDir()
	} else {
		_ = os.MkdirAll(tmpDir, 0o700)
	}
	return V4L2{TmpDir: tmpDir}
}

// FfmpegArgs is the ffmpeg command line for a clip; size "" leaves the
// camera's own size and frame rate.
func FfmpegArgs(dev string, secs int, size, out string) []string {
	args := []string{"-nostdin", "-loglevel", "error", "-f", "v4l2"}
	if size != "" {
		args = append(args, "-framerate", "30", "-video_size", size)
	}
	return append(args, "-i", dev, "-t", strconv.Itoa(secs), "-an",
		"-c:v", "libx264", "-preset", "veryfast", "-b:v", "2500k", "-pix_fmt", "yuv420p",
		"-movflags", "+faststart", "-y", out)
}

// Video implements Provider through ffmpeg.
func (v V4L2) Video(ctx context.Context, c Camera, d time.Duration) ([]byte, error) {
	dev, err := v.device(c)
	if err != nil {
		return nil, err
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, ErrNoVideoTool
	}
	secs := min(max(int(d.Round(time.Second)/time.Second), 1), MaxVideoSeconds)
	dir, err := os.MkdirTemp(v.TmpDir, "clawdyx-clip-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "clip.mp4")
	var last error
	for _, size := range []string{"1280x720", ""} {
		cctx, cancel := context.WithTimeout(ctx, time.Duration(secs)*time.Second+60*time.Second)
		cmd := exec.CommandContext(cctx, ffmpeg, FfmpegArgs(dev, secs, size, out)...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err := cmd.Run()
		timedOut := cctx.Err() != nil
		cancel()
		if err == nil {
			b, err := os.ReadFile(out)
			if err != nil {
				return nil, err
			}
			if len(b) == 0 {
				return nil, errors.New("ffmpeg recorded an empty clip")
			}
			if len(b) > MaxVideoBytes {
				return nil, errors.New("the clip is too large to send")
			}
			return b, nil
		}
		if timedOut {
			return nil, errors.New("the camera timed out " + timeoutHint)
		}
		msg := lastLine(stderr.String())
		if _, err := queryCap(dev); err != nil {
			return nil, devErr(err)
		}
		last = fmt.Errorf("ffmpeg could not record (%s)", msg)
	}
	return nil, last
}
