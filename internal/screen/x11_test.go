package screen

import "testing"

func TestParseEDIDName(t *testing.T) {
	edid := make([]byte, 128)
	// Descriptor 2 (offset 72) is the monitor name.
	copy(edid[72:], []byte{0, 0, 0, 0xFC, 0})
	copy(edid[77:], "DELL U3423WE\n")
	if got := ParseEDIDName(edid); got != "DELL U3423WE" {
		t.Fatalf("name = %q", got)
	}
	if got := ParseEDIDName(edid[:100]); got != "" {
		t.Fatalf("short block gave %q", got)
	}
	if got := ParseEDIDName(make([]byte, 128)); got != "" {
		t.Fatalf("no descriptor gave %q", got)
	}
}

func TestKeysyms(t *testing.T) {
	cases := map[string]uint32{"a": 'a', "é": 0xe9, "€": 0x010020ac, "return": 0xff0d, "f12": 0xffc9, "nosuchkey": 0}
	for k, want := range cases {
		if got := X11KeyKeysym(k); got != want {
			t.Errorf("%q = %#x, want %#x", k, got, want)
		}
	}
	for k := range NamedKeys {
		if _, ok := x11Keysyms[k]; !ok {
			t.Errorf("named key %q has no X11 keysym", k)
		}
	}
	if KeysymForRune('\n') != 0xff0d {
		t.Error("newline is not Return")
	}
}

func TestBGRAToRGBA(t *testing.T) {
	pix := []byte{1, 2, 3, 0, 4, 5, 6, 9}
	BGRAToRGBA(pix)
	want := []byte{3, 2, 1, 255, 6, 5, 4, 255}
	for i := range want {
		if pix[i] != want[i] {
			t.Fatalf("pix = %v, want %v", pix, want)
		}
	}
}
