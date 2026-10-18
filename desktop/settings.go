package main

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/Publicthrone547/clawdyX/desktop/host"
	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/control"
	"github.com/Publicthrone547/clawdyX/internal/settings"
	"github.com/Publicthrone547/clawdyX/internal/setup"
)

// The app edits the settings file itself only while the worker is down
// (for example because the file is invalid and the worker refuses to
// start). Both paths use internal/settings and config.Rewrite, so the app
// never writes a file the worker would reject. While the worker runs, the
// UI uses its settings.* methods instead.

var errWorkerUp = errors.New("the worker is running; its settings are changed through it")

func (s *SystemService) offlineOnly() error {
	if s.w != nil && s.w.Connection().Connected {
		return errWorkerUp
	}
	return nil
}

// SettingsOffline reads the settings file while the worker is down;
// Invalid says why the worker would refuse it.
func (s *SystemService) SettingsOffline() (control.SettingsInfo, error) {
	if err := s.offlineOnly(); err != nil {
		return control.SettingsInfo{}, err
	}
	path := config.DefaultPath()
	c, invalid, err := settings.Load(path)
	if err != nil {
		return control.SettingsInfo{}, err
	}
	si := settings.Info(c, path, "", false)
	if invalid != nil {
		si.Invalid = invalid.Error()
	}
	return si, nil
}

// SetSettingsOffline changes the settings file while the worker is down.
// Removing or renaming a workspace needs the worker (it knows the sessions).
func (s *SystemService) SetSettingsOffline(p control.SettingsPatch) (control.SettingsInfo, error) {
	if err := s.offlineOnly(); err != nil {
		return control.SettingsInfo{}, err
	}
	if p.RemoveWorkspace != "" || p.RenameWorkspace != nil {
		return control.SettingsInfo{}, errors.New("start the worker to remove or rename a workspace (it checks which sessions use it)")
	}
	if _, err := settings.Save(config.DefaultPath(), p, func(string) bool { return false }); err != nil {
		return control.SettingsInfo{}, err
	}
	return s.SettingsOffline()
}

// ResetSettingsOffline resets a section of the settings file while the
// worker is down.
func (s *SystemService) ResetSettingsOffline(section string) (control.SettingsInfo, error) {
	if err := s.offlineOnly(); err != nil {
		return control.SettingsInfo{}, err
	}
	if _, err := settings.ResetFile(config.DefaultPath(), section); err != nil {
		return control.SettingsInfo{}, err
	}
	return s.SettingsOffline()
}

// ExportSettings asks where to save the settings and writes them (0600).
// The file never contains the bot token.
func (s *SystemService) ExportSettings() (string, error) {
	ex, err := settings.Export(config.DefaultPath())
	if err != nil {
		return "", err
	}
	home, _ := os.UserHomeDir()
	path, err := s.app.Dialog.SaveFile().SetFilename(ex.Filename).SetDirectory(home).PromptForSingleSelection()
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, []byte(ex.JSON), 0o600); err != nil {
		return "", err
	}
	return homeRel(path), nil
}

// ImportPreview is a settings file chosen for import and what it changes.
type ImportPreview struct {
	File   string                       `json:"file"` // home-relative, for display
	JSON   string                       `json:"json"`
	Result control.SettingsImportResult `json:"result"`
}

// ChooseImport asks for a settings file and previews its changes (nothing
// is changed yet). An empty File means the dialog was cancelled.
func (s *SystemService) ChooseImport() (ImportPreview, error) {
	path, err := s.app.Dialog.OpenFile().CanChooseFiles(true).AddFilter("Settings", "*.json").
		SetTitle("Import clawdyX settings").PromptForSingleSelection()
	if err != nil || path == "" {
		return ImportPreview{}, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return ImportPreview{}, err
	}
	if st.Size() > 1<<20 {
		return ImportPreview{}, errors.New("that file is too large to be clawdyX settings")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ImportPreview{}, err
	}
	pv := ImportPreview{File: homeRel(filepath.Clean(path)), JSON: string(b)}
	pv.Result, err = s.importSettings(control.SettingsImportParams{JSON: pv.JSON})
	return pv, err
}

// ApplyImport replaces the settings with a previewed file.
func (s *SystemService) ApplyImport(jsonDoc string, allowOwnerChange bool) (control.SettingsImportResult, error) {
	return s.importSettings(control.SettingsImportParams{JSON: jsonDoc, AllowOwnerChange: allowOwnerChange, Apply: true})
}

func (s *SystemService) importSettings(p control.SettingsImportParams) (control.SettingsImportResult, error) {
	if s.w != nil && s.w.Connection().Connected {
		var r control.SettingsImportResult
		return r, s.w.call("settings.import", p, &r, short)
	}
	path := config.DefaultPath()
	r, nc, err := settings.Import(path, p)
	if err == nil && nc != nil {
		r.Settings = settings.Info(nc, path, "", false)
	}
	return r, err
}

// InstalledBrowsers lists the browsers /open can use (macOS; elsewhere
// /open uses the default browser and the list is empty).
func (s *SystemService) InstalledBrowsers() []string { return host.InstalledBrowsers() }

// DetectClaude finds the claude CLI (for Settings → Claude Code → Detect).
func (s *SystemService) DetectClaude() string { return setup.DetectClaude() }

// DetectGitBash finds Git for Windows' bash.exe ("" elsewhere or when missing).
func (s *SystemService) DetectGitBash() string { return setup.DetectGitBash() }

// ChooseFile shows a file picker and returns the chosen path ("" when cancelled).
func (s *SystemService) ChooseFile(title string) (string, error) {
	return s.app.Dialog.OpenFile().CanChooseFiles(true).SetTitle(title).PromptForSingleSelection()
}

// ChooseFolder shows a folder picker and returns the chosen path ("" when cancelled).
func (s *SystemService) ChooseFolder(title string) (string, error) {
	return s.app.Dialog.OpenFile().CanChooseDirectories(true).CanChooseFiles(false).CanCreateDirectories(true).
		SetTitle(title).PromptForSingleSelection()
}
