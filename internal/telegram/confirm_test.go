package telegram

import (
	"testing"
	"time"
)

func TestConfirms(t *testing.T) {
	c := NewConfirms()
	tok := c.New("stop_all", "2")
	if _, _, err := c.Take(tok, "shutdown"); err != ErrConfirmMismatch {
		t.Fatalf("mismatched action accepted: %v", err)
	}
	a, args, err := c.Take(tok, "stop_all")
	if err != nil || a != "stop_all" || args != "2" {
		t.Fatal(err)
	}
	if _, _, err := c.Take(tok, "stop_all"); err != ErrConfirmUsed {
		t.Fatalf("replay accepted: %v", err)
	}
	if _, _, err := c.Take("ZZZZZZ", ""); err != ErrConfirmUnknown {
		t.Fatal("unknown token accepted")
	}
	old := c.New("shutdown", "")
	now := time.Now()
	c.now = func() time.Time { return now.Add(ConfirmTTL + time.Second) }
	if _, _, err := c.Take(old, "shutdown"); err != ErrConfirmUnknown {
		t.Fatalf("stale token accepted: %v", err)
	}
	c.now = time.Now
	d := c.New("x", "")
	if !c.Deny(d) || c.Deny(d) {
		t.Fatal("deny semantics")
	}
	if _, _, err := c.Take(d, "x"); err != ErrConfirmUsed {
		t.Fatal("denied token usable")
	}
}
