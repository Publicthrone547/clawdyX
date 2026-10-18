package store

import (
	"context"
	"database/sql"
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

func TestMigrateV1ToV2KeepsDataAndBacksUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(migrations[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sessions(name,claude_session_id,workspace_name,workspace_path,mode,created_at,last_used_at) VALUES('old','uuid-1','w','/tmp','observe',1,1)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	os.Chmod(path, 0o600)
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	x, err := s.GetSession(1)
	if err != nil || x.Name != "old" || !x.ForgottenAt.IsZero() {
		t.Fatalf("session after migration: %v %+v", err, x)
	}
	bk, _ := filepath.Glob(filepath.Join(dir, "backups", "state-pre-v4-*.db"))
	if len(bk) != 1 {
		t.Fatalf("expected one pre-migration backup, got %v", bk)
	}
	if err := s.MarkForgotten(1, false); err != nil {
		t.Fatal(err)
	}
	if l, _ := s.ListSessions(true, 10); len(l) != 0 {
		t.Fatal("forgotten session listed")
	}
	if l, _ := s.ListForgotten(time.Hour); len(l) != 1 {
		t.Fatal("tombstone missing")
	}
	s.AuditCapture(Capture{DisplayKey: "d1", Width: 10, Height: 10, Bytes: 5, Destination: "preview"})
	if s.CaptureCount(time.Now().Add(-time.Minute)) != 1 {
		t.Fatal("capture audit missing")
	}
}

func TestMigrateV3ToV4AddsCameraAuditColumns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range migrations[:3] {
		if _, err := db.Exec(m); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`PRAGMA user_version = 3`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO captures(ts,display_key,width,height,bytes,destination) VALUES(1,'d1',10,10,5,'telegram')`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	os.Chmod(path, 0o600)
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.AuditCapture(Capture{Kind: "camera-video", DisplayKey: "cam-1", Width: 1280, Height: 720, Bytes: 900, DurationMS: 10000, Destination: "telegram", Requester: "telegram-owner"})
	got, err := s.RecentCaptures(5)
	if err != nil || len(got) != 2 {
		t.Fatalf("captures: %v %+v", err, got)
	}
	if got[0].Kind != "camera-video" || got[0].DurationMS != 10000 || got[0].DisplayKey != "cam-1" {
		t.Fatalf("camera row %+v", got[0])
	}
	if got[1].Kind != "screen" || got[1].DurationMS != 0 {
		t.Fatalf("old row after migration %+v", got[1])
	}
}
