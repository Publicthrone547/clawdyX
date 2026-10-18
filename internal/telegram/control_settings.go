package telegram

import (
	"context"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/camera"
	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/control"
	"github.com/Publicthrone547/clawdyX/internal/setup"
)

var restartPending atomic.Bool

func settingsInfo(c *config.Config, path, def string) control.SettingsInfo {
	si := control.SettingsInfo{ConfigPath: path, DefaultWorkspace: def, ClaudeBinary: c.Claude.Binary, MaxConcurrentJobs: c.Claude.MaxConcurrentJobs,
		JobTimeoutMinutes: int(c.Claude.JobTimeout.Duration / time.Minute), MaxTurns: c.Claude.MaxTurns, AutomationEnabled: c.Modes.AutomationEnabled,
		ScreenControl: c.Desktop.ScreenControl, Camera: c.Desktop.Camera, CameraSupported: camera.Supported, OpenURLs: c.Desktop.OpenEnabled(), BrowserApp: c.Desktop.BrowserApp, ShutdownCommand: c.ShutdownCommandEnabled,
		LogLevel: c.Logging.Level, JobsDays: c.Retention.JobsDays, JobEventsDays: c.Retention.JobEventsDays, AuditDays: c.Retention.AuditDays,
		RestartPending: restartPending.Load(), Workspaces: []control.WorkspaceInfo{}}
	for _, w := range c.Workspaces {
		si.Workspaces = append(si.Workspaces, control.WorkspaceInfo{Name: w.Name, Path: w.Path, DefaultMode: w.DefaultMode,
			AllowedCommands: append([]string{}, w.AllowedCommands...), DeniedCommands: append([]string{}, w.DeniedCommands...), IsDefault: w.Name == def})
	}
	return si
}

// applyPatch changes c according to p (validated afterwards by config.Rewrite).
func applyPatch(c *config.Config, p control.SettingsPatch, inUse func(ws string) bool) error {
	if p.MaxConcurrentJobs != nil {
		c.Claude.MaxConcurrentJobs = *p.MaxConcurrentJobs
	}
	if p.JobTimeoutMinutes != nil {
		c.Claude.JobTimeout = config.Duration{Duration: time.Duration(*p.JobTimeoutMinutes) * time.Minute}
	}
	if p.MaxTurns != nil {
		c.Claude.MaxTurns = *p.MaxTurns
	}
	if p.AutomationEnabled != nil {
		c.Modes.AutomationEnabled = *p.AutomationEnabled
	}
	if p.ScreenControl != nil {
		c.Desktop.ScreenControl = *p.ScreenControl
	}
	if p.Camera != nil {
		if *p.Camera && !camera.Supported {
			return camera.ErrUnsupported
		}
		c.Desktop.Camera = *p.Camera
	}
	if p.OpenURLs != nil {
		v := *p.OpenURLs
		c.Desktop.OpenURLs = &v
	}
	if p.BrowserApp != nil {
		c.Desktop.BrowserApp = *p.BrowserApp
	}
	if p.ShutdownCommand != nil {
		c.ShutdownCommandEnabled = *p.ShutdownCommand
	}
	if p.LogLevel != nil {
		c.Logging.Level = *p.LogLevel
	}
	if p.JobsDays != nil {
		c.Retention.JobsDays = *p.JobsDays
	}
	if p.JobEventsDays != nil {
		c.Retention.JobEventsDays = *p.JobEventsDays
	}
	if p.AuditDays != nil {
		c.Retention.AuditDays = *p.AuditDays
	}
	for name, mode := range p.WorkspaceModes {
		found := false
		for i := range c.Workspaces {
			if c.Workspaces[i].Name == name {
				c.Workspaces[i].DefaultMode, found = mode, true
			}
		}
		if !found {
			return fmt.Errorf("unknown workspace %q", name)
		}
	}
	if p.AddWorkspace != nil {
		path := filepath.Clean(setup.ExpandHome(p.AddWorkspace.Path))
		if err := setup.WorkspaceOK(path); err != nil {
			return err
		}
		name := p.AddWorkspace.Name
		if name == "" {
			name = setup.WorkspaceName(path)
		}
		w := setup.NewWorkspace(name, path)
		if p.AddWorkspace.DefaultMode != "" {
			w.DefaultMode = p.AddWorkspace.DefaultMode
		}
		c.Workspaces = append(c.Workspaces, w)
	}
	if p.RemoveWorkspace != "" {
		if p.RemoveWorkspace == c.DefaultWorkspace {
			return fmt.Errorf("%s is the default workspace in config.json; make another one the default first", p.RemoveWorkspace)
		}
		if inUse(p.RemoveWorkspace) {
			return fmt.Errorf("%s is used by sessions; archive or forget them first", p.RemoveWorkspace)
		}
		kept := c.Workspaces[:0]
		for _, w := range c.Workspaces {
			if w.Name != p.RemoveWorkspace {
				kept = append(kept, w)
			}
		}
		if len(kept) == len(c.Workspaces) {
			return fmt.Errorf("unknown workspace %q", p.RemoveWorkspace)
		}
		c.Workspaces = kept
	}
	return nil
}

func (a *App) registerSettingsControl(srv *control.Server) {
	srv.Handle("settings.get", 5*time.Second, typed(func(context.Context, none) (any, error) {
		return settingsInfo(a.cfg, a.ConfigPath, a.sess.DefaultWorkspace().Name), nil
	}))
	srv.Handle("settings.set", 10*time.Second, typed(func(_ context.Context, p control.SettingsPatch) (any, error) {
		if a.ConfigPath == "" {
			return nil, control.Errorf("the worker does not know its config path")
		}
		inUse := func(ws string) bool {
			list, _ := a.st.ListSessions(false, 10000)
			for _, s := range list {
				if s.WorkspaceName == ws {
					return true
				}
			}
			return false
		}
		nc, err := config.Rewrite(a.ConfigPath, func(c *config.Config) error { return applyPatch(c, p, inUse) })
		if err != nil {
			return nil, control.Errorf("not saved: %v", err)
		}
		restartPending.Store(true)
		a.st.Audit("settings", "set", "", "ok", 0)
		a.emit("settings.changed", map[string]bool{"restartPending": true})
		return settingsInfo(nc, a.ConfigPath, a.sess.DefaultWorkspace().Name), nil
	}))
	srv.Handle("workspace.setDefault", 5*time.Second, typed(func(_ context.Context, p control.WorkspaceDefaultParams) (any, error) {
		if err := a.sess.SetDefaultWorkspace(p.Name); err != nil {
			return nil, control.InvalidParams("%s", err.Error())
		}
		return settingsInfo(a.cfg, a.ConfigPath, p.Name), nil
	}))
	srv.Handle("worker.restart", 5*time.Second, typed(func(context.Context, none) (any, error) {
		a.log.Info("restart requested from the desktop app")
		a.requestRestart()
		return map[string]bool{"restarting": true}, nil
	}))
	// worker.stop ends the worker cleanly (exit 0). `clawdyx service stop`
	// uses it on Windows, where a detached process cannot be sent SIGTERM.
	srv.Handle("worker.stop", 5*time.Second, typed(func(context.Context, none) (any, error) {
		a.log.Info("stop requested over the control socket")
		go func() {
			time.Sleep(500 * time.Millisecond) // let the reply reach the client
			a.Stop()
		}()
		return map[string]bool{"stopping": true}, nil
	}))
}
