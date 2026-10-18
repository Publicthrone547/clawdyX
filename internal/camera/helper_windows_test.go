package camera

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeHelperCmd imitates clawdyx-camera.exe's command line and output.
const fakeHelperCmd = `@echo off
if "%1"=="list" echo {"ok":true,"cameras":[{"id":"usb-046d","name":"HD Webcam","builtin":true}]}& exit /b 0
if "%1"=="preflight" echo {"ok":true,"status":"denied"}& exit /b 0
if "%1"=="photo" (
  if not "%2"=="usb-046d" echo {"ok":false,"error":"no such camera"}& exit /b 4
  echo JPEG> "%~3"
  echo {"ok":true,"bytes":4}
  exit /b 0
)
if "%1"=="video" echo {"ok":false,"error":"camera permission missing (denied)"}& exit /b 3
exit /b 2
`

func TestWindowsHelperProtocol(t *testing.T) {
	dir := t.TempDir()
	h := filepath.Join(dir, "fake.cmd")
	if err := os.WriteFile(h, []byte(fakeHelperCmd), 0o755); err != nil {
		t.Fatal(err)
	}
	p := HelperCamera{TmpDir: t.TempDir(), Path: h}
	ctx := context.Background()
	cs, err := p.Cameras(ctx)
	if err != nil || len(cs) != 1 || cs[0].Name != "HD Webcam" || cs[0].Index != 1 {
		t.Fatalf("list: %v %+v", err, cs)
	}
	if st, err := p.Preflight(ctx); err != nil || st != "denied" {
		t.Fatalf("preflight: %q %v", st, err)
	}
	b, err := p.Photo(ctx, cs[0])
	if err != nil || len(b) == 0 {
		t.Fatalf("photo: %v", err)
	}
	if _, err := p.Photo(ctx, Camera{ID: "other"}); !errors.Is(err, ErrNoSuchCamera) {
		t.Fatalf("unknown camera: %v", err)
	}
	if _, err := p.Video(ctx, cs[0], 2*time.Second); !errors.Is(err, ErrCameraDenied) {
		t.Fatalf("denied: %v", err)
	}
	// The private capture folders are gone afterwards.
	if left, _ := filepath.Glob(filepath.Join(p.TmpDir, "cxcam*")); len(left) != 0 {
		t.Fatalf("left behind: %v", left)
	}
}

func TestFindHelperOnWindows(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "clawdyx.exe")
	if FindHelper(exe) != "" {
		t.Fatal("no helper yet")
	}
	os.WriteFile(filepath.Join(dir, "clawdyx-camera.exe"), []byte("MZ"), 0o644)
	if got := FindHelper(exe); got != filepath.Join(dir, "clawdyx-camera.exe") {
		t.Fatal(got)
	}
}
