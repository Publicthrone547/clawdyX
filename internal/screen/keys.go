package screen

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// NamedKeys are the key names press_key accepts on every OS (plus single
// characters and the modifiers of the OS, see modifierNames).
var NamedKeys = map[string]bool{
	"return": true, "enter": true, "tab": true, "space": true, "delete": true, "backspace": true, "escape": true, "esc": true,
	"forwarddelete": true, "home": true, "end": true, "pageup": true, "pagedown": true,
	"left": true, "right": true, "up": true, "down": true,
	"f1": true, "f2": true, "f3": true, "f4": true, "f5": true, "f6": true, "f7": true, "f8": true, "f9": true, "f10": true, "f11": true, "f12": true,
}

// SplitCombo checks a key combination such as "ctrl+shift+t" against this
// OS's modifier names and returns the modifiers and the key, lowercase.
func SplitCombo(combo string) (mods []string, key string, err error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(combo)), "+")
	if combo == "" || len(parts) > 5 {
		return nil, "", fmt.Errorf("invalid key %q", combo)
	}
	for _, p := range parts[:len(parts)-1] {
		p = strings.TrimSpace(p)
		if !modifierNames[p] {
			return nil, "", fmt.Errorf("unknown modifier %q (%s)", p, modifierHint)
		}
		mods = append(mods, p)
	}
	key = strings.TrimSpace(parts[len(parts)-1])
	if !NamedKeys[key] && utf8.RuneCountInString(key) != 1 {
		return nil, "", fmt.Errorf("unknown key %q", key)
	}
	return mods, key, nil
}
