package vault

import (
	"errors"
	"strings"
	"testing"
)

func TestMemoryAndAccounts(t *testing.T) {
	m := NewMemory()
	if _, err := m.Get("a"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	_ = m.Set("a", "v")
	if v, _ := m.Get("a"); v != "v" {
		t.Fatal(v)
	}
	if m.Delete("a") != nil || !errors.Is(m.Delete("a"), ErrNotFound) {
		t.Fatal("delete")
	}
	a1, a2 := TokenAccount("/x/config.json"), TokenAccount("/y/config.json")
	if a1 == a2 || !strings.HasPrefix(a1, "telegram-bot-token@") {
		t.Fatalf("%s %s", a1, a2)
	}
}
