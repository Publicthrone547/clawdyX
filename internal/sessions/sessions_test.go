package sessions

import (
	"path/filepath"
	"testing"

	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/store"
)

func setup(t *testing.T) *Service {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	dir := t.TempDir()
	canon, _ := filepath.EvalSymlinks(dir)
	cfg := &config.Config{Workspaces: []config.Workspace{{Name: "w", Path: dir, Canonical: canon, DefaultMode: "observe"}}, DefaultWorkspace: "w",
		Claude: config.Claude{DefaultModel: "fable", DefaultEffort: "auto"}}
	return New(st, cfg)
}

func TestLifecycle(t *testing.T) {
	s := setup(t)
	if a, _ := s.Active(); a != nil {
		t.Fatal("expected no active session")
	}
	x, created, err := s.EnsureActive()
	if err != nil || !created || x.Label() != "S1" || len(x.ClaudeSessionID) != 36 {
		t.Fatalf("ensure: %+v %v %v", x, created, err)
	}
	y, err := s.Create("work", nil)
	if err != nil || y.Label() != "S2" {
		t.Fatal(err)
	}
	if a, _ := s.Active(); a.ID != y.ID {
		t.Fatal("new session must be active")
	}
	for _, ref := range []string{"S1", "1", x.ClaudeSessionID[:8]} {
		got, err := s.Resolve(ref)
		if err != nil || got.ID != x.ID {
			t.Fatalf("resolve %q: %v", ref, err)
		}
	}
	if got, err := s.Resolve("work"); err != nil || got.ID != y.ID {
		t.Fatal("resolve by name")
	}
	if _, err := s.Resolve("nope"); err == nil {
		t.Fatal("bad ref resolved")
	}
	if _, err := s.Create("bad name!", nil); err == nil {
		t.Fatal("bad name accepted")
	}
	m, e := s.Effective(x)
	if m != "fable" || e != "auto" {
		t.Fatalf("effective %s %s", m, e)
	}
	_ = s.SetDefaultModel("sonnet")
	if m, _ := s.Effective(x); m != "sonnet" {
		t.Fatal("default model not applied")
	}
	if err := s.Archive(y); err != nil {
		t.Fatal(err)
	}
	if a, _ := s.Active(); a != nil {
		t.Fatal("archived session still active")
	}
	if _, err := s.Switch("work"); err == nil {
		t.Fatal("switch to archived allowed")
	}
}
