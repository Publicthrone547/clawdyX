package main

import (
	"os"
	"path/filepath"

	"github.com/gofrs/flock"

	"github.com/Publicthrone547/clawdyX/internal/config"
)

// lockHeld reports whether some clawdyx instance (foreground or launchd) is running.
func lockHeld(cfg *config.Config) bool {
	_ = os.MkdirAll(cfg.StateDir, 0o700)
	l := flock.New(filepath.Join(cfg.StateDir, "bot.lock"))
	ok, err := l.TryLock()
	if err != nil || !ok {
		return true
	}
	_ = l.Unlock()
	return false
}
