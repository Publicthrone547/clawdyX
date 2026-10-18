package screen

import (
	"fmt"
	"unicode/utf8"
)

// KeyHelp lists example keys for Claude's press_key tool on this OS.
const KeyHelp = "e.g. return, tab, escape, ctrl+l, ctrl+t, ctrl+w, alt+tab, shift+tab, pagedown, up"

// modifierNames: "cmd" is not a key here; ctrl does what ⌘ does on a Mac.
var modifierNames = map[string]bool{"ctrl": true, "control": true, "shift": true, "alt": true, "win": true, "super": true, "meta": true}

const modifierHint = "use ctrl, shift, alt, win"

// modifierVK are the virtual-key codes of the modifiers.
var modifierVK = map[string]uint16{"ctrl": 0x11, "control": 0x11, "shift": 0x10, "alt": 0x12, "win": 0x5B, "super": 0x5B, "meta": 0x5B}

// namedVK are the virtual-key codes of NamedKeys; ext marks the keys that
// need KEYEVENTF_EXTENDEDKEY (the navigation block).
var namedVK = map[string]struct {
	vk  uint16
	ext bool
}{
	"return": {0x0D, false}, "enter": {0x0D, false}, "tab": {0x09, false}, "space": {0x20, false},
	"delete": {0x08, false}, "backspace": {0x08, false}, "escape": {0x1B, false}, "esc": {0x1B, false},
	"forwarddelete": {0x2E, true}, "home": {0x24, true}, "end": {0x23, true}, "pageup": {0x21, true}, "pagedown": {0x22, true},
	"left": {0x25, true}, "right": {0x27, true}, "up": {0x26, true}, "down": {0x28, true},
	"f1": {0x70, false}, "f2": {0x71, false}, "f3": {0x72, false}, "f4": {0x73, false}, "f5": {0x74, false}, "f6": {0x75, false},
	"f7": {0x76, false}, "f8": {0x77, false}, "f9": {0x78, false}, "f10": {0x79, false}, "f11": {0x7A, false}, "f12": {0x7B, false},
}

// VirtualKey maps a key name or a single character to its virtual-key
// code. Letters and digits use their fixed codes (layout-independent
// shortcuts such as ctrl+c); other characters ask the current layout.
func VirtualKey(k string) (vk uint16, extended bool, err error) {
	if n, ok := namedVK[k]; ok {
		return n.vk, n.ext, nil
	}
	r, size := utf8.DecodeRuneInString(k)
	if size != len(k) {
		return 0, false, fmt.Errorf("unknown key %q", k)
	}
	switch {
	case r >= 'a' && r <= 'z':
		return uint16(r - 'a' + 'A'), false, nil
	case r >= '0' && r <= '9':
		return uint16(r), false, nil
	}
	v, _, _ := procVkKeyScanW.Call(uintptr(r))
	if int16(v) == -1 {
		return 0, false, fmt.Errorf("the key %q is not on this keyboard layout (type_text types any character)", k)
	}
	return uint16(v & 0xff), false, nil
}
