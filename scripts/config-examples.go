//go:build ignore

// config-examples writes config.example.json (macOS), config.example.windows.json
// and config.example.linux.json from the code's defaults, so the examples
// show each OS's built-in deny rules exactly: go run scripts/config-examples.go
package main

import (
	"encoding/json"
	"log"
	"os"

	"github.com/Publicthrone547/clawdyX/internal/policy"
)

// The types keep the key order of config.json.
type (
	telegram struct {
		OwnerUserID        int64 `json:"owner_user_id"`
		PollTimeoutSeconds int   `json:"poll_timeout_seconds"`
	}
	claude struct {
		Binary               string   `json:"binary"`
		DefaultModel         string   `json:"default_model"`
		DefaultEffort        string   `json:"default_effort"`
		SettingSources       string   `json:"setting_sources"`
		StrictMCPConfig      bool     `json:"strict_mcp_config"`
		JobTimeout           string   `json:"job_timeout"`
		MaxTurns             int      `json:"max_turns"`
		MaxBudgetUSD         float64  `json:"max_budget_usd"`
		MaxConcurrentJobs    int      `json:"max_concurrent_jobs"`
		MaxQueuePerSession   int      `json:"max_queue_per_session"`
		MaxOutputBytes       int      `json:"max_output_bytes"`
		MaxChildRSSMB        int      `json:"max_child_rss_mb"`
		BgWaitCeilingMS      int      `json:"bg_wait_ceiling_ms"`
		ChildPath            string   `json:"child_path"`
		ExtraDisallowedTools []string `json:"extra_disallowed_tools"`
	}
	workspace struct {
		Name            string   `json:"name"`
		Path            string   `json:"path"`
		DefaultMode     string   `json:"default_mode"`
		AllowedCommands []string `json:"allowed_commands"`
		DeniedCommands  []string `json:"denied_commands"`
	}
	desktop struct {
		OpenURLs      bool   `json:"open_urls"`
		BrowserApp    string `json:"browser_app"`
		ScreenControl bool   `json:"screen_control"`
		Camera        bool   `json:"camera"`
	}
	doc struct {
		Telegram               telegram    `json:"telegram"`
		Claude                 claude      `json:"claude"`
		Workspaces             []workspace `json:"workspaces"`
		DefaultWorkspace       string      `json:"default_workspace"`
		ShutdownCommandEnabled bool        `json:"shutdown_command_enabled"`
		Modes                  struct {
			AutomationEnabled bool `json:"automation_enabled"`
		} `json:"modes"`
		Desktop   desktop           `json:"desktop"`
		StateDir  string            `json:"state_dir"`
		LogDir    string            `json:"log_dir"`
		Logging   map[string]string `json:"logging"`
		Retention map[string]int    `json:"retention"`
	}
)

func main() {
	for goos, ex := range map[string]struct{ file, claude, childPath, workspace, browser string }{
		"darwin":  {"config.example.json", "/opt/homebrew/bin/claude", "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin", "/Users/you/projects/my-project", "Firefox"},
		"windows": {"config.example.windows.json", `C:\Users\you\.local\bin\claude.exe`, `C:\Users\you\.local\bin;C:\Program Files\Git\cmd;C:\Program Files\nodejs;C:\Windows\System32;C:\Windows`, `C:\Users\you\projects\my-project`, "default"},
		"linux":   {"config.example.linux.json", "/home/you/.local/bin/claude", "/home/you/.local/bin:/usr/local/bin:/usr/bin:/bin", "/home/you/projects/my-project", "default"},
	} {
		d := doc{
			Telegram: telegram{PollTimeoutSeconds: 30},
			Claude: claude{Binary: ex.claude, DefaultModel: "fable", DefaultEffort: "auto", SettingSources: "user", StrictMCPConfig: true,
				JobTimeout: "15m", MaxTurns: 60, MaxConcurrentJobs: 2, MaxQueuePerSession: 5, MaxOutputBytes: 1 << 20,
				MaxChildRSSMB: 2048, BgWaitCeilingMS: 60000, ChildPath: ex.childPath, ExtraDisallowedTools: []string{}},
			Workspaces: []workspace{{Name: "my-project", Path: ex.workspace, DefaultMode: "observe",
				AllowedCommands: []string{"Bash(git status *)", "Bash(git diff *)", "Bash(git log *)", "Bash(make test *)", "Bash(go test *)", "Bash(go build *)", "Bash(go vet *)"},
				DeniedCommands:  policy.DeniedCommandsFor(goos)}},
			DefaultWorkspace: "my-project",
			Desktop:          desktop{OpenURLs: true, BrowserApp: ex.browser},
			Logging:          map[string]string{"level": "info"},
			Retention:        map[string]int{"jobs_days": 90, "job_events_days": 14, "processed_updates_days": 7, "audit_days": 180},
		}
		b, err := json.MarshalIndent(d, "", "  ")
		if err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(ex.file, append(b, '\n'), 0o644); err != nil {
			log.Fatal(err)
		}
	}
}
