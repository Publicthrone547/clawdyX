package setup

import "path/filepath"

func privacyWarning(string, string) string { return "" }

// DetectBrowser: /open uses the default browser (xdg-open) on Linux.
func DetectBrowser() string { return "default" }

// claudeCandidates are the usual install locations of the claude CLI, in
// the order they are tried after PATH: the native installer, a manual or
// package install, npm with a user prefix or a global prefix.
func claudeCandidates(home string) []string {
	return []string{filepath.Join(home, ".local", "bin", "claude"), "/usr/local/bin/claude", "/usr/bin/claude",
		filepath.Join(home, ".npm-global", "bin", "claude"), "/usr/local/lib/node_modules/.bin/claude"}
}
