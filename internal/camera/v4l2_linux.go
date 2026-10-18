package camera

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// The V4L2 ioctls and structs clawdyX needs (linux/videodev2.h). Struct
// layouts follow the kernel's for the build's word size.

const (
	capVideoCapture = 0x00000001
	capStreaming    = 0x04000000
	capDeviceCaps   = 0x80000000
	bufTypeCapture  = 1
	memoryMMap      = 1
	fieldAny        = 0
)

func fourcc(s string) uint32 {
	return uint32(s[0]) | uint32(s[1])<<8 | uint32(s[2])<<16 | uint32(s[3])<<24
}

var (
	fmtYUYV = fourcc("YUYV")
	fmtMJPG = fourcc("MJPG")
)

type v4l2Capability struct {
	Driver       [16]byte
	Card         [32]byte
	BusInfo      [32]byte
	Version      uint32
	Capabilities uint32
	DeviceCaps   uint32
	Reserved     [3]uint32
}

type v4l2FmtDesc struct {
	Index       uint32
	Type        uint32
	Flags       uint32
	Description [32]byte
	PixelFormat uint32
	MbusCode    uint32
	Reserved    [3]uint32
}

type v4l2PixFormat struct {
	Width, Height, PixelFormat, Field, BytesPerLine, SizeImage uint32
	Colorspace, Priv, Flags, YcbcrEnc, Quantization, XferFunc  uint32
}

type v4l2Format struct {
	Type uint32
	Fmt  struct {
		_   [0]uintptr // the union holds pointers, so it is word-aligned (first: a last zero-size field adds padding)
		Pix v4l2PixFormat
		_   [200 - unsafe.Sizeof(v4l2PixFormat{})]byte
	}
}

type v4l2RequestBuffers struct {
	Count, Type, Memory, Capabilities uint32
	Flags                             uint8
	Reserved                          [3]uint8
}

type v4l2Timecode struct {
	Type, Flags                     uint32
	Frames, Seconds, Minutes, Hours uint8
	UserBits                        [4]uint8
}

type v4l2Buffer struct {
	Index, Type, BytesUsed, Flags, Field uint32
	Timestamp                            unix.Timeval
	Timecode                             v4l2Timecode
	Sequence, Memory                     uint32
	M                                    uintptr // offset for MMAP buffers
	Length, Reserved2, RequestFD         uint32
}

func ioc(dir, nr, size uintptr) uintptr { return dir<<30 | size<<16 | 'V'<<8 | nr }

const (
	iocWrite = 1
	iocRead  = 2
)

var (
	vidiocQueryCap  = ioc(iocRead, 0, unsafe.Sizeof(v4l2Capability{}))
	vidiocEnumFmt   = ioc(iocRead|iocWrite, 2, unsafe.Sizeof(v4l2FmtDesc{}))
	vidiocSFmt      = ioc(iocRead|iocWrite, 5, unsafe.Sizeof(v4l2Format{}))
	vidiocReqBufs   = ioc(iocRead|iocWrite, 8, unsafe.Sizeof(v4l2RequestBuffers{}))
	vidiocQueryBuf  = ioc(iocRead|iocWrite, 9, unsafe.Sizeof(v4l2Buffer{}))
	vidiocQBuf      = ioc(iocRead|iocWrite, 15, unsafe.Sizeof(v4l2Buffer{}))
	vidiocDQBuf     = ioc(iocRead|iocWrite, 17, unsafe.Sizeof(v4l2Buffer{}))
	vidiocStreamOn  = ioc(iocWrite, 18, unsafe.Sizeof(int32(0)))
	vidiocStreamOff = ioc(iocWrite, 19, unsafe.Sizeof(int32(0)))
)

func ioctl(fd int, req uintptr, arg unsafe.Pointer) error {
	for {
		_, _, e := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), req, uintptr(arg))
		if e == unix.EINTR {
			continue
		}
		if e != 0 {
			return e
		}
		return nil
	}
}

func cstr(b []byte) string {
	if i := strings.IndexByte(string(b), 0); i >= 0 {
		b = b[:i]
	}
	return strings.TrimSpace(string(b))
}

