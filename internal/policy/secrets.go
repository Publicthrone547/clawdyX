package policy

import (
	"path"
	"strings"
)

// commonSecrets are home-relative places whose contents Claude must never
// read or change, in any tier, on every OS: credentials, shell history,
// Claude Code's own settings and the whole ~/.config (which also holds
// clawdyX's settings on macOS and Linux).
var commonSecrets = []string{
	".ssh", ".aws", ".gnupg", ".kube", ".docker", ".netrc", ".npmrc", ".pypirc",
	".config", ".claude", ".claude.json", ".git-credentials", ".zsh_history", ".bash_history",
}

// osSecrets are each OS's browser profiles, credential stores and
// clawdyX's own state and settings (home-relative, "/"-separated).
var osSecrets = map[string][]string{
	"darwin": {
		"Library/Keychains", "Library/Cookies",
		"Library/Application Support/Google", "Library/Application Support/Firefox",
		"Library/Application Support/BraveSoftware", "Library/Safari",
		"Library/Application Support/clawdyx", "Library/Application Support/claude-tg",
	},
	"windows": {
		"AppData/Local/Google", "AppData/Roaming/Mozilla", "AppData/Local/Mozilla", "AppData/Local/BraveSoftware",
		"AppData/Local/Microsoft/Edge", "AppData/Roaming/Microsoft/Credentials", "AppData/Local/Microsoft/Credentials",
		"AppData/Roaming/Microsoft/Protect", "AppData/Local/Microsoft/Windows/WebCache", "AppData/Local/Microsoft/Windows/INetCookies",
		"AppData/Roaming/Opera Software", "AppData/Local/Vivaldi", "AppData/Roaming/Code/User/globalStorage",
		"AppData/Roaming/gnupg", "AppData/Local/Packages", "_netrc",
		"AppData/Local/clawdyx", "AppData/Roaming/clawdyx",
	},
	"linux": {
		".local/share/keyrings", ".local/state/clawdyx", ".mozilla", ".var/app", "snap",
		".password-store", ".pki", ".cert", ".vault-token", ".terraform.d",
	},
}

// SecretRelPaths are the home-relative secret places of an OS.
func SecretRelPaths(goos string) []string {
	return append(append([]string{}, commonSecrets...), osSecrets[goos]...)
}

// ruleHome writes a home folder the way Claude Code's permission rules
// name absolute paths: "//" + the POSIX form. Windows paths become their
// Git Bash form (C:\Users\me → //c/Users/me).
func ruleHome(goos, home string) string {
	if goos == "windows" {
		h := strings.ReplaceAll(home, `\`, "/")
		if len(h) >= 2 && h[1] == ':' {
			return "//" + strings.ToLower(h[:1]) + h[2:]
		}
		return "/" + h
	}
	return "/" + home
}

// secretRulesFor denies reading and editing every secret place of an OS
// under home.
func secretRulesFor(goos, home string) []string {
	base := strings.TrimSuffix(ruleHome(goos, home), "/")
	var rules []string
	for _, rel := range SecretRelPaths(goos) {
		abs := path.Join(base, rel)
		if strings.HasPrefix(base, "//") && !strings.HasPrefix(abs, "//") {
			abs = "/" + abs // path.Join cleans the leading "//"
		}
		rules = append(rules, "Read("+abs+"/**)", "Read("+abs+")", "Edit("+abs+"/**)", "Edit("+abs+")")
	}
	return rules
}
