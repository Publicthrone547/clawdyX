package config

import (
	"os"
	"path/filepath"
	"strings"
)

// Claude Code's native Windows installer puts claude.exe here.
func defaultClaudeBinary(home string) string {
	return filepath.Join(home, ".local", "bin", "claude.exe")
}

// DefaultChildPath is the PATH given to claude children: the user's PATH
// (absolute entries only), so Git for Windows and Node are found.
func DefaultChildPath() string {
	var keep []string
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if p != "" && filepath.IsAbs(p) {
			keep = append(keep, p)
		}
	}
	return strings.Join(keep, string(os.PathListSeparator))
}

func localAppData(home string) string {
	if d := os.Getenv("LOCALAPPDATA"); d != "" && filepath.IsAbs(d) {
		return d
	}
	return filepath.Join(home, "AppData", "Local")
}

func roamingAppData(home string) string {
	if d := os.Getenv("APPDATA"); d != "" && filepath.IsAbs(d) {
		return d
	}
	return filepath.Join(home, "AppData", "Roaming")
}

// configDir is %APPDATA%\clawdyx, the Windows place for per-user settings
// (0.4.x used %USERPROFILE%\.config\clawdyx; see MigrateLegacy).
func configDir(home string) string { return filepath.Join(roamingAppData(home), "clawdyx") }

// legacyConfigPath is where 0.4.x kept config.json on Windows.
func legacyConfigPath(home string) string {
	return filepath.Join(home, ".config", "clawdyx", "config.json")
}

func defaultStateDir(home string) string { return filepath.Join(localAppData(home), "clawdyx") }

func defaultLogDir(home string) string { return filepath.Join(localAppData(home), "clawdyx", "logs") }

// Private is always true on Windows: Go cannot read NTFS ACLs, and the
// files live in the user's profile, which only that user (and
// administrators) can read by default.
func Private(os.FileInfo) bool { return true }

// Executable reports whether st is a regular file with a program extension.
func Executable(st os.FileInfo) bool {
	switch strings.ToLower(filepath.Ext(st.Name())) {
	case ".exe", ".cmd", ".bat", ".com":
		return st.Mode().IsRegular()
	}
	return false
}

// pathEqual compares canonical paths; NTFS is case-insensitive.
func pathEqual(a, b string) bool { return strings.EqualFold(a, b) }

// gitBashUsed: claude.git_bash_path matters only on Windows.
const gitBashUsed = true
