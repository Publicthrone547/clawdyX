package processes

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestLinuxProcFixture(t *testing.T) {
	root := t.TempDir()
	w := func(p, s string) {
		os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o755)
		os.WriteFile(filepath.Join(root, p), []byte(s), 0o644)
	}
	boot := time.Now().Unix() - 1000
	w("stat", "cpu 1\nbtime "+itoa(boot)+"\n")
	// started 900 s after boot (90000 ticks), used 50 s of CPU (5000 ticks).
	w("10/stat", "10 (claude) S 1 10 10 0 -1 0 0 0 0 0 4000 1000 0 0 20 0 1 0 90000 0 256 0")
	w("10/comm", "claude\n")
	w("11/comm", "bash\n")
	w("loadavg", "1.00 2.00 3.00 1/2 3\n")
	w("meminfo", "MemTotal: 1000 kB\nMemAvailable: 400 kB\n")
	old := procRoot
	procRoot = root
	defer func() { procRoot = old }()

	s := Sample(context.Background(), []int{10, 99})[10]
	if s.RSSKB != 256*int64(os.Getpagesize())/1024 || s.CPU < 45 || s.CPU > 55 {
		t.Fatalf("%+v", s)
	}
	if CountClaude(context.Background()) != 1 {
		t.Fatal("count")
	}
	sys := SystemSummary(context.Background())
	if !sys.OK || !sys.HasLoad || sys.Load15 != 3 || sys.MemTotal != 1000*1024 || sys.MemFree != 400*1024 {
		t.Fatalf("%+v", sys)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
