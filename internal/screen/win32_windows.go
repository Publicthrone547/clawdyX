package screen

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// The Win32 calls the Windows screen provider uses (user32, gdi32,
// shcore). They are loaded lazily; a missing one fails its call only.
var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	gdi32  = windows.NewLazySystemDLL("gdi32.dll")
	shcore = windows.NewLazySystemDLL("shcore.dll")

	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")
	procSetProcessDpiAwareness        = shcore.NewProc("SetProcessDpiAwareness")
	procGetDpiForMonitor              = shcore.NewProc("GetDpiForMonitor")

	procEnumDisplayMonitors = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW     = user32.NewProc("GetMonitorInfoW")
	procEnumDisplayDevicesW = user32.NewProc("EnumDisplayDevicesW")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	procGetDC               = user32.NewProc("GetDC")
	procReleaseDC           = user32.NewProc("ReleaseDC")
	procSendInput           = user32.NewProc("SendInput")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procVkKeyScanW          = user32.NewProc("VkKeyScanW")
	procMapVirtualKeyW      = user32.NewProc("MapVirtualKeyW")
	procOpenInputDesktop    = user32.NewProc("OpenInputDesktop")
	procCloseDesktop        = user32.NewProc("CloseDesktop")

	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procBitBlt                 = gdi32.NewProc("BitBlt")
	procGetDIBits              = gdi32.NewProc("GetDIBits")
	procDeleteObject           = gdi32.NewProc("DeleteObject")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
)

type rect struct{ Left, Top, Right, Bottom int32 }

type point struct{ X, Y int32 }

// monitorInfoEx is MONITORINFOEXW.
type monitorInfoEx struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
	Device  [32]uint16
}

// displayDevice is DISPLAY_DEVICEW.
type displayDevice struct {
	Size   uint32
	Name   [32]uint16
	String [128]uint16
	Flags  uint32
	ID     [128]uint16
	Key    [128]uint16
}

const (
	monitorInfoPrimary   = 1
	mdtEffectiveDPI      = 0
	smXVirtualScreen     = 76
	smYVirtualScreen     = 77
	smCXVirtualScreen    = 78
	smCYVirtualScreen    = 79
	srccopy              = 0x00CC0020
	captureblt           = 0x40000000
	biRGB                = 0
	dibRGBColors         = 0
	eddGetDeviceIfName   = 0x00000001
	dpiPerMonitorAwareV2 = ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4
)

// bitmapInfoHeader is BITMAPINFOHEADER.
type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

func metric(i int) int32 {
	r, _, _ := procGetSystemMetrics.Call(uintptr(i))
	return int32(r)
}

// virtualScreen is the rectangle around every monitor, in physical pixels.
func virtualScreen() rect {
	x, y := metric(smXVirtualScreen), metric(smYVirtualScreen)
	return rect{x, y, x + metric(smCXVirtualScreen), y + metric(smCYVirtualScreen)}
}

func wstr(b []uint16) string { return windows.UTF16ToString(b) }

func callErr(name string, err error) error {
	if errno, ok := err.(syscall.Errno); ok && errno == 0 {
		return &windowsError{name: name}
	}
	return &windowsError{name: name, err: err}
}

type windowsError struct {
	name string
	err  error
}

func (e *windowsError) Error() string {
	if e.err == nil {
		return e.name + " failed"
	}
	return e.name + ": " + e.err.Error()
}
