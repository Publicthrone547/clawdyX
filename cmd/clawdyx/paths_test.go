package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintPaths(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.json")
	state := filepath.Join(dir, "st")
	os.WriteFile(cfg, []byte(`{"state_dir":`+quoteJSON(state)+`}`), 0o600)
	var out bytes.Buffer
	if printPaths(&out, cfg, filepath.Join(dir, ".env"), nil) != exitOK {
		t.Fatal(out.String())
	}
	for _, want := range []string{"config  " + cfg, "state   " + state, "socket  " + filepath.Join(state, "control.sock")} {
		if !strings.Contains(out.String(), want+"\n") {
			t.Errorf("missing %q in:\n%s", want, out.String())
		}
	}
	out.Reset()
	if printPaths(&out, cfg, "", []string{"state"}) != exitOK || out.String() != state+"\n" {
		t.Fatalf("one path: %q", out.String())
	}
	if printPaths(&out, cfg, "", []string{"bogus"}) != exitConfig {
		t.Fatal("unknown name accepted")
	}
}

func quoteJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
