//go:build amd64 || arm64

package camera

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unsafe"
)

// The kernel's sizes on 64-bit Linux; a wrong layout fails to compile.
var (
	_ [unsafe.Sizeof(v4l2Capability{}) - 104]struct{}
	_ [104 - unsafe.Sizeof(v4l2Capability{})]struct{}
	_ [unsafe.Sizeof(v4l2Format{}) - 208]struct{}
	_ [208 - unsafe.Sizeof(v4l2Format{})]struct{}
	_ [unsafe.Sizeof(v4l2Buffer{}) - 88]struct{}
	_ [88 - unsafe.Sizeof(v4l2Buffer{})]struct{}
)

func TestIoctlNumbers(t *testing.T) {
	want := map[string][2]uintptr{
		"QUERYCAP": {vidiocQueryCap, 0x80685600},
		"ENUM_FMT": {vidiocEnumFmt, 0xc0405602},
		"S_FMT":    {vidiocSFmt, 0xc0d05605},
		"REQBUFS":  {vidiocReqBufs, 0xc0145608},
		"QUERYBUF": {vidiocQueryBuf, 0xc0585609},
		"QBUF":     {vidiocQBuf, 0xc058560f},
		"DQBUF":    {vidiocDQBuf, 0xc0585611},
		"STREAMON": {vidiocStreamOn, 0x40045612},
	}
	for name, v := range want {
		if v[0] != v[1] {
			t.Errorf("VIDIOC_%s = %#x, want %#x", name, v[0], v[1])
		}
	}
}

func TestNoCameraIsAnEmptyList(t *testing.T) {
	v := V4L2{Dev: t.TempDir(), TmpDir: t.TempDir()}
	cs, err := v.Cameras(context.Background())
	if err != nil || len(cs) != 0 {
		t.Fatalf("cameras %v, %v", cs, err)
	}
	if _, err := v.Photo(context.Background(), Camera{ID: "/etc/passwd"}); err == nil {
		t.Fatal("a path outside the device folder was opened")
	}
}

// On Linux clips land in a private folder under the state folder and are
// gone after the call.
func TestClipFolderIsPrivateAndRemoved(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "state", "tmp")
	v, ok := NewProvider("", tmp).(V4L2)
	if !ok || v.TmpDir != tmp {
		t.Fatalf("provider %#v", v)
	}
	if st, err := os.Stat(tmp); err != nil || st.Mode().Perm() != 0o700 {
		t.Fatalf("capture folder %v %v", st, err)
	}
	v.Dev = t.TempDir()
	dev := filepath.Join(v.Dev, "video0")
	if err := os.WriteFile(dev, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	// The fake ffmpeg checks that its output folder is private, then writes.
	script := "#!/bin/sh\nfor a; do out=$a; done\n[ \"$(stat -c %a \"$(dirname \"$out\")\")\" = 700 ] || exit 3\nprintf mp4 > \"$out\"\n"
	if err := os.WriteFile(filepath.Join(bin, "ffmpeg"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	b, err := v.Video(context.Background(), Camera{ID: dev}, 2*time.Second)
	if err != nil || string(b) != "mp4" {
		t.Fatalf("video %q, %v", b, err)
	}
	if left, _ := os.ReadDir(tmp); len(left) != 0 {
		t.Fatalf("left behind: %v", left)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := v.Video(context.Background(), Camera{ID: dev}, time.Second); err != ErrNoVideoTool {
		t.Fatalf("without ffmpeg: %v", err)
	}
}
