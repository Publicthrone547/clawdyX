package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/control"
)

// ctlMethods is shown by `clawdyx ctl help` (the worker is the authority).
var ctlMethods = []string{
	"ping", "status.get", "bot.start", "bot.stop {cancelJobs}", "bot.pause", "bot.resume",
	"sessions.list {all,forgotten}", "sessions.create {name,workspace}", "sessions.switch {ref}", "sessions.archive {ref}", "sessions.restore {ref}",
	"sessions.newForget {name,deleteHistory}", "sessions.fresh", "sessions.forget {ref,deleteHistory}", "sessions.rename {ref,name}", "sessions.history {ref}",
	"jobs.submit {session,text,captureId}",
	"screens.list", "screens.capture {display}", "screens.full {id}", "screens.discard {id}", "screens.use {id,session,text}", "screens.enabled.set {enabled}",
	"jobs.list {limit}", "jobs.cancel {id}", "jobs.events {id,limit}", "processes.list",
	"claude.status", "claude.models", "model.set {session,model,default}", "effort.set {session,effort,default}", "mode.set {session,mode}",
	"log.tail {lines,level}", "diagnostics.run",
	"telegram.allowlist.get", "telegram.allowlist.set {users:[{id,label,role}]}", "telegram.detect.start {seconds}", "telegram.detect.result", "telegram.detect.stop",
	"telegram.token.status", "telegram.token.test {token}", "telegram.token.set {token}",
	"settings.get", "settings.set {…}", "workspace.setDefault {name}", "worker.restart", "worker.stop",
}

func ctl(cfgPath string, args []string) int {
	if len(args) == 0 || args[0] == "help" {
		fmt.Println("clawdyx ctl METHOD [JSON params]   — talks to the running worker over its private socket")
		fmt.Println("clawdyx ctl events                 — print worker events until Ctrl-C")
		m := append([]string(nil), ctlMethods...)
		sort.Strings(m)
		for _, x := range m {
			fmt.Println("  " + x)
		}
		return exitOK
	}
	cfg, err := loadConfigOnly(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return exitConfig
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	dctx, dcancel := context.WithTimeout(ctx, 3*time.Second)
	c, err := control.Dial(dctx, cfg.ControlSocket())
	dcancel()
	if err != nil {
		fmt.Fprintln(os.Stderr, "the clawdyx worker is not running (no control socket). Start it with: make start")
		return exitError
	}
	defer c.Close()
	if args[0] == "events" {
		if err := c.Subscribe(ctx); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitError
		}
		enc := json.NewEncoder(os.Stdout)
		for {
			select {
			case ev, ok := <-c.Events():
				if !ok {
					return exitOK
				}
				_ = enc.Encode(ev)
			case <-ctx.Done():
				return exitOK
			}
		}
	}
	var params any
	if len(args) > 1 {
		raw := json.RawMessage(args[1])
		if !json.Valid(raw) {
			fmt.Fprintln(os.Stderr, "params must be valid JSON, e.g. '{\"limit\":5}'")
			return exitConfig
		}
		params = raw
	}
	cctx, ccancel := context.WithTimeout(ctx, 3*time.Minute)
	defer ccancel()
	var out json.RawMessage
	if err := c.Call(cctx, args[0], params, &out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitError
	}
	var pretty any
	if json.Unmarshal(out, &pretty) == nil {
		b, _ := json.MarshalIndent(pretty, "", "  ")
		fmt.Println(string(b))
	} else {
		fmt.Println(string(out))
	}
	return exitOK
}

// workerBotState asks a running worker for its polling state ("" if unreachable).
func workerBotState(socket string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c, err := control.Dial(ctx, socket)
	if err != nil {
		return ""
	}
	defer c.Close()
	var st control.BotState
	var s control.Status
	if err := c.Call(ctx, "status.get", nil, &s); err != nil {
		return ""
	}
	st = s.Bot
	return st.State
}
