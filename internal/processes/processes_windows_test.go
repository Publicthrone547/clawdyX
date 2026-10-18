package processes

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestWindowsCPUAndNoLoad(t *testing.T) {
	end := time.Now().Add(300 * time.Millisecond)
	for x := 0; time.Now().Before(end); x++ {
		_ = x * x
	}
	if s := Sample(context.Background(), []int{os.Getpid()})[os.Getpid()]; s.CPU <= 0 {
		t.Fatalf("no CPU for a busy process: %+v", s)
	}
	if sys := SystemSummary(context.Background()); sys.HasLoad {
		t.Fatal("Windows has no load average")
	}
}
