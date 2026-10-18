package policy

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Publicthrone547/clawdyX/internal/config"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// goldenHomes are fixed home folders per OS so the files do not depend on
// the machine.
var goldenHomes = map[string]string{"darwin": "/Users/you", "linux": "/home/you", "windows": `C:\Users\you`}

func goldenWorkspace() *config.Workspace {
	return &config.Workspace{Name: "w", Path: "/w", Canonical: "/w", AllowedCommands: []string{"Bash(go test *)"},
		DeniedCommands: append([]string{}, config.MinimumDenied...)}
}

// TestGoldenFlags pins the complete permission flags of every OS and tier.
// Run `go test ./internal/policy -update` after changing a list on purpose,
// and review the diff.
func TestGoldenFlags(t *testing.T) {
	for goos, home := range goldenHomes {
		for _, mode := range []string{config.ModeObserve, config.ModeAssisted, config.ModeAutomation} {
			f, err := ForOS(goos, mode, goldenWorkspace(), config.Claude{}, home, true)
			if err != nil {
				t.Fatal(err)
			}
			got := fmt.Sprintf("permission-mode: %s\nsettings: %s\nallowed:\n  %s\ndisallowed:\n  %s\n",
				f.PermissionMode, f.SettingsJSON, strings.Join(f.Allowed, "\n  "), strings.Join(f.Disallowed, "\n  "))
			file := filepath.Join("golden", goos+"-"+mode+".txt")
			if *update {
				os.MkdirAll("golden", 0o755)
				if err := os.WriteFile(file, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				continue
			}
			want, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("%v (run with -update to create it)", err)
			}
			if got != string(want) {
				t.Errorf("%s differs from the golden file; review and run with -update:\n%s", file, got)
			}
		}
	}
}

func TestBuiltinDenyPerOS(t *testing.T) {
	cases := map[string][]string{
		"windows": {"Bash(powershell *)", "Bash(cmd *)", "Bash(certutil *)", "Bash(Invoke-WebRequest *)", "PowerShell", "Bash(rm *)", "Bash(ssh *)"},
		"linux":   {"Bash(apt *)", "Bash(systemctl *)", "Bash(pkexec *)", "Bash(rm *)"},
		"darwin":  {"Bash(brew *)", "Bash(launchctl *)", "Bash(security *)", "Bash(rm *)"},
	}
	for goos, want := range cases {
		for _, mode := range []string{config.ModeObserve, config.ModeAssisted, config.ModeAutomation} {
			f, _ := ForOS(goos, mode, goldenWorkspace(), config.Claude{}, goldenHomes[goos], true)
			for _, w := range want {
				if !has(f.Disallowed, w) {
					t.Errorf("%s %s: %s is not denied", goos, mode, w)
				}
			}
		}
	}
	if has(BuiltinDeniedFor("darwin"), "PowerShell") || has(BuiltinDeniedFor("linux"), "Bash(brew *)") || has(BuiltinDeniedFor("windows"), "Bash(apt *)") {
		t.Fatal("an OS got another OS's rules")
	}
	// The workspace defaults are rules only (config validation needs "Tool(...)").
	for _, r := range DeniedCommandsFor("windows") {
		if !strings.HasPrefix(r, "Bash(") || !strings.HasSuffix(r, ")") {
			t.Errorf("not a command rule: %s", r)
		}
	}
}

func TestSecretRulesPerOS(t *testing.T) {
	cases := map[string][]string{
		"linux":   {"Read(//home/you/.local/share/keyrings/**)", "Read(//home/you/.ssh/**)", "Edit(//home/you/.config)"},
		"windows": {"Read(//c/Users/you/AppData/Roaming/clawdyx/**)", "Read(//c/Users/you/.ssh/**)", "Read(//c/Users/you/AppData/Local/clawdyx/**)"},
		"darwin":  {"Read(//Users/you/Library/Keychains/**)", "Read(//Users/you/Library/Application Support/clawdyx/**)"},
	}
	for goos, want := range cases {
		rules := secretRulesFor(goos, goldenHomes[goos])
		for _, w := range want {
			if !has(rules, w) {
				t.Errorf("%s: missing %s in %v", goos, w, rules[:8])
			}
		}
	}
	if r := secretRulesFor("windows", `D:\Users\Иван Петров`)[0]; r != "Read(//d/Users/Иван Петров/.ssh/**)" {
		t.Fatal(r)
	}
}
