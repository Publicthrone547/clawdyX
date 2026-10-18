package screen

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/png"
	"sort"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

var dpiOnce sync.Once

// InitDPI makes this process per-monitor DPI aware (V2), so monitor
// rectangles, captures and mouse coordinates are physical pixels on every
// monitor and scaling. Older Windows versions fall back to the system-wide
// awareness. It is called once before the first screen call.
func InitDPI() {
	dpiOnce.Do(func() {
		if procSetProcessDpiAwarenessContext.Find() == nil {
			if r, _, _ := procSetProcessDpiAwarenessContext.Call(dpiPerMonitorAwareV2); r != 0 {
				return
			}
		}
		if procSetProcessDpiAwareness.Find() == nil {
			if r, _, _ := procSetProcessDpiAwareness.Call(2); r == 0 { // PROCESS_PER_MONITOR_DPI_AWARE
				return
			}
		}
		_, _, _ = procSetProcessDPIAware.Call()
	})
}

// rawMonitor is one monitor as Win32 describes it.
type rawMonitor struct {
	Rect     rect
	Primary  bool
	Device   string // \\.\DISPLAY1
	Name     string // the monitor's friendly name, may be empty
	DeviceID string // MONITOR\GSM5B7F\{…}\0001: stable across reboots
	DPI      uint32
}

func enumMonitors() ([]rawMonitor, error) {
	InitDPI()
	var out []rawMonitor
	cb := syscall.NewCallback(func(hmon, hdc, lprc, lparam uintptr) uintptr {
		var mi monitorInfoEx
		mi.Size = uint32(unsafe.Sizeof(mi))
		if r, _, _ := procGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(&mi))); r == 0 {
			return 1
		}
		m := rawMonitor{Rect: mi.Monitor, Primary: mi.Flags&monitorInfoPrimary != 0, Device: wstr(mi.Device[:]), DPI: 96}
		var dd displayDevice
		dd.Size = uint32(unsafe.Sizeof(dd))
		dev, _ := syscall.UTF16PtrFromString(m.Device)
		if r, _, _ := procEnumDisplayDevicesW.Call(uintptr(unsafe.Pointer(dev)), 0, uintptr(unsafe.Pointer(&dd)), 0); r != 0 {
			m.Name, m.DeviceID = wstr(dd.String[:]), wstr(dd.ID[:])
		}
		if procGetDpiForMonitor.Find() == nil {
			var dx, dy uint32
			if r, _, _ := procGetDpiForMonitor.Call(hmon, mdtEffectiveDPI, uintptr(unsafe.Pointer(&dx)), uintptr(unsafe.Pointer(&dy))); r == 0 && dx > 0 {
				m.DPI = dx
			}
		}
		out = append(out, m)
		return 1
	})
	if r, _, err := procEnumDisplayMonitors.Call(0, 0, cb, 0); r == 0 {
		return nil, callErr("EnumDisplayMonitors", err)
	}
	if len(out) == 0 {
		return nil, errors.New("no monitor is active (is the PC locked or the session disconnected?)")
	}
	return out, nil
}

// MonitorsToDisplays numbers Windows monitors like the other OSes: the
// primary monitor first, then left to right, top to bottom. Geometry stays
// in physical pixels of the virtual screen (X and Y may be negative).
func MonitorsToDisplays(ms []rawMonitor) []Display {
	ds := make([]Display, 0, len(ms))
	for _, m := range ms {
		w, h := int(m.Rect.Right-m.Rect.Left), int(m.Rect.Bottom-m.Rect.Top)
		keySrc := m.DeviceID
		if keySrc == "" {
			keySrc = m.Device
		}
		sum := sha256.Sum256([]byte(keySrc))
		name := strings.TrimSpace(m.Name)
		if name == "" || strings.EqualFold(name, "Generic PnP Monitor") {
			name = strings.TrimPrefix(m.Device, `\\.\`)
		}
		ds = append(ds, Display{
			ID: m.Device, Key: "mon-" + hex.EncodeToString(sum[:8]), Name: name,
			X: float64(m.Rect.Left), Y: float64(m.Rect.Top), W: float64(w), H: float64(h), PixelW: w, PixelH: h,
			Scale: float64(m.DPI) / 96, Main: m.Primary,
		})
	}
	sort.SliceStable(ds, func(i, j int) bool {
		if ds[i].Main != ds[j].Main {
			return ds[i].Main
		}
		if ds[i].X != ds[j].X {
			return ds[i].X < ds[j].X
		}
		return ds[i].Y < ds[j].Y
	})
	for i := range ds {
		ds[i].Index = i + 1
	}
	return ds
}

// Displays lists the active monitors (no permission needed, nothing captured).
func (Win) Displays(context.Context) ([]Display, error) {
	ms, err := enumMonitors()
	if err != nil {
		return nil, err
	}
	return MonitorsToDisplays(ms), nil
}

// CapturePNG copies one monitor from the screen with GDI (BitBlt) and
// returns it as PNG. Nothing is written to disk.
func (Win) CapturePNG(ctx context.Context, d Display) ([]byte, error) {
	InitDPI()
	img, err := captureRect(int32(d.X), int32(d.Y), int32(d.W), int32(d.H))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func captureRect(x, y, w, h int32) (*image.RGBA, error) {
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("display has no size (%d×%d)", w, h)
	}
	screen, _, err := procGetDC.Call(0)
	if screen == 0 {
		return nil, callErr("GetDC", err)
	}
	defer procReleaseDC.Call(0, screen)
	mem, _, err := procCreateCompatibleDC.Call(screen)
	if mem == 0 {
		return nil, callErr("CreateCompatibleDC", err)
	}
	defer procDeleteDC.Call(mem)
	bmp, _, err := procCreateCompatibleBitmap.Call(screen, uintptr(w), uintptr(h))
	if bmp == 0 {
		return nil, callErr("CreateCompatibleBitmap", err)
	}
	defer procDeleteObject.Call(bmp)
	old, _, _ := procSelectObject.Call(mem, bmp)
	defer procSelectObject.Call(mem, old)
	if r, _, err := procBitBlt.Call(mem, 0, 0, uintptr(w), uintptr(h), screen, uintptr(x), uintptr(y), srccopy|captureblt); r == 0 {
		// Fails on the secure desktop (UAC prompt, lock screen).
		return nil, Denied("the screen could not be copied (" + callErr("BitBlt", err).Error() + "); the PC may be locked or showing a security prompt")
	}
	hdr := bitmapInfoHeader{BitCount: 32, Planes: 1, Width: w, Height: -h, Compression: biRGB}
	hdr.Size = uint32(unsafe.Sizeof(hdr))
	img := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	if r, _, err := procGetDIBits.Call(mem, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&img.Pix[0])), uintptr(unsafe.Pointer(&hdr)), dibRGBColors); r == 0 {
		return nil, callErr("GetDIBits", err)
	}
	BGRAToRGBA(img.Pix)
	return img, nil
}

// BGRAToRGBA swaps the blue and red bytes of 32-bit GDI pixels in place and
// makes them opaque (GDI leaves the alpha byte undefined).
func BGRAToRGBA(pix []byte) {
	for i := 0; i+3 < len(pix); i += 4 {
		pix[i], pix[i+2], pix[i+3] = pix[i+2], pix[i], 0xff
	}
}
