package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Publicthrone547/clawdyX/internal/platform"
)

// testdata/config-0.4.1-*.json are files the 0.4.1 app wrote (its first
// settings change writes every key). @CLAUDE@, @WORKSPACE@, @STATE@ and @LOGS@
// stand for files and folders of the test.
func fixture041(t *testing.T, goos string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "config-0.4.1-"+goos+".json"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	ws := filepath.Join(dir, "ws")
	if err := os.Mkdir(ws, 0o700); err != nil {
		t.Fatal(err)
	}
	claude := filepath.Join(dir, "claude"+platform.ExeSuffix)
	if err := os.WriteFile(claude, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	q := func(s string) string { b, _ := json.Marshal(s); return strings.Trim(string(b), `"`) }
	return []byte(strings.NewReplacer("@CLAUDE@", q(claude), "@WORKSPACE@", q(ws), "@STATE@", q(filepath.Join(dir, "state")), "@LOGS@", q(filepath.Join(dir, "logs"))).Replace(string(b)))
}

// keys lists the JSON keys of a document as dotted paths.
func keys(t *testing.T, doc []byte) []string {
	t.Helper()
	var v any
	if err := json.Unmarshal(doc, &v); err != nil {
		t.Fatal(err)
	}
	var out []string
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, c := range x {
				out = append(out, prefix+k)
				walk(prefix+k+".", c)
			}
		case []any:
			for _, c := range x {
				walk(prefix, c)
			}
		}
	}
	walk("", v)
	sort.Strings(out)
	return compact(out)
}

func compact(s []string) []string {
	var out []string
	for i, v := range s {
		if i == 0 || v != s[i-1] {
			out = append(out, v)
		}
	}
	return out
}

func TestConfig041Loads(t *testing.T) {
	for _, goos := range []string{"darwin", "windows"} {
		doc := fixture041(t, goos)
		c, invalid, err := ParseLenient(doc)
		if err != nil {
			t.Fatalf("%s: %v", goos, err)
		}
		if goos == platform.W.OS && invalid != nil {
			t.Fatalf("%s: %v", goos, invalid)
		}
		if c.Telegram.OwnerUserID != 111111111 || len(c.Workspaces) != 1 {
			t.Fatalf("%s: %+v", goos, c)
		}
	}
}

// A 1.0.0 rewrite writes no key that 0.4.1 does not know (0.4.1 rejects
// unknown keys), so going back to 0.4.1 works, unless the Git Bash
// location was set.
func TestConfig041RewriteStaysReadableBy041(t *testing.T) {
	for _, goos := range []string{"darwin", "windows"} {
		doc := fixture041(t, goos)
		c, err := decode(doc)
		if err != nil {
			t.Fatal(err)
		}
		out, _ := json.Marshal(c)
		if got, want := strings.Join(keys(t, out), " "), strings.Join(keys(t, doc), " "); got != want {
			t.Errorf("%s: 1.0.0 writes\n%s\n0.4.1 wrote\n%s", goos, got, want)
		}
		c.Claude.GitBashPath = `C:\Program Files\Git\bin\bash.exe`
		out, _ = json.Marshal(c)
		var extra []string
		known := map[string]bool{}
		for _, k := range keys(t, doc) {
			known[k] = true
		}
		for _, k := range keys(t, out) {
			if !known[k] {
				extra = append(extra, k)
			}
		}
		if strings.Join(extra, " ") != "claude.git_bash_path" {
			t.Errorf("%s: keys 0.4.1 does not know: %v", goos, extra)
		}
	}
}

// On this OS a 0.4.1 file survives a real rewrite unchanged in meaning.
func TestConfig041Rewrite(t *testing.T) {
	if platform.W.OS != "darwin" && platform.W.OS != "windows" {
		t.Skip("0.4.1 was released for macOS and Windows only")
	}
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, fixture041(t, platform.W.OS), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := Parse(mustRead(t, p))
	if err != nil {
		t.Fatal(err)
	}
	after, err := Rewrite(p, func(*Config) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if before.Claude.Binary != after.Claude.Binary || before.Workspaces[0].Path != after.Workspaces[0].Path || before.Desktop.BrowserApp != after.Desktop.BrowserApp {
		t.Fatalf("rewrite changed the settings: %+v → %+v", before, after)
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
