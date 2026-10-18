package platform

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var updateFixtures = flag.Bool("update-fixtures", false, "rewrite the desktop app's copies of the word tables")

// The desktop app's Vitest guard renders every view with the words of each
// OS and checks them against the same forbidden lists. It reads copies of
// both files; this test keeps them identical to the Go side.
var frontendFixtures = filepath.Join("..", "..", "desktop", "frontend", "src", "__tests__")

func TestFrontendFixturesMatch(t *testing.T) {
	words, err := json.MarshalIndent(map[string]Words{"darwin": Darwin, "windows": Windows, "linux": Linux}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{"words.json": append(words, '\n'), "forbidden.json": ForbiddenJSON}
	for name, want := range files {
		p := filepath.Join(frontendFixtures, name)
		if *updateFixtures {
			if err := os.WriteFile(p, want, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("%v (run: go test ./internal/platform -update-fixtures)", err)
		}
		if !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), want) {
			t.Errorf("%s differs from the Go side; run: go test ./internal/platform -update-fixtures", p)
		}
	}
}
