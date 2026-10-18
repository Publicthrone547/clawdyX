package config

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

// DefaultServiceLabel names the background worker's login entry
// (LaunchAgent label, Windows Run value, systemd unit "clawdyx").
const DefaultServiceLabel = "local.clawdyx"

var labelNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ServiceLabel is CLAWDYX_SERVICE_LABEL when it is a valid name, else
// DefaultServiceLabel. A second label lets a test instance register its
// own login entry next to the real one.
func ServiceLabel() string {
	if l := os.Getenv("CLAWDYX_SERVICE_LABEL"); labelNameRe.MatchString(l) {
		return l
	}
	return DefaultServiceLabel
}

// Folders returns the state and log folders a config file names, with the
// defaults filled in, even when the file is otherwise invalid (the
// supervisor needs them to wait for a fixed config). It reads nothing but
// these two keys.
func Folders(configPath string) (stateDir, logDir string) {
	var c struct {
		StateDir string `json:"state_dir"`
		LogDir   string `json:"log_dir"`
	}
	if b, err := os.ReadFile(configPath); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	home, _ := os.UserHomeDir()
	if c.StateDir == "" {
		c.StateDir = defaultStateDir(home)
	}
	if c.LogDir == "" {
		c.LogDir = defaultLogDir(home)
	}
	return c.StateDir, c.LogDir
}

// SystemdUnit is the systemd user unit of a service label: "clawdyx" for
// the default label, "clawdyx-dev" for local.clawdyx.dev.
func SystemdUnit(label string) string {
	n := strings.ReplaceAll(strings.TrimPrefix(label, "local."), ".", "-")
	if n == "" {
		return "clawdyx"
	}
	return n
}

// UnitProgram returns the executable of a systemd unit file's ExecStart
// line (the first word, unquoted).
func UnitProgram(unit string) string {
	for _, l := range strings.Split(unit, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(l), "ExecStart=")
		if !ok {
			continue
		}
		if !strings.HasPrefix(rest, `"`) {
			if f := strings.Fields(rest); len(f) > 0 {
				return f[0]
			}
			return ""
		}
		var b strings.Builder
		for i := 1; i < len(rest); i++ {
			switch c := rest[i]; {
			case c == '\\' && i+1 < len(rest):
				i++
				b.WriteByte(rest[i])
			case c == '"':
				return strings.NewReplacer("$$", "$", "%%", "%").Replace(b.String())
			default:
				b.WriteByte(c)
			}
		}
		return ""
	}
	return ""
}
