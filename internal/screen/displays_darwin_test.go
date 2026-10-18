package screen

import "testing"

func TestParseDisplaysOrderAndKeys(t *testing.T) {
	out := `[{"id":5,"name":"BenQ","x":-1920,"y":-1080,"w":1920,"h":1080,"pw":1920,"ph":1080,"scale":1,"main":false,"vendor":2513,"model":31044,"serial":21573,"uuid":""},
	{"id":4,"name":"VG34","x":0,"y":0,"w":3440,"h":1440,"pw":3440,"ph":1440,"scale":1,"main":true,"uuid":"661406D5-EFCE"},
	{"id":1,"name":"","x":3440,"y":0,"w":1512,"h":982,"pw":3024,"ph":1964,"scale":2,"builtin":true,"uuid":"X"}]`
	ds, err := ParseDisplays(out)
	if err != nil {
		t.Fatal(err)
	}
	if ds[0].ID != "4" || ds[0].Index != 1 || ds[1].ID != "5" || ds[2].ID != "1" || ds[2].Name != "Display 1" {
		t.Fatalf("order %+v", ds)
	}
	if ds[1].Key == "" || ds[1].Key[:3] != "hw-" {
		t.Fatalf("fallback key %q", ds[1].Key)
	}
	if ds[2].PixelW != 3024 {
		t.Fatal("retina pixel size lost")
	}
}

func TestParseKey(t *testing.T) {
	c, f, err := ParseKey("cmd+shift+t")
	if err != nil || c != 17 || f != 0x100000|0x20000 {
		t.Fatalf("%d %x %v", c, f, err)
	}
	if _, _, err := ParseKey("Return"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "cmd+", "super+a", "a+b+c+d+e+f"} {
		if _, _, err := ParseKey(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}
