package telegram

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenURLUsesXDGOpen(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "opened")
	script := "#!/bin/sh\necho \"$1 $DISPLAY\" > " + out + "\n"
	if err := os.WriteFile(filepath.Join(dir, "xdg-open"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	t.Setenv("DISPLAY", ":7")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("XDG_SESSION_TYPE", "x11")
	if err := openURL(context.Background(), "Firefox", "https://example.org/a"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	if got := strings.TrimSpace(string(b)); got != "https://example.org/a :7" {
		t.Fatalf("xdg-open got %q", got)
	}
	t.Setenv("DISPLAY", "")
	t.Setenv("XDG_SESSION_TYPE", "")
	if err := openURL(context.Background(), "", "https://example.org"); err == nil || !strings.Contains(err.Error(), "no graphical session") {
		t.Fatalf("headless: %v", err)
	}
}
