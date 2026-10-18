package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/vault"
)

// TestMain keeps tests away from the real token store and the real config
// (and so from the settings migration).
func TestMain(m *testing.M) {
	config.TokenVault = vault.NewMemory()
	os.Setenv("CLAWDYX_CONFIG", filepath.Join(os.TempDir(), "clawdyx-test-never-created", "config.json"))
	os.Exit(m.Run())
}

func TestSecondLaunchShowsTheWindow(t *testing.T) {
	shown := 0
	o := appOptions(nil, func() { shown++ })
	if o.SingleInstance == nil || o.SingleInstance.UniqueID != instanceID || o.SingleInstance.OnSecondInstanceLaunch == nil {
		t.Fatalf("single instance: %+v", o.SingleInstance)
	}
	o.SingleInstance.OnSecondInstanceLaunch(application.SecondInstanceData{})
	if shown != 1 {
		t.Fatal("a second launch did not show the window")
	}
	if o.Mac.ApplicationShouldTerminateAfterLastWindowClosed {
		t.Fatal("closing the window must not quit on macOS")
	}
}
