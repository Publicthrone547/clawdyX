package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSessionsAndJobs(t *testing.T) {
	s := open(t)
	sess, err := s.CreateSession(Session{Name: "a", ClaudeSessionID: "u1", WorkspaceName: "ws", WorkspacePath: "/tmp", Mode: "observe"})
	if err != nil {
		t.Fatal(err)
	}
	if sess.Label() != "S1" || sess.Model != "" || sess.Started {
		t.Fatalf("unexpected session %+v", sess)
	}
	if err := s.SetSessionField(sess.ID, "model", "sonnet"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSessionField(sess.ID, "claude_session_id", "x"); err == nil {
		t.Fatal("non-whitelisted field must be rejected")
	}
	got, _ := s.GetSession(sess.ID)
	if got.Model != "sonnet" {
		t.Fatalf("model not set: %+v", got)
	}

	j, err := s.InsertJob(Job{SessionID: sess.ID, TgUpdateID: 100, Model: "sonnet"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertJob(Job{SessionID: sess.ID, TgUpdateID: 100}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("expected duplicate, got %v", err)
	}
	if err := s.MarkJobStarted(j.ID, 1234, 99); err != nil {
		t.Fatal(err)
	}
	running, _ := s.JobsInStates(JobRunning)
	if len(running) != 1 || running[0].PID != 1234 {
		t.Fatalf("running jobs: %+v", running)
	}
	j.State, j.InTokens, j.OutTokens = JobCompleted, 10, 5
	if err := s.FinishJob(*j); err != nil {
		t.Fatal(err)
	}
	st, err := s.Stats(time.Now().Add(-time.Hour))
	if err != nil || st.ByState[JobCompleted] != 1 || st.InTokens != 10 {
		t.Fatalf("stats %+v %v", st, err)
	}
	if err := s.TouchSession(sess.ID, j.ID, true, 1); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetSession(sess.ID)
	if !got.Started || got.Turns != 1 {
		t.Fatalf("touch failed %+v", got)
	}
}

func TestUpdatesAliasesEventsCaps(t *testing.T) {
	s := open(t)
	first, _ := s.MarkUpdate(5)
	again, _ := s.MarkUpdate(5)
	if !first || again {
		t.Fatal("update dedupe broken")
	}
	if err := s.SetAlias(Alias{Name: "fast", Model: "haiku", Canonical: "claude-haiku"}); err != nil {
		t.Fatal(err)
	}
	a, err := s.GetAlias("fast")
	if err != nil || a.Model != "haiku" {
		t.Fatalf("alias %v %v", a, err)
	}
	if err := s.DeleteAlias("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatal("expected not found")
	}
	sess, _ := s.CreateSession(Session{ClaudeSessionID: "u", WorkspaceName: "w", WorkspacePath: "/", Mode: "observe"})
	j, _ := s.InsertJob(Job{SessionID: sess.ID})
	for i := 1; i <= 3; i++ {
		_ = s.AddJobEvent(j.ID, i, "tool_use", "Read x")
	}
	ev, _ := s.JobEvents(j.ID, 2)
	if len(ev) != 2 || ev[0].Seq != 2 {
		t.Fatalf("events %+v", ev)
	}
	_ = s.SetCapability("model:fable", "claude-fable-5-1", "2.1.292")
	if v, ok := s.Capability("model:fable", "2.1.292", time.Hour); !ok || v != "claude-fable-5-1" {
		t.Fatal("cap miss")
	}
	if _, ok := s.Capability("model:fable", "2.1.293", time.Hour); ok {
		t.Fatal("cap must be invalidated by version change")
	}
}

func TestBackupAndPerms(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	st, _ := os.Stat(filepath.Join(dir, "state.db"))
	if st.Mode().Perm()&0o077 != 0 {
		t.Fatalf("db perms %v", st.Mode().Perm())
	}
	bk := filepath.Join(dir, "b.db")
	if err := s.Backup(context.Background(), bk); err != nil {
		t.Fatal(err)
	}
	if err := s.QuickCheck(); err != nil {
		t.Fatal(err)
	}
	if err := s.Prune(Retention{JobsDays: 1, JobEventsDays: 1, ProcessedUpdatesDays: 1, AuditDays: 1}); err != nil {
		t.Fatal(err)
	}
}