// devErr turns an open or ioctl error into the message the owner sees.
func devErr(err error) error {
	switch {
	case errors.Is(err, unix.EACCES), errors.Is(err, unix.EPERM):
		return ErrNoCameraAccess
	case errors.Is(err, unix.EBUSY):
		return errors.New("the camera is busy " + timeoutHint)
	case errors.Is(err, unix.ENOENT), errors.Is(err, unix.ENODEV):
		return errors.New("the camera was disconnected")
	}
	return err
}

// V4L2 is the Linux camera provider: photos straight from /dev/video*
// (pure Go), clips through ffmpeg when it is installed. Clips land briefly
// in a private folder under TmpDir.
type V4L2 struct {
	TmpDir string
	// Dev is the device folder (tests point it elsewhere).
	Dev string
}

func (v V4L2) dev() string {
	if v.Dev != "" {
		return v.Dev
	}
	return "/dev"
}

// devices returns /dev/video* in numeric order.
func (v V4L2) devices() []string {
	paths, _ := filepath.Glob(filepath.Join(v.dev(), "video[0-9]*"))
	num := func(p string) int { n, _ := strconv.Atoi(strings.TrimPrefix(filepath.Base(p), "video")); return n }
	sort.Slice(paths, func(i, j int) bool { return num(paths[i]) < num(paths[j]) })
	return paths
}

func queryCap(path string) (v4l2Capability, error) {
	var c v4l2Capability
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return c, err
	}
	defer unix.Close(fd)
	err = ioctl(fd, vidiocQueryCap, unsafe.Pointer(&c))
	return c, err
}

// builtin: a camera on a USB port that is not removable (laptop webcams).
func builtin(path string) bool {
	b, err := os.ReadFile(filepath.Join("/sys/class/video4linux", filepath.Base(path), "device", "..", "removable"))
	return err == nil && strings.TrimSpace(string(b)) == "fixed"
}

// Cameras implements Provider: capture nodes, one per device (a webcam
// often has a second node for metadata).
func (v V4L2) Cameras(context.Context) ([]Camera, error) {
	var out []Camera
	seen := map[string]bool{}
	denied := false
	for _, p := range v.devices() {
		c, err := queryCap(p)
		if err != nil {
			if errors.Is(err, unix.EACCES) || errors.Is(err, unix.EPERM) {
				denied = true
			}
			continue
		}
		caps := c.Capabilities
		if caps&capDeviceCaps != 0 {
			caps = c.DeviceCaps
		}
		if caps&capVideoCapture == 0 || caps&capStreaming == 0 {
			continue
		}
		bus := cstr(c.BusInfo[:])
		if bus != "" && seen[bus] {
			continue
		}
		seen[bus] = true
		name := cstr(c.Card[:])
		if name == "" {
			name = filepath.Base(p)
		}
		out = append(out, Camera{ID: p, Name: name, Builtin: builtin(p)})
	}
	if len(out) == 0 && denied {
		return nil, ErrNoCameraAccess
	}
	return Order(out), nil
}

// Preflight implements Preflighter: a device that cannot be opened for
// lack of permission is "denied".
func (v V4L2) Preflight(context.Context) (string, error) {
	devs := v.devices()
	if len(devs) == 0 {
		return "", errors.New("no camera device")
	}
	for _, p := range devs {
		if _, err := queryCap(p); err == nil {
			return "authorized", nil
		}
	}
	return "denied", nil
}

func (v V4L2) device(c Camera) (string, error) {
	if !strings.HasPrefix(c.ID, filepath.Join(v.dev(), "video")) {
		return "", fmt.Errorf("%w: %q", ErrNoSuchCamera, c.ID)
	}
	return c.ID, nil
}

// warmupFrames are dropped so exposure and white balance can settle.
const warmupFrames = 8

