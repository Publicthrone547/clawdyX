package platform

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// scanDirs are the packages whose strings reach users: bot replies, CLI
// output and the desktop app's Go side.
var scanDirs = []string{
	filepath.Join("..", "telegram"),
	filepath.Join("..", "..", "cmd", "clawdyx"),
	filepath.Join("..", "..", "desktop"),
	filepath.Join("..", "..", "desktop", "host"),
	filepath.Join("..", "screen"),
	filepath.Join("..", "camera"),
	filepath.Join("..", "setup"),
	filepath.Join("..", "settings"),
	filepath.Join("..", "config"),
	filepath.Join("..", "jobs"),
}

// osFile reports whether a file holds one OS's code (it may name that OS).
func osFile(name string) bool {
	base := strings.TrimSuffix(name, ".go")
	for _, s := range []string{"_darwin", "_windows", "_linux", "_unix", "_other"} {
		if strings.HasSuffix(base, s) {
			return true
		}
	}
	return false
}

// scanAllowed are literals that may stay in shared files, with the reason.
var scanAllowed = map[string]string{
	"clawdyxw.exe":                          "file name of the Windows login launcher (runkey.go is shared so its quoting is tested everywhere)",
	"clawdyx.exe":                           "file name of the Windows worker (same)",
	"bash.exe":                              "file name checked in claude.git_bash_path, which only Windows validates",
	"sh.exe":                                "same",
	"io.github.publicthrone547.clawdyx.app": "the app's single-instance id, not text",
	".dmg":                                  "the suffix of a release file, matched by the update check",
	"-setup.exe":                            "the same, for the Windows installer",
	`must point at bash.exe (for example C:\Program Files\Git\bin\bash.exe)`: "claude.git_bash_path is validated only on Windows",
}

// TestNoOSWordsInSharedStrings fails when a string literal in a shared
// (not OS-specific) file names an OS or an OS-only product: such text would
// reach users of the other OSes. Move it into a *_darwin.go/_windows.go/
// _linux.go file or into Words.
func TestNoOSWordsInSharedStrings(t *testing.T) {
	for _, dir := range scanDirs {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range files {
			name := filepath.Base(p)
			if strings.HasSuffix(name, "_test.go") || osFile(name) {
				continue
			}
			src, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, p, src, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(f, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				s, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				if _, ok := scanAllowed[s]; ok {
					return true
				}
				if bad := AnyForbidden(s); len(bad) > 0 {
					t.Errorf("%s: %v in %q", fset.Position(lit.Pos()), bad, trim(s))
				}
				return true
			})
		}
	}
}

func trim(s string) string {
	if len(s) > 120 {
		return s[:120] + "…"
	}
	return s
}
