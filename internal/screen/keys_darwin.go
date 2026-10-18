package screen

import (
	"fmt"
	"strings"
)

// KeyHelp lists example keys for Claude's press_key tool on this OS.
const KeyHelp = "e.g. return, tab, escape, cmd+l, cmd+t, cmd+w, shift+tab, pagedown, up"

// Modifiers are the modifier flag masks (CGEventFlags).
var Modifiers = map[string]uint64{
	"cmd": 0x100000, "command": 0x100000, "shift": 0x20000, "option": 0x80000, "alt": 0x80000, "ctrl": 0x40000, "control": 0x40000,
}

// KeyCodes maps key names to macOS virtual key codes (ANSI layout).
var KeyCodes = map[string]int{
	"return": 36, "enter": 36, "tab": 48, "space": 49, "delete": 51, "backspace": 51, "escape": 53, "esc": 53,
	"forwarddelete": 117, "home": 115, "end": 119, "pageup": 116, "pagedown": 121,
	"left": 123, "right": 124, "down": 125, "up": 126,
	"a": 0, "s": 1, "d": 2, "f": 3, "h": 4, "g": 5, "z": 6, "x": 7, "c": 8, "v": 9, "b": 11, "q": 12, "w": 13, "e": 14, "r": 15,
	"y": 16, "t": 17, "1": 18, "2": 19, "3": 20, "4": 21, "6": 22, "5": 23, "=": 24, "9": 25, "7": 26, "-": 27, "8": 28, "0": 29,
	"]": 30, "o": 31, "u": 32, "[": 33, "i": 34, "p": 35, "l": 37, "j": 38, "'": 39, "k": 40, ";": 41, "\\": 42, ",": 43, "/": 44,
	"n": 45, "m": 46, ".": 47, "`": 50,
	"f1": 122, "f2": 120, "f3": 99, "f4": 118, "f5": 96, "f6": 97, "f7": 98, "f8": 100, "f9": 101, "f10": 109, "f11": 103, "f12": 111,
}

// ParseKey parses "cmd+shift+t", "return", "ctrl+c".
func ParseKey(combo string) (int, uint64, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(combo)), "+")
	if len(parts) == 0 || len(parts) > 5 {
		return 0, 0, fmt.Errorf("invalid key %q", combo)
	}
	var flags uint64
	for _, p := range parts[:len(parts)-1] {
		f, ok := Modifiers[strings.TrimSpace(p)]
		if !ok {
			return 0, 0, fmt.Errorf("unknown modifier %q (use cmd, shift, option, ctrl)", p)
		}
		flags |= f
	}
	k := strings.TrimSpace(parts[len(parts)-1])
	code, ok := KeyCodes[k]
	if !ok {
		return 0, 0, fmt.Errorf("unknown key %q", k)
	}
	return code, flags, nil
}

var modifierNames = map[string]bool{"cmd": true, "command": true, "shift": true, "option": true, "alt": true, "ctrl": true, "control": true}

const modifierHint = "use cmd, shift, option, ctrl"
