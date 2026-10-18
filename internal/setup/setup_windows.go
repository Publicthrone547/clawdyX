package setup

import (
	"os"
	"path/filepath"
)

func privacyWarning(string, string) string { return "" }

// DetectBrowser: /open uses the default browser on Windows.
func DetectBrowser() string { return "default" }

// claudeCandidates are the usual install locations of the claude CLI, in
// the order they are tried after PATH: the native installer, WinGet's
// links folder, npm's global shim.
func claudeCandidates(home string) []string {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		local = filepath.Join(home, "AppData", "Local")
	}
	roaming := os.Getenv("APPDATA")
	if roaming == "" {
		roaming = filepath.Join(home, "AppData", "Roaming")
	}
	return []string{filepath.Join(home, ".local", "bin", "claude.exe"),
		filepath.Join(local, "Microsoft", "WinGet", "Links", "claude.exe"),
		filepath.Join(roaming, "npm", "claude.cmd")}
}
