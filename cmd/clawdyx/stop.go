//go:build !darwin

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/agent"
	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/control"
)

// stopWorker asks the worker to exit cleanly (worker.stop over the control
// socket) and waits for the state lock; after 40 s it ends the worker's
// process tree.
func stopWorker(cfg *config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if c, err := control.Dial(ctx, cfg.ControlSocket()); err == nil {
		_ = c.Call(ctx, "worker.stop", nil, nil)
		c.Close()
	}
	cancel()
	for i := 0; i < 160; i++ {
		if !lockHeld(cfg) {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	pid := readPID(cfg.StateDir)
	if pid > 0 && agent.ProcessComm(pid) == "clawdyx" {
		_ = agent.KillGroup(pid, true)
		for i := 0; i < 20 && lockHeld(cfg); i++ {
			time.Sleep(250 * time.Millisecond)
		}
	}
	if lockHeld(cfg) {
		return fmt.Errorf("the worker (pid %d) did not stop; end the clawdyx process by hand", pid)
	}
	return nil
}

// stopAll stops the supervisor (so it does not start the worker again) and
// then the worker.
func stopAll(cfg *config.Config) error {
	if _, ok := supervisorAlive(cfg.StateDir); ok {
		if err := writeStopFile(cfg.StateDir); err != nil {
			return err
		}
		defer removeStopFile(cfg.StateDir)
		defer waitSupervisorGone(cfg.StateDir, 10*time.Second)
	}
	return stopWorker(cfg)
}
