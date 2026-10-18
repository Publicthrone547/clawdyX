package processes

import (
	"testing"
	"time"
)

func TestParseLoadAvgAndMemInfo(t *testing.T) {
	l1, l5, l15, ok := ParseLoadAvg("0.52 0.58 0.59 1/467 12345\n")
	if !ok || l1 != 0.52 || l5 != 0.58 || l15 != 0.59 {
		t.Fatal(l1, l5, l15, ok)
	}
	if _, _, _, ok := ParseLoadAvg("x"); ok {
		t.Fatal("garbage accepted")
	}
	total, avail := ParseMemInfo("MemTotal:       16314012 kB\nMemFree:          812340 kB\nMemAvailable:    9876543 kB\n")
	if total != 16314012*1024 || avail != 9876543*1024 {
		t.Fatal(total, avail)
	}
	if _, avail := ParseMemInfo("MemTotal: 100 kB\nMemFree: 40 kB\n"); avail != 40*1024 {
		t.Fatal("old kernels: MemFree stands in", avail)
	}
}

func TestLifetimeCPU(t *testing.T) {
	if got := LifetimeCPU(5, 10*time.Second); got != 50 {
		t.Fatal(got)
	}
	if LifetimeCPU(5, 0) != 0 {
		t.Fatal("zero age")
	}
}