// Photo implements Provider: YUYV frames are encoded here; MJPEG frames
// are used as they are when they decode.
func (v V4L2) Photo(ctx context.Context, c Camera) ([]byte, error) {
	path, err := v.device(c)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, devErr(err)
	}
	defer unix.Close(fd)

	pixfmt := uint32(0)
	for i := uint32(0); i < 64; i++ {
		d := v4l2FmtDesc{Index: i, Type: bufTypeCapture}
		if ioctl(fd, vidiocEnumFmt, unsafe.Pointer(&d)) != nil {
			break
		}
		if d.PixelFormat == fmtYUYV {
			pixfmt = fmtYUYV
			break
		}
		if d.PixelFormat == fmtMJPG {
			pixfmt = fmtMJPG
		}
	}
	if pixfmt == 0 {
		return nil, errors.New("the camera offers neither YUYV nor MJPEG frames")
	}
	f := v4l2Format{Type: bufTypeCapture}
	f.Fmt.Pix = v4l2PixFormat{Width: 1280, Height: 720, PixelFormat: pixfmt, Field: fieldAny}
	if err := ioctl(fd, vidiocSFmt, unsafe.Pointer(&f)); err != nil {
		return nil, devErr(err)
	}
	pix := f.Fmt.Pix
	if pix.PixelFormat != pixfmt {
		return nil, errors.New("the camera refused the picture format")
	}

	frame, err := grab(ctx, fd)
	if err != nil {
		return nil, err
	}
	if pixfmt == fmtMJPG {
		if err := CheckJPEG(frame); err != nil {
			return nil, fmt.Errorf("the camera's JPEG frame does not decode (%v)", err)
		}
		return frame, nil
	}
	return YUYVToJPEG(frame, int(pix.Width), int(pix.Height), int(pix.BytesPerLine), 85)
}

// grab streams through memory-mapped buffers and copies one frame after
// the warm-up.
func grab(ctx context.Context, fd int) ([]byte, error) {
	rb := v4l2RequestBuffers{Count: 4, Type: bufTypeCapture, Memory: memoryMMap}
	if err := ioctl(fd, vidiocReqBufs, unsafe.Pointer(&rb)); err != nil {
		return nil, devErr(err)
	}
	defer func() {
		free := v4l2RequestBuffers{Type: bufTypeCapture, Memory: memoryMMap}
		_ = ioctl(fd, vidiocReqBufs, unsafe.Pointer(&free))
	}()
	if rb.Count == 0 {
		return nil, errors.New("the camera gave no buffers")
	}
	bufs := make([][]byte, rb.Count)
	defer func() {
		for _, b := range bufs {
			if b != nil {
				_ = unix.Munmap(b)
			}
		}
	}()
	for i := range bufs {
		b := v4l2Buffer{Index: uint32(i), Type: bufTypeCapture, Memory: memoryMMap}
		if err := ioctl(fd, vidiocQueryBuf, unsafe.Pointer(&b)); err != nil {
			return nil, devErr(err)
		}
		m, err := unix.Mmap(fd, int64(uint32(b.M)), int(b.Length), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
		if err != nil {
			return nil, err
		}
		bufs[i] = m
		if err := ioctl(fd, vidiocQBuf, unsafe.Pointer(&b)); err != nil {
			return nil, devErr(err)
		}
	}
	typ := int32(bufTypeCapture)
	if err := ioctl(fd, vidiocStreamOn, unsafe.Pointer(&typ)); err != nil {
		return nil, devErr(err)
	}
	defer func() { _ = ioctl(fd, vidiocStreamOff, unsafe.Pointer(&typ)) }()

	for n := 0; ; {
		if ctx.Err() != nil {
			return nil, errors.New("the camera timed out " + timeoutHint)
		}
		pfd := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		if _, err := unix.Poll(pfd, 500); err != nil && !errors.Is(err, unix.EINTR) {
			return nil, err
		}
		if pfd[0].Revents&unix.POLLIN == 0 {
			continue
		}
		b := v4l2Buffer{Type: bufTypeCapture, Memory: memoryMMap}
		if err := ioctl(fd, vidiocDQBuf, unsafe.Pointer(&b)); err != nil {
			if errors.Is(err, unix.EAGAIN) {
				continue
			}
			return nil, devErr(err)
		}
		if n++; n > warmupFrames && b.BytesUsed > 0 && int(b.Index) < len(bufs) {
			used := min(int(b.BytesUsed), len(bufs[b.Index]))
			return append([]byte(nil), bufs[b.Index][:used]...), nil
		}
		if err := ioctl(fd, vidiocQBuf, unsafe.Pointer(&b)); err != nil {
			return nil, devErr(err)
		}
	}
}
