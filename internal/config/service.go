package config

import (
	"encoding/json"
	"os"
	"regexp"
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
