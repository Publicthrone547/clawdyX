package telegram

import (
	"testing"
	"time"
)

func TestConfirms(t *testing.T) {
	const me, other = 111111111, 222222222
	c := NewConfirms()
	tok := c.New(me, "stop_all", "2")
	if _, _, err := c.Take(tok, "shutdown", me); err != ErrConfirmMismatch {
		t.Fatalf("mismatched action accepted: %v", err)
	}
	if _, _, err := c.Take(tok, "stop_all", other); err != ErrConfirmMismatch {
		t.Fatalf("another user's confirmation accepted: %v", err)
	}
	if c.Deny(tok, other) {
		t.Fatal("another user denied my confirmation")
	}
	a, args, err := c.Take(tok, "stop_all", me)
	if err != nil || a != "stop_all" || args != "2" {
		t.Fatal(err)
	}
	if _, _, err := c.Take(tok, "stop_all", me); err != ErrConfirmUsed {
		t.Fatalf("replay accepted: %v", err)
	}
	if _, _, err := c.Take("ZZZZZZ", "", me); err != ErrConfirmUnknown {
		t.Fatal("unknown token accepted")
	}
	old := c.New(me, "shutdown", "")
	now := time.Now()
	c.now = func() time.Time { return now.Add(ConfirmTTL + time.Second) }
	if _, _, err := c.Take(old, "shutdown", me); err != ErrConfirmUnknown {
		t.Fatalf("stale token accepted: %v", err)
	}
	c.now = time.Now
	d := c.New(me, "x", "")
	if !c.Deny(d, me) || c.Deny(d, me) {
		t.Fatal("deny semantics")
	}
	if _, _, err := c.Take(d, "x", me); err != ErrConfirmUsed {
		t.Fatal("denied token usable")
	}
}
