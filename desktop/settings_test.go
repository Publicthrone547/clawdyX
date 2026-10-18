package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Publicthrone547/clawdyX/internal/control"
	"github.com/Publicthrone547/clawdyX/internal/platform"
	"github.com/Publicthrone547/clawdyX/internal/settings"
	"github.com/Publicthrone547/clawdyX/internal/setup"
)

func offlineConfig(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "claude"+platform.ExeSuffix)
	os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755)
	p := filepath.Join(t.TempDir(), "config.json")
	if err := setup.WriteConfig(p, setup.GenerateConfig(111111111, t.TempDir(), "Firefox", bin)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAWDYX_CONFIG", p)
	return p
}

func TestOfflineSettings(t *testing.T) {
	p := offlineConfig(t)
	s := &SystemService{w: &WorkerService{}} // not connected: the worker is down
	si, err := s.SettingsOffline()
	if err != nil || si.ConfigPath != p || si.Invalid != "" || len(si.Workspaces) != 1 {
		t.Fatalf("read: %v %+v", err, si)
	}
	turns := 42
	if si, err = s.SetSettingsOffline(control.SettingsPatch{MaxTurns: &turns}); err != nil || si.MaxTurns != 42 {
		t.Fatalf("write: %v", err)
	}
	if _, err := s.SetSettingsOffline(control.SettingsPatch{RemoveWorkspace: si.Workspaces[0].Name}); err == nil {
		t.Fatal("removing a workspace needs the worker")
	}
	if si, err = s.ResetSettingsOffline(control.SectionLimits); err != nil || si.MaxTurns == 42 {
		t.Fatalf("reset: %v", err)
	}

	// A broken file: shown with the reason, and fixable from here.
	raw, _ := os.ReadFile(p)
	broken := strings.Replace(string(raw), `"max_turns": 60`, `"max_turns": 9999`, 1)
	os.WriteFile(p, []byte(broken), 0o600)
	if si, err = s.SettingsOffline(); err != nil || !strings.Contains(si.Invalid, "max_turns") {
		t.Fatalf("invalid: %v %q", err, si.Invalid)
	}
	if si, err = s.SetSettingsOffline(control.SettingsPatch{MaxTurns: &turns}); err != nil || si.Invalid != "" {
		t.Fatalf("repair: %v %q", err, si.Invalid)
	}

	ex, _ := settings.Export(p)
	if r, err := s.importSettings(control.SettingsImportParams{JSON: ex.JSON, Apply: true}); err != nil || len(r.Changes) != 0 || !r.Applied {
		t.Fatalf("import: %v %+v", err, r)
	}
}
