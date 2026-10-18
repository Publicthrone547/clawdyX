package screen

import "strings"

// Helpers of the X11 provider (screen on Linux), shared so they are tested
// on every OS.

// ParseEDIDName returns the monitor name of an EDID block (the display
// descriptor of type 0xFC), or "".
func ParseEDIDName(edid []byte) string {
	if len(edid) < 128 {
		return ""
	}
	for off := 54; off+18 <= 126; off += 18 {
		d := edid[off : off+18]
		if d[0] == 0 && d[1] == 0 && d[3] == 0xFC {
			name := string(d[5:18])
			if i := strings.IndexByte(name, '\n'); i >= 0 {
				name = name[:i]
			}
			return strings.TrimSpace(name)
		}
	}
	return ""
}

// KeysymForRune is the X keysym of a character: Latin-1 characters map to
// themselves, everything else to the Unicode keysym range.
func KeysymForRune(r rune) uint32 {
	if (r >= 0x20 && r <= 0x7e) || (r >= 0xa0 && r <= 0xff) {
		return uint32(r)
	}
	switch r {
	case '\n':
		return 0xff0d // Return
	case '\t':
		return 0xff09 // Tab
	}
	return 0x01000000 | uint32(r)
}

// x11Keysyms are the keysyms of NamedKeys.
var x11Keysyms = map[string]uint32{
	"return": 0xff0d, "enter": 0xff0d, "tab": 0xff09, "space": 0x20, "delete": 0xff08, "backspace": 0xff08,
	"escape": 0xff1b, "esc": 0xff1b, "forwarddelete": 0xffff, "home": 0xff50, "end": 0xff57,
	"pageup": 0xff55, "pagedown": 0xff56, "left": 0xff51, "up": 0xff52, "right": 0xff53, "down": 0xff54,
	"f1": 0xffbe, "f2": 0xffbf, "f3": 0xffc0, "f4": 0xffc1, "f5": 0xffc2, "f6": 0xffc3,
	"f7": 0xffc4, "f8": 0xffc5, "f9": 0xffc6, "f10": 0xffc7, "f11": 0xffc8, "f12": 0xffc9,
}

// x11Modifiers are the keysyms of the modifiers press_key accepts on Linux.
var x11Modifiers = map[string]uint32{
	"ctrl": 0xffe3, "control": 0xffe3, "shift": 0xffe1, "alt": 0xffe9, "win": 0xffeb, "super": 0xffeb, "meta": 0xffeb,
}

// X11KeyKeysym returns the keysym of a key name or a single character.
func X11KeyKeysym(k string) uint32 {
	if ks, ok := x11Keysyms[k]; ok {
		return ks
	}
	r := []rune(k)
	if len(r) == 1 {
		return KeysymForRune(r[0])
	}
	return 0
}

// BGRAToRGBA swaps the blue and red bytes of 32-bit pixels in place and
// makes them opaque (GDI and X11 leave the fourth byte undefined).
func BGRAToRGBA(pix []byte) {
	for i := 0; i+3 < len(pix); i += 4 {
		pix[i], pix[i+2], pix[i+3] = pix[i+2], pix[i], 0xff
	}
}
