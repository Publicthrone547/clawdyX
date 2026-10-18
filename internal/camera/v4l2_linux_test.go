//go:build amd64 || arm64

package camera

import (
	"context"
	"testing"
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
