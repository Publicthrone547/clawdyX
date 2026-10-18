package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Publicthrone547/clawdyX/internal/platform"
)

func twoWorkspaces(t *testing.T, a, b string) string {
	t.Helper()
	deny := `,"denied_commands":["Bash(rm *)","Bash(sudo *)","Bash(git push *)"]`
	return `{"telegram":{"owner_user_id":1},"claude":{"binary":` + jsonStr(fakeBinary(t)) + `},
	"workspaces":[{"name":"a","path":` + jsonStr(a) + deny + `},{"name":"b","path":` + jsonStr(b) + deny + `}],
	"default_workspace":"a"}`
}

func TestNonASCIIAndSpacesInPaths(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "Иван Петров", "мои проекты", "jöhn's app")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	c, err := Parse([]byte(baseJSON(t, "1", ws, "")))
	if err != nil {
		t.Fatalf("non-ASCII workspace rejected: %v", err)
	}
	if !strings.Contains(c.Workspaces[0].Canonical, "мои проекты") {
		t.Fatalf("canonical %q", c.Workspaces[0].Canonical)
	}
}

func TestWorkspacesDifferingOnlyInCase(t *testing.T) {
	root := t.TempDir()
	lower := filepath.Join(root, "proj")
	if err := os.MkdirAll(lower, 0o755); err != nil {
		t.Fatal(err)
	}
	upper := filepath.Join(root, "PROJ")
	caseSensitiveFS := os.MkdirAll(upper, 0o755) == nil && !sameFile(lower, upper)
	_, err := Parse([]byte(twoWorkspaces(t, lower, upper)))
	switch {
	case platform.IsLinux && caseSensitiveFS:
		if err != nil {
			t.Fatalf("two folders on a case-sensitive file system are two workspaces: %v", err)
		}
	default:
		// macOS and Windows: the same folder, whatever the case.
		if err == nil || !strings.Contains(err.Error(), "duplicates another workspace path") {
			t.Fatalf("same folder in another case accepted: %v", err)
		}
	}
}

func sameFile(a, b string) bool {
	x, err1 := os.Stat(a)
	y, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(x, y)
}

func TestLongWorkspacePath(t *testing.T) {
	ws := t.TempDir()
	for len(ws) < 300 {
		ws = filepath.Join(ws, strings.Repeat("d", 40))
	}
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Skipf("the file system refuses a %d-character path: %v", len(ws), err)
	}
	// Accepted, or rejected with a message; never a crash.
	if _, err := Parse([]byte(baseJSON(t, "1", ws, ""))); err != nil && !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("unexpected error for a %d-character path: %v", len(ws), err)
	}
}
