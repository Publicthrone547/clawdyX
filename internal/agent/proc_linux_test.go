package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProcFactsFromFixture(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "77"), 0o755)
	os.WriteFile(filepath.Join(root, "stat"), []byte("cpu 1\nbtime 1000\n"), 0o644)
	os.WriteFile(filepath.Join(root, "77", "stat"), []byte("77 (claude) S 1 77 77 0 -1 0 0 0 0 0 5 5 0 0 20 0 1 0 12345 0 10 0"), 0o644)
	os.WriteFile(filepath.Join(root, "77", "comm"), []byte("claude\n"), 0o644)
	old := procRoot
	procRoot = root
	defer func() { procRoot = old }()
	if got := ProcessStartUnix(77); got != 1000+123 {
		t.Fatalf("start %d", got)
	}
	if ProcessComm(77) != "claude" || ProcessComm(78) != "" || ProcessStartUnix(78) != 0 {
		t.Fatal("comm or missing pid")
	}
}
