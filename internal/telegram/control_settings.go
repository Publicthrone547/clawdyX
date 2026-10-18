package telegram

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/control"
	"github.com/Publicthrone547/clawdyX/internal/settings"
)

var restartPending atomic.Bool

// settingsInfo describes the settings file the worker loaded, or c when the
// file cannot be read.
func (a *App) settingsInfo(c *config.Config, def string) control.SettingsInfo {
	return settings.Info(c, a.ConfigPath, def, restartPending.Load())
}

// inUse reports whether a session uses a workspace.
func (a *App) inUse(ws string) bool {
	list, _ := a.st.ListSessions(false, 10000)
	for _, s := range list {
		if s.WorkspaceName == ws {
			return true
		}
	}
	return false
}

// saved records a change of the settings file (it applies after a restart).
func (a *App) saved(what string) {
	restartPending.Store(true)
	a.st.Audit("settings", what, "", "ok", 0)
	a.emit("settings.changed", map[string]bool{"restartPending": true})
}

func (a *App) registerSettingsControl(srv *control.Server) {
	needPath := func() error {
		if a.ConfigPath == "" {
			return control.Errorf("the worker does not know its config path")
		}
		return nil
	}
	srv.Handle("settings.get", 5*time.Second, typed(func(context.Context, none) (any, error) {
		return a.settingsInfo(a.cfg, a.sess.DefaultWorkspace().Name), nil
	}))
	srv.Handle("settings.set", 10*time.Second, typed(func(_ context.Context, p control.SettingsPatch) (any, error) {
		if err := needPath(); err != nil {
			return nil, err
		}
		nc, err := settings.Save(a.ConfigPath, p, a.inUse)
		if err != nil {
			return nil, control.Errorf("not saved: %v", err)
		}
		a.saved("set")
		return a.settingsInfo(nc, a.sess.DefaultWorkspace().Name), nil
	}))
	srv.Handle("settings.reset", 10*time.Second, typed(func(_ context.Context, p control.SettingsResetParams) (any, error) {
		if err := needPath(); err != nil {
			return nil, err
		}
		nc, err := settings.ResetFile(a.ConfigPath, p.Section)
		if err != nil {
			return nil, control.Errorf("not reset: %v", err)
		}
		a.saved("reset " + p.Section)
		return a.settingsInfo(nc, a.sess.DefaultWorkspace().Name), nil
	}))
	srv.Handle("settings.export", 5*time.Second, typed(func(context.Context, none) (any, error) {
		if err := needPath(); err != nil {
			return nil, err
		}
		ex, err := settings.Export(a.ConfigPath)
		if err != nil {
			return nil, control.Errorf("%v", err)
		}
		return ex, nil
	}))
	srv.Handle("settings.import", 10*time.Second, typed(func(_ context.Context, p control.SettingsImportParams) (any, error) {
		if err := needPath(); err != nil {
			return nil, err
		}
		res, nc, err := settings.Import(a.ConfigPath, p)
		if err != nil {
			return nil, control.Errorf("not imported: %v", err)
		}
		if res.Applied {
			a.saved("import")
			res.Settings = a.settingsInfo(nc, a.sess.DefaultWorkspace().Name)
		} else {
			res.Settings = a.settingsInfo(a.cfg, a.sess.DefaultWorkspace().Name)
		}
		return res, nil
	}))
	srv.Handle("workspace.setDefault", 5*time.Second, typed(func(_ context.Context, p control.WorkspaceDefaultParams) (any, error) {
		if err := a.sess.SetDefaultWorkspace(p.Name); err != nil {
			return nil, control.InvalidParams("%s", err.Error())
		}
		return a.settingsInfo(a.cfg, p.Name), nil
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
