package screen

import (
	"errors"
	"math"
	"time"
	"unicode/utf16"
	"unsafe"
)

// SendInput structures (amd64 and arm64 layout: INPUT is 40 bytes).
type mouseInput struct {
	Dx, Dy    int32
	MouseData uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type keybdInput struct {
	Vk, Scan  uint16
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type input struct {
	Type uint32
	_    uint32
	data [32]byte
}

const (
	inputMouse    = 0
	inputKeyboard = 1

	mouseMove       = 0x0001
	mouseLeftDown   = 0x0002
	mouseLeftUp     = 0x0004
	mouseRightDown  = 0x0008
	mouseRightUp    = 0x0010
	mouseWheel      = 0x0800
	mouseHWheel     = 0x1000
	mouseVirtualDsk = 0x4000
	mouseAbsolute   = 0x8000
	wheelDelta      = 120

	keyExtended = 0x0001
	keyUp       = 0x0002
	keyUnicode  = 0x0004
)

func mouse(m mouseInput) input {
	in := input{Type: inputMouse}
	*(*mouseInput)(unsafe.Pointer(&in.data[0])) = m
	return in
}

func key(k keybdInput) input {
	in := input{Type: inputKeyboard}
	*(*keybdInput)(unsafe.Pointer(&in.data[0])) = k
	return in
}

// errInputBlocked explains why Windows dropped synthetic input.
var errInputBlocked = errors.New("input was blocked (a window running as administrator, a security prompt or the lock screen has the focus)")

func send(ins ...input) error {
	if len(ins) == 0 {
		return nil
	}
	n, _, _ := procSendInput.Call(uintptr(len(ins)), uintptr(unsafe.Pointer(&ins[0])), unsafe.Sizeof(ins[0]))
	if int(n) != len(ins) {
		return errInputBlocked
	}
	return nil
}

// Normalize maps a point in virtual-screen pixels to SendInput's absolute
// coordinates (0..65535 across the whole virtual screen).
func Normalize(x, y float64, vs rect) (int32, int32) {
	w, h := float64(vs.Right-vs.Left), float64(vs.Bottom-vs.Top)
	if w <= 1 || h <= 1 {
		return 0, 0
	}
	nx := math.Round((x - float64(vs.Left)) * 65535 / (w - 1))
	ny := math.Round((y - float64(vs.Top)) * 65535 / (h - 1))
	clamp := func(v float64) int32 { return int32(math.Max(0, math.Min(65535, v))) }
	return clamp(nx), clamp(ny)
}

func moveTo(x, y float64) input {
	nx, ny := Normalize(x, y, virtualScreen())
	return mouse(mouseInput{Dx: nx, Dy: ny, Flags: mouseMove | mouseAbsolute | mouseVirtualDsk})
}

func clickAt(x, y float64, button string, count int) error {
	InitDPI()
	if err := send(moveTo(x, y)); err != nil {
		return err
	}
	down, up := uint32(mouseLeftDown), uint32(mouseLeftUp)
	if button == "right" {
		down, up = mouseRightDown, mouseRightUp
	}
	time.Sleep(50 * time.Millisecond)
	for i := 0; i < count; i++ {
		if err := send(mouse(mouseInput{Flags: down}), mouse(mouseInput{Flags: up})); err != nil {
			return err
		}
		time.Sleep(60 * time.Millisecond)
	}
	return nil
}

// scrollAt scrolls at (x, y): dy > 0 scrolls up, dx > 0 scrolls left
// (wheel semantics, as on the other OSes).
func scrollAt(x, y float64, dx, dy int) error {
	InitDPI()
	ins := []input{moveTo(x, y)}
	if dy != 0 {
		ins = append(ins, mouse(mouseInput{Flags: mouseWheel, MouseData: uint32(int32(dy * wheelDelta))}))
	}
	if dx != 0 {
		// HWHEEL is positive to the right.
		ins = append(ins, mouse(mouseInput{Flags: mouseHWheel, MouseData: uint32(int32(-dx * wheelDelta))}))
	}
	return send(ins...)
}

// UnicodeKeys turns text into KEYEVENTF_UNICODE key presses, one down/up
// pair per UTF-16 unit (characters outside the BMP as surrogate pairs), so
// any text types regardless of the keyboard layout.
func UnicodeKeys(text string) []keybdInput {
	var out []keybdInput
	for _, u := range utf16.Encode([]rune(text)) {
		out = append(out, keybdInput{Scan: u, Flags: keyUnicode}, keybdInput{Scan: u, Flags: keyUnicode | keyUp})
	}
	return out
}

func typeText(text string) error {
	ks := UnicodeKeys(text)
	for i := 0; i < len(ks); i += 2 {
		if err := send(key(ks[i]), key(ks[i+1])); err != nil {
			return err
		}
		time.Sleep(5 * time.Millisecond)
	}
	return nil
}

func pressCombo(combo string) error {
	mods, k, err := SplitCombo(combo)
	if err != nil {
		return err
	}
	vk, ext, err := VirtualKey(k)
	if err != nil {
		return err
	}
	var down, up []input
	for _, m := range mods {
		mv := modifierVK[m]
		down = append(down, key(keybdInput{Vk: mv}))
		up = append([]input{key(keybdInput{Vk: mv, Flags: keyUp})}, up...)
	}
	flags := uint32(0)
	if ext {
		flags = keyExtended
	}
	scan := mapScan(vk)
	ins := append(down, key(keybdInput{Vk: vk, Scan: scan, Flags: flags}), key(keybdInput{Vk: vk, Scan: scan, Flags: flags | keyUp}))
	return send(append(ins, up...)...)
}

func mapScan(vk uint16) uint16 {
	r, _, _ := procMapVirtualKeyW.Call(uintptr(vk), 0) // MAPVK_VK_TO_VSC
	return uint16(r)
}

// inputDesktop reports whether this process can send input to the desktop
// that has the keyboard (not the secure desktop of a UAC prompt or the
// lock screen).
func inputDesktop() bool {
	h, _, _ := procOpenInputDesktop.Call(0, 0, 0x0100) // DESKTOP_SWITCHDESKTOP
	if h == 0 {
		return false
	}
	_, _, _ = procCloseDesktop.Call(h)
	return true
}
