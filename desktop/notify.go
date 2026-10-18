package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/Publicthrone547/clawdyX/internal/control"
)

// notifier shows a system notification for what happened while the
// window was hidden: a finished job, the worker going away. It is off
// until the UI turns it on (the "Show notifications" preference lives in
// the window's storage). Notifications carry no prompt or reply text.
type notifier struct {
	on   atomic.Bool
	send func(notifications.NotificationOptions) error
	// authorize asks the OS once (macOS shows a prompt; elsewhere it is a no-op).
	authorize func() (bool, error)
	visible   func() bool

	mu   sync.Mutex
	last map[string]time.Time
}

// SetNotifications turns system notifications on or off (the UI calls it
// at start and when the preference changes).
func (s *SystemService) SetNotifications(on bool) {
	if on && !appNotifier.on.Load() && appNotifier.authorize != nil {
		go func() { _, _ = appNotifier.authorize() }()
	}
	appNotifier.on.Store(on)
}

var appNotifier = &notifier{}

// event handles one worker event.
func (n *notifier) event(ev control.Event) {
	if ev.Type != "job.finished" {
		return
	}
	var d struct {
		Job        control.JobInfo `json:"job"`
		State      string          `json:"state"`
		DurationMS int64           `json:"durationMs"`
	}
	if json.Unmarshal(ev.Data, &d) != nil {
		return
	}
	verb := map[string]string{"completed": "finished", "failed": "failed", "timeout": "timed out", "cancelled": "was cancelled", "interrupted": "was interrupted"}[d.State]
	if verb == "" {
		verb = "ended (" + d.State + ")"
	}
	body := fmt.Sprintf("Job %s %s after %s.", d.Job.Label, verb, (time.Duration(d.DurationMS) * time.Millisecond).Round(time.Second))
	n.show("job-"+d.Job.Label, "clawdyX", body)
}

// lost reports that the worker went away (it was connected before).
func (n *notifier) lost() {
	n.show("worker-lost", "clawdyX", "The background worker stopped. Telegram is not answered until it runs again.")
}

func (n *notifier) show(id, title, body string) {
	if !n.on.Load() || n.send == nil || (n.visible != nil && n.visible()) {
		return
	}
	n.mu.Lock()
	if n.last == nil {
		n.last = map[string]time.Time{}
	}
	// The same notification at most once a minute (a worker that keeps
	// failing must not flood the screen).
	if t, ok := n.last[id]; ok && time.Since(t) < time.Minute {
		n.mu.Unlock()
		return
	}
	n.last[id] = time.Now()
	n.mu.Unlock()
	_ = n.send(notifications.NotificationOptions{ID: id, Title: title, Body: body})
}
