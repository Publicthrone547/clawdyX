package store

import (
	"os"
	"path/filepath"
	"testing"
)

// testdata/state-0.4.1.db was written by the 0.4.1 store (schema v4): one
// session, one job, one audit row. 1.0.0 opens it as it is.
func TestOpensA041Database(t *testing.T) {
	if schemaVersion != 4 {
		t.Fatalf("schema v%d: a new schema needs a migration from v4 (0.4.1) and an upgrade note", schemaVersion)
	}
	b, err := os.ReadFile(filepath.Join("testdata", "state-0.4.1.db"))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "state.db")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var v int
	if err := st.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil || v != 4 {
		t.Fatalf("user_version %d, %v", v, err)
	}
	ss, err := st.ListSessions(true, 10)
	if err != nil || len(ss) != 1 || ss[0].Name != "main" || ss[0].WorkspacePath != "/work/app" {
		t.Fatalf("sessions %+v, %v", ss, err)
	}
	js, err := st.ListJobs(10)
	if err != nil || len(js) != 1 || js[0].State != "queued" || js[0].PromptPreview != "hello" {
		t.Fatalf("jobs %+v, %v", js, err)
	}
}
