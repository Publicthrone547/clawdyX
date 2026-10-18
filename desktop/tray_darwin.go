package main

import "github.com/wailsapp/wails/v3/pkg/application"

// setTrayIcon uses a template icon, which macOS tints for the menu bar.
func setTrayIcon(t *application.SystemTray) { t.SetTemplateIcon(trayIcon) }
