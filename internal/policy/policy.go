// Package policy maps permission tiers to Claude CLI flags and enforces
// workspace path restrictions. It is the single place that decides what a
// Claude child process may do.
package policy

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Publicthrone547/clawdyX/internal/config"
)

// Flags is the permission-related part of a claude invocation.
type Flags struct {
	PermissionMode string   // value for --permission-mode
	Allowed        []string // --allowedTools entries
	Disallowed     []string // --disallowedTools entries
	SettingsJSON   string   // value for --settings
}

// ErrModeDisabled is returned for tiers that are not enabled in this build.
var ErrModeDisabled = errors.New("automation mode is not enabled")

// alwaysDisallowedTools are removed from every session: network tools that
// could exfiltrate data outside the ordinary Claude request flow.
var alwaysDisallowedTools = []string{"WebFetch", "WebSearch"}

// observeDisallowedTools are additionally removed in observe mode.
var observeDisallowedTools = []string{"Edit", "Write", "NotebookEdit"}

// SecretPaths returns home-relative locations whose contents must never be
// read or written by Claude, regardless of tier.
func SecretPaths(home string) []string {
	rel := []string{
		".ssh", ".aws", ".gnupg", ".kube", ".docker", ".netrc", ".npmrc", ".pypirc",
		".config", ".claude", ".claude.json", ".git-credentials", ".zsh_history", ".bash_history",
		"Library/Keychains", "Library/Cookies",
		"Library/Application Support/Google", "Library/Application Support/Firefox",
		"Library/Application Support/BraveSoftware", "Library/Safari",
		"Library/Application Support/clawdyx", "Library/Application Support/claude-tg",
	}
	rel = append(rel, platformSecretPaths...)
	out := make([]string, 0, len(rel))
	for _, r := range rel {
		out = append(out, filepath.Join(home, r))
	}
	return out
}

func secretRules(home string) []string {
	var rules []string
	for _, p := range SecretPaths(home) {
		abs := ruleAbs(p)
		rules = append(rules, "Read("+abs+"/**)", "Read("+abs+")", "Edit("+abs+"/**)", "Edit("+abs+")")
	}
	return rules
}

// For computes the permission flags for a mode in a workspace.
//
// automationEnabled must come from the config; it is re-checked for every job.
func For(mode string, ws *config.Workspace, cl config.Claude, home string, automationEnabled bool) (Flags, error) {
	perms := map[string]any{"disableBypassPermissionsMode": "disable"}
	if mode != config.ModeAutomation {
		// Outside automation, auto mode is unavailable and file tools may not
		// read outside the workspace. In automation, Claude Code's classifier
		// judges such reads; secret paths and destructive commands stay denied.
		perms["disableAutoMode"] = "disable"
		perms["blockReadsOutsideWorkingDirectories"] = true
	}
	sj, err := json.Marshal(map[string]any{"permissions": perms})
	if err != nil {
		return Flags{}, err
	}
	f := Flags{SettingsJSON: string(sj)}
	var deny []string
	for _, t := range alwaysDisallowedTools {
		// Automation may search the web (to find a specific video, page or
		// app); fetching arbitrary pages stays denied in every tier.
		if mode == config.ModeAutomation && t == "WebSearch" {
			continue
		}
		deny = append(deny, t)
	}
	deny = append(deny, secretRules(home)...)
	deny = append(deny, ws.DeniedCommands...)
	deny = append(deny, cl.ExtraDisallowedTools...)

	switch mode {
	case config.ModeObserve:
		f.PermissionMode = "dontAsk"
		f.Disallowed = append(append([]string{}, observeDisallowedTools...), deny...)
	case config.ModeAssisted:
		f.PermissionMode = "acceptEdits"
		f.Allowed = append([]string{}, ws.AllowedCommands...)
		f.Disallowed = deny
	case config.ModeAutomation:
		if !automationEnabled {
			return Flags{}, ErrModeDisabled
		}
		f.PermissionMode = "auto"
		f.Allowed = append([]string{}, ws.AllowedCommands...)
		f.Disallowed = deny
	default:
		return Flags{}, fmt.Errorf("unknown mode %q", mode)
	}
	f.Disallowed = dedupe(f.Disallowed)
	f.Allowed = dedupe(f.Allowed)
	for _, a := range f.Allowed {
		if a == "Bash" || a == "*" || strings.HasPrefix(a, "Bash(*") {
			return Flags{}, fmt.Errorf("allow rule %q is too broad", a)
		}
	}
	return f, nil
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, v := range in {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// Within reports whether path (after symlink resolution) lies inside root.
// root must already be canonical. Non-existent paths are rejected.
func Within(root, path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be absolute")
	}
	canon, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("cannot resolve path: %w", err)
	}
	canon = filepath.Clean(canon)
	rel, err := filepath.Rel(root, canon)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path escapes the workspace")
	}
	return canon, nil
}

// CheckWorkspaceDir re-validates a workspace directory right before use
// (it may have been replaced by a symlink since startup).
func CheckWorkspaceDir(ws *config.Workspace) (string, error) {
	canon, err := filepath.EvalSymlinks(ws.Path)
	if err != nil {
		return "", fmt.Errorf("workspace %s unavailable: %w", ws.Name, err)
	}
	canon = filepath.Clean(canon)
	if canon != ws.Canonical {
		return "", fmt.Errorf("workspace %s now resolves elsewhere; refusing", ws.Name)
	}
	st, err := os.Stat(canon)
	if err != nil || !st.IsDir() {
		return "", fmt.Errorf("workspace %s is not a directory", ws.Name)
	}
	return canon, nil
}

// DisplayPath shows a path only if it is inside one of the roots; otherwise a
// placeholder. Roots must be canonical.
func DisplayPath(p string, roots []string) string {
	clean := filepath.Clean(p)
	for _, r := range roots {
		if clean == r || strings.HasPrefix(clean, r+string(filepath.Separator)) {
			return clean
		}
	}
	return "<outside workspace>"
}
