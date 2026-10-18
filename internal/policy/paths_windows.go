package policy

import (
	"path/filepath"
	"strings"
)

// platformSecretPaths are Windows browser profiles, credential stores and
// clawdyX's own state (home-relative).
var platformSecretPaths = []string{
	"AppData/Local/Google", "AppData/Roaming/Mozilla", "AppData/Local/Mozilla", "AppData/Local/BraveSoftware",
	"AppData/Local/Microsoft/Edge", "AppData/Roaming/Microsoft/Credentials", "AppData/Local/Microsoft/Credentials",
	"AppData/Roaming/Microsoft/Protect", "AppData/Local/clawdyx",
}

// ruleAbs writes an absolute path for a permission rule. Claude Code
// matches Windows paths in POSIX form (C:\Users\me → /c/Users/me), and "//"
// anchors an absolute path: //c/Users/me/.ssh.
func ruleAbs(p string) string {
	vol := filepath.VolumeName(p)
	rest := filepath.ToSlash(p[len(vol):])
	if len(vol) == 2 && vol[1] == ':' {
		return "//" + strings.ToLower(vol[:1]) + rest
	}
	return "/" + rest
}
