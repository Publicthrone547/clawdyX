package sessions

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateAndForget(t *testing.T) {
	s := setup(t)
	// No active session: behaves like Create.
	a, old, err := s.CreateAndForget("first", nil)
	if err != nil || old != nil || a.Name != "first" {
		t.Fatalf("%v %+v %+v", err, a, old)
	}
	// Busy old session: refused, nothing changes.
	if _, _, err := s.CreateAndForget("second", func(int64) bool { return true }); !errors.Is(err, ErrBusy) {
		t.Fatalf("busy: %v", err)
	}
	if cur, _ := s.Active(); cur.ID != a.ID {
		t.Fatal("active changed after a refused /newf")
	}
	if _, _, err := s.CreateAndForget("first", nil); err == nil {
		t.Fatal("duplicate name accepted")
	}
	b, old, err := s.CreateAndForget("second", func(int64) bool { return false })
	if err != nil || old == nil || old.ID != a.ID || old.Status != "forgotten" {
		t.Fatalf("%v %+v", err, old)
	}
	if cur, _ := s.Active(); cur.ID != b.ID {
		t.Fatal("new session must be active")
	}
	if _, err := s.Resolve("first"); err == nil {
		t.Fatal("forgotten session still resolvable by name")
	}
	if _, err := s.Resolve("S1"); err == nil {
		t.Fatal("forgotten session still resolvable by number")
	}
	// The name of a forgotten session can be reused.
	if _, err := s.Create("first", nil); err != nil {
		t.Fatal(err)
	}
	// Restore within the window.
	if _, err := s.Restore("S1"); err != nil {
		t.Fatal(err)
	}
	x, _ := s.st.GetSession(a.ID)
	if x.Status != "active" || !x.ForgottenAt.IsZero() {
		t.Fatalf("restored %+v", x)
	}
}

func TestFreshCopiesSettingsAndKeepsOld(t *testing.T) {
	s := setup(t)
	a, _ := s.Create("work", nil)
	_ = s.st.SetSessionField(a.ID, "mode", "assisted")
	_ = s.st.SetSessionField(a.ID, "model", "sonnet")
	_ = s.st.SetSessionField(a.ID, "effort", "high")
	f, from, err := s.Fresh(nil)
	if err != nil || from.ID != a.ID {
		t.Fatal(err)
	}
	if f.Mode != "assisted" || f.Model != "sonnet" || f.Effort != "high" || f.Name != "work-fresh" || f.ClaudeSessionID == a.ClaudeSessionID {
		t.Fatalf("fresh %+v", f)
	}
	if old, _ := s.st.GetSession(a.ID); old.Status != "active" {
		t.Fatal("old session must be kept")
	}
	g, _, _ := s.Fresh(nil)
	if g.Name != "work-fresh-fresh" && !strings.HasPrefix(g.Name, "work-fresh") {
		t.Fatalf("second fresh name %q", g.Name)
	}
	if _, _, err := s.Fresh(func(int64) bool { return true }); !errors.Is(err, ErrBusy) {
		t.Fatal("fresh must refuse a busy session")
	}
}

func TestForgetRenameRestoreRules(t *testing.T) {
	s := setup(t)
	a, _ := s.Create("a", nil)
	if err := s.Rename(a, "bad name!"); err == nil {
		t.Fatal("invalid name accepted")
	}
	if err := s.Rename(a, "alpha"); err != nil {
		t.Fatal(err)
	}
	if err := s.Forget(a, func(int64) bool { return true }); !errors.Is(err, ErrBusy) {
		t.Fatal("forget must refuse a busy session")
	}
	if err := s.Forget(a, nil); err != nil {
		t.Fatal(err)
	}
	if cur, _ := s.Active(); cur != nil {
		t.Fatal("forgetting the active session must leave none active")
	}
	_ = s.st.MarkHistoryDeleted(a.ID)
	if _, err := s.Restore("S1"); err == nil {
		t.Fatal("restore after history deletion must fail")
	}
}

