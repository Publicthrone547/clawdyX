package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// privacyWarning flags folders that macOS privacy protection can hide from
// a background (launchd) process.
func privacyWarning(home, p string) string {
	for _, d := range []string{"Desktop", "Documents", "Downloads"} {
		root := filepath.Join(home, d)
		if p == root || strings.HasPrefix(p, root+string(filepath.Separator)) {
			return fmt.Sprintf("%s is protected by macOS privacy settings; the background service may be denied access there. A folder such as ~/projects/<name> avoids that.", root)
		}
	}
	return ""
}

// DetectBrowser picks an installed browser application for /open.
func DetectBrowser() string {
	for _, c := range [][2]string{{"/Applications/Firefox.app", "Firefox"}, {"/Applications/Google Chrome.app", "Google Chrome"}, {"/Applications/Brave Browser.app", "Brave Browser"}, {"/Applications/Arc.app", "Arc"}} {
		if st, err := os.Stat(c[0]); err == nil && st.IsDir() {
			return c[1]
		}
	}
	return "Safari"
}

// claudeCandidates are the usual install locations of the claude CLI, in
// the order they are tried after PATH: Homebrew (Apple silicon, Intel),
// the native installer, the old local install.
func claudeCandidates(home string) []string {
	return []string{"/opt/homebrew/bin/claude", "/usr/local/bin/claude",
		filepath.Join(home, ".local", "bin", "claude"), filepath.Join(home, ".claude", "local", "claude")}
}
