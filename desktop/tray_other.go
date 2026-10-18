//go:build !darwin

package main

import (
	_ "embed"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed build/appicon.png
var appIconPNG []byte

// setTrayIcon uses the app icon (template icons exist only on macOS).
func setTrayIcon(t *application.SystemTray) { t.SetIcon(appIconPNG) }