func TestHistoryFindAndDelete(t *testing.T) {
	s := setup(t)
	a, _ := s.Create("x", nil)
	projects := t.TempDir()
	slug, ok := ProjectSlug(a.WorkspacePath)
	if !ok || strings.ContainsAny(slug, "/_. ") {
		t.Fatalf("slug %q", slug)
	}
	if h := FindHistory(projects, a); h.Deletable {
		t.Fatal("no transcript yet, must not be deletable")
	}
	dir := filepath.Join(projects, slug)
	os.MkdirAll(filepath.Join(dir, a.ClaudeSessionID, "subagents"), 0o700)
	os.WriteFile(filepath.Join(dir, a.ClaudeSessionID+".jsonl"), []byte("{}\n"), 0o600)
	os.WriteFile(filepath.Join(dir, a.ClaudeSessionID, "subagents", "s.jsonl"), []byte("{}"), 0o600)
	// Things that must survive.
	other := filepath.Join(dir, "11111111-2222-3333-4444-555555555555.jsonl")
	os.WriteFile(other, []byte("{}"), 0o600)
	os.MkdirAll(filepath.Join(dir, "memory"), 0o700)
	os.WriteFile(filepath.Join(dir, "memory", "MEMORY.md"), []byte("m"), 0o600)

	h := FindHistory(projects, a)
	if !h.Deletable || len(h.Paths) != 2 || h.Bytes == 0 {
		t.Fatalf("history %+v", h)
	}
	removed, err := DeleteHistory(projects, a)
	if err != nil || len(removed) != 2 {
		t.Fatalf("%v %v", err, removed)
	}
	for _, keep := range []string{other, filepath.Join(dir, "memory", "MEMORY.md")} {
		if _, err := os.Stat(keep); err != nil {
			t.Fatalf("%s was deleted", keep)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, a.ClaudeSessionID)); !os.IsNotExist(err) {
		t.Fatal("session folder survived")
	}
}

func TestHistoryAfterWorkspaceMoveAndAmbiguity(t *testing.T) {
	s := setup(t)
	a, _ := s.Create("x", nil)
	projects := t.TempDir()
	oldDir := filepath.Join(projects, "-Users-me-old-name")
	os.MkdirAll(oldDir, 0o700)
	os.WriteFile(filepath.Join(oldDir, a.ClaudeSessionID+".jsonl"), []byte("{}"), 0o600)
	if h := FindHistory(projects, a); !h.Deletable || !strings.HasPrefix(h.Paths[0], oldDir) {
		t.Fatalf("moved workspace: %+v", h)
	}
	twin := filepath.Join(projects, "-Users-me-twin")
	os.MkdirAll(twin, 0o700)
	os.WriteFile(filepath.Join(twin, a.ClaudeSessionID+".jsonl"), []byte("{}"), 0o600)
	if h := FindHistory(projects, a); h.Deletable {
		t.Fatal("ambiguous transcript must not be offered")
	}
	// Symlinked transcripts are never followed.
	os.Remove(filepath.Join(twin, a.ClaudeSessionID+".jsonl"))
	os.Remove(filepath.Join(oldDir, a.ClaudeSessionID+".jsonl"))
	target := filepath.Join(t.TempDir(), "victim")
	os.WriteFile(target, []byte("keep"), 0o600)
	os.Symlink(target, filepath.Join(oldDir, a.ClaudeSessionID+".jsonl"))
	if h := FindHistory(projects, a); h.Deletable {
		t.Fatal("symlink offered for deletion")
	}
	if _, err := DeleteHistory(projects, a); err == nil {
		t.Fatal("symlink deleted")
	}
	if b, _ := os.ReadFile(target); string(b) != "keep" {
		t.Fatal("symlink target touched")
	}
	long := strings.Repeat("a", 210)
	if _, ok := ProjectSlug("/" + long); ok {
		t.Fatal("over-long slug must be refused")
	}
	_ = time.Now
}
