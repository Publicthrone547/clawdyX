package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/Publicthrone547/clawdyX/internal/control"
)

func TestNotifierShowsJobsOnlyWhenHiddenAndOn(t *testing.T) {
	var sent []notifications.NotificationOptions
	visible := false
	n := &notifier{send: func(o notifications.NotificationOptions) error { sent = append(sent, o); return nil }, visible: func() bool { return visible }}
	data, _ := json.Marshal(map[string]any{"job": control.JobInfo{Label: "j-7"}, "state": "completed", "durationMs": 42000, "text": "the secret reply"})
	ev := control.Event{Type: "job.finished", Data: data}

	n.event(ev)
	if len(sent) != 0 {
		t.Fatal("sent while off")
	}
	n.on.Store(true)
	visible = true
	n.event(ev)
	if len(sent) != 0 {
		t.Fatal("sent while the window is visible")
	}
	visible = false
	n.event(ev)
	if len(sent) != 1 || sent[0].Body != "Job j-7 finished after 42s." || strings.Contains(sent[0].Body, "secret") {
		t.Fatalf("sent %+v", sent)
	}
	n.event(ev)
	if len(sent) != 1 {
		t.Fatal("the same notification twice within a minute")
	}
	n.event(control.Event{Type: "job.progress", Data: data})
	n.lost()
	if len(sent) != 2 || sent[1].ID != "worker-lost" {
		t.Fatalf("sent %+v", sent)
	}
}
