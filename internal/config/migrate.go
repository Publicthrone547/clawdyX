package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Publicthrone547/clawdyX/internal/vault"
)

// MigratedMarker is written next to a migrated config file; it holds the
// old path. The worker records it in its state database at the next start
// and removes it.
const MigratedMarker = ".migrated-from"

// MigrateDefault moves the config of an earlier release to the standard
// path of this OS (Windows: %USERPROFILE%\.config\clawdyx → %APPDATA%\clawdyx).
// It does nothing when CLAWDYX_CONFIG is set, when this OS never moved its
// config, or when the standard config already exists. cfgPath is the path
// the caller works with; a path other than the old default (for example a
// test instance's -config) also skips the move. Callers run it only while
// no worker uses the old file: `clawdyx service start` after stopping the
// worker, right before it registers the new path for login.
func MigrateDefault(cfgPath string) (moved bool, from string, err error) {
	old := LegacyPath()
	if old == "" || os.Getenv("CLAWDYX_CONFIG") != "" || !samePath(cfgPath, old) {
		return false, "", nil
	}
	moved, err = MigrateLegacy(StandardPath(), old)
	return moved, old, err
}

// MigrateLegacy moves config.json and .env from oldPath's folder to
// newPath's folder and re-keys the stored bot token (its vault account is
// derived from the config path). It refuses to overwrite an existing
// newPath and is a no-op when oldPath does not exist, so running it again
// is harmless. The old folder is removed when it is empty afterwards.
func MigrateLegacy(newPath, oldPath string) (bool, error) {
	if oldPath == "" || samePath(newPath, oldPath) {
		return false, nil
	}
	if _, err := os.Stat(newPath); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("config migration: %w", err)
	}
	if _, err := os.Stat(oldPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("config migration: %w", err)
	}
	dir := filepath.Dir(newPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, fmt.Errorf("config migration: %w", err)
	}
	_ = os.Chmod(dir, 0o700)

	// Copy the token to the new account first: if anything below fails, the
	// old config still finds its token.
	tok, terr := StoredToken(oldPath)
	switch {
	case terr == nil:
		if err := TokenVault.Set(vault.TokenAccount(newPath), tok); err != nil {
			return false, fmt.Errorf("config migration: storing the token for the new location: %v", err)
		}
	case errors.Is(terr, vault.ErrNotFound):
	default:
		return false, fmt.Errorf("config migration: reading the stored token: %v", terr)
	}

	oldEnv, newEnv := EnvPathFor(oldPath), EnvPathFor(newPath)
	if err := moveFile(oldPath, newPath); err != nil {
		if terr == nil {
			_ = TokenVault.Delete(vault.TokenAccount(newPath))
		}
		return false, fmt.Errorf("config migration: %w", err)
	}
	if _, err := os.Stat(oldEnv); err == nil {
		if _, err := os.Stat(newEnv); errors.Is(err, os.ErrNotExist) {
			if err := moveFile(oldEnv, newEnv); err != nil {
				return true, fmt.Errorf("config migration: the settings moved, but %s could not: %w", oldEnv, err)
			}
		}
	}
	if terr == nil {
		_ = TokenVault.Delete(vault.TokenAccount(oldPath))
	}
	_ = os.WriteFile(filepath.Join(dir, MigratedMarker), []byte(oldPath+"\n"), 0o600)
	_ = os.Remove(filepath.Dir(oldPath)) // only succeeds when empty
	return true, nil
}

// TakeMigratedMarker returns the old path recorded by MigrateLegacy for the
// config at configPath and removes the marker ("" when there is none).
func TakeMigratedMarker(configPath string) string {
	p := filepath.Join(filepath.Dir(configPath), MigratedMarker)
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	_ = os.Remove(p)
	return strings.TrimSpace(string(b))
}

func samePath(a, b string) bool {
	x, err1 := filepath.Abs(a)
	y, err2 := filepath.Abs(b)
	return err1 == nil && err2 == nil && pathEqual(x, y)
}

// moveFile renames src to dst, copying across volumes (mode 0600).
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		in.Close()
		return err
	}
	_, err = io.Copy(out, in)
	in.Close()
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(dst)
		return err
	}
	return os.Remove(src)
}
