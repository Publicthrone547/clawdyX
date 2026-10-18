package agent

import "testing"

func TestParseProcStat(t *testing.T) {
	line := "4242 (a b) c)) S 1 4242 4242 0 -1 4194560 100 0 0 0 250 50 0 0 20 0 3 0 987654 123456789 2048 18446744073709551615 1 1 0 0 0 0 0 0 0 0 0 0 17 3 0 0 0 0 0"
	st, ok := ParseProcStat(line)
	if !ok || st.Comm != "a b) c)" || st.UTime != 250 || st.STime != 50 || st.StartTicks != 987654 || st.RSSPages != 2048 {
		t.Fatalf("%+v %v", st, ok)
	}
	for _, bad := range []string{"", "1 (x", "1 (x) S 1 2"} {
		if _, ok := ParseProcStat(bad); ok {
			t.Errorf("accepted %q", bad)
		}
	}
	if b, ok := ParseBootTime("cpu 1 2 3\nbtime 1760000000\nprocesses 9\n"); !ok || b != 1760000000 {
		t.Fatal(b, ok)
	}
	if _, ok := ParseBootTime("cpu 1"); ok {
		t.Fatal("no btime")
	}
}
