package screen

import "errors"

// KeyHelp lists example keys for Claude's press_key tool on this OS.
const KeyHelp = "e.g. return, tab, escape, ctrl+l, ctrl+t, ctrl+w, alt+tab, shift+tab, pagedown, up"

// NewDesktop returns this OS's Desktop, which is also its Provider.
func NewDesktop(string) Desktop {
	return Unsupported{Err: errors.New("screen features are not available on this PC yet")}
}

// modifierNames: "cmd" is not a key here; ctrl does what ⌘ does on a Mac.
var modifierNames = map[string]bool{"ctrl": true, "control": true, "shift": true, "alt": true, "win": true, "super": true, "meta": true}

const modifierHint = "use ctrl, shift, alt, win"
