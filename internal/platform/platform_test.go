package platform

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDetectSession(t *testing.T) {
	cases := []struct {
		env  map[string]string
		want Session
	}{
		{map[string]string{}, SessionHeadless},
		{map[string]string{"DISPLAY": ":0"}, SessionX11},
		{map[string]string{"XDG_SESSION_TYPE": "x11", "DISPLAY": ":1"}, SessionX11},
		{map[string]string{"XDG_SESSION_TYPE": "x11"}, SessionHeadless},
		{map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, SessionWayland},
		{map[string]string{"XDG_SESSION_TYPE": "wayland", "DISPLAY": ":0"}, SessionWayland},
		{map[string]string{"XDG_SESSION_TYPE": "Wayland"}, SessionWayland},
		{map[string]string{"XDG_SESSION_TYPE": "tty"}, SessionHeadless},
	}
	for _, c := range cases {
		if got := DetectSession(func(k string) string { return c.env[k] }); got != c.want {
			t.Errorf("%v: got %s want %s", c.env, got, c.want)
		}
	}
}

func TestParseOSRelease(t *testing.T) {
	doc := "# comment\nNAME=\"Ubuntu\"\nVERSION_ID=\"24.04\"\nPRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\nID=ubuntu\nID_LIKE=debian\n"
	pretty, ver, ids := ParseOSRelease(doc)
	if pretty != "Ubuntu 24.04.1 LTS" || ver != "24.04" || ids != "ubuntu debian" {
		t.Fatalf("%q %q %q", pretty, ver, ids)
	}
	if p, v, i := ParseOSRelease(""); p != "" || v != "" || i != "" {
		t.Fatal("empty document")
	}
}

func TestWindowsPretty(t *testing.T) {
	if got := WindowsPretty(10, 22631, "23H2"); got != "Windows 11 23H2" {
		t.Fatal(got)
	}
	if got := WindowsPretty(10, 19045, "22H2"); got != "Windows 10 22H2" {
		t.Fatal(got)
	}
	if got := WindowsPretty(6, 9600, ""); got != "Windows" {
		t.Fatal(got)
	}
}

func TestCurrentIsComplete(t *testing.T) {
	in := Current()
	if in.OS == "" || in.Arch == "" || in.OSName != W.OSName || in.Session == "" || in.Pretty == "" {
		t.Fatalf("%+v", in)
	}
	if in.Headless != (in.Session == SessionHeadless) {
		t.Fatal("headless flag")
	}
}

// optional fields may be empty for an OS that has no such thing.
var optional = map[string]bool{"ScreenPermission": true, "InputPermission": true}

func TestWordsAreCompleteAndClean(t *testing.T) {
	for _, w := range All {
		v := reflect.ValueOf(w)
		for i := 0; i < v.NumField(); i++ {
			name := v.Type().Field(i).Name
			s := v.Field(i).String()
			if s == "" && !optional[name] {
				t.Errorf("%s: %s is empty", w.OS, name)
			}
			if bad := ForbiddenWords(w.OS, s); len(bad) > 0 {
				t.Errorf("%s: %s = %q uses %v", w.OS, name, s, bad)
			}
		}
		if ByOS(w.OS).OSName != w.OSName {
			t.Errorf("ByOS(%s)", w.OS)
		}
	}
	if W.OS != Current().OS {
		t.Fatalf("W is %s on %s", W.OS, Current().OS)
	}
}

func TestForbiddenMatching(t *testing.T) {
	cases := []struct {
		goos, text string
		want       []string
	}{
		{"windows", "Show in Finder", []string{"Finder"}},
		{"windows", "Macs and Macintosh", []string{"Macs"}},
		{"windows", "open a window", nil},
		{"windows", "press ⌘R", []string{"⌘"}},
		{"darwin", "run clawdyx.exe", []string{".exe"}},
		{"darwin", "Ctrl-C stops it", nil},
		{"darwin", "press Ctrl+R", []string{"Ctrl"}},
		{"darwin", "drag clawdyX.app to Applications", nil},
		{"linux", "drag clawdyX.app", []string{".app"}},
		{"linux", "adapt the apple", nil},
		{"linux", "sudo apt install ffmpeg", nil},
		{"windows", "sudo apt install ffmpeg", []string{"apt"}},
	}
	for _, c := range cases {
		if got := ForbiddenWords(c.goos, c.text); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s %q: got %v want %v", c.goos, c.text, got, c.want)
		}
	}
	if got := AnyForbidden("Finder or File Explorer"); !reflect.DeepEqual(got, []string{"Explorer", "File Explorer", "Finder"}) {
		t.Fatal(got)
	}
}

// TestNoGOOSOutsidePlatform enforces the platform rule: OS differences are
// build-tagged files or this package, never runtime.GOOS checks elsewhere.
func TestNoGOOSOutsidePlatform(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".git", "bin", "platform", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(token.NewFileSet(), p, b, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "runtime" && (sel.Sel.Name == "GOOS") {
					t.Errorf("%s uses runtime.GOOS; use internal/platform or a build-tagged file", p)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
