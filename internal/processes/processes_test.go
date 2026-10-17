package processes

import (
	"context"
	"os"
	"testing"
)

func TestSampleSelf(t *testing.T) {
	s := Sample(context.Background(), []int{os.Getpid()})
	if s[os.Getpid()].RSSKB <= 0 {
		t.Fatalf("no rss for self: %+v", s)
	}
	if len(Sample(context.Background(), nil)) != 0 {
		t.Fatal("empty input must give empty output")
	}
}

func TestSystemSummary(t *testing.T) {
	s := SystemSummary(context.Background())
	if !s.OK || s.MemTotal == 0 || s.Load1 < 0 {
		t.Fatalf("summary %+v", s)
	}
}

func TestCountClaude(t *testing.T) {
	if CountClaude(context.Background()) < 0 {
		t.Fatal("ps failed")
	}
}
