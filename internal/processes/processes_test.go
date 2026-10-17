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

func TestParseCPUTime(t *testing.T) {
	cases := map[string]float64{"0:01.50": 1.5, "12:34.00": 754, "1:02:03": 3723, "2-00:00:01": 172801}
	for in, want := range cases {
		if got, ok := ParseCPUTime(in); !ok || got != want {
			t.Errorf("%s: got %v %v want %v", in, got, ok, want)
		}
	}
	if _, ok := ParseCPUTime("abc"); ok {
		t.Error("garbage accepted")
	}
}
