package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/control"
	"github.com/Publicthrone547/clawdyX/internal/platform"
	"github.com/Publicthrone547/clawdyX/internal/policy"
	"github.com/Publicthrone547/clawdyX/internal/setup"
)

func fakeClaude(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "claude"+platform.ExeSuffix)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// writeConfig writes a valid config made by setup and returns its path.
func writeConfig(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.json")
	if err := setup.WriteConfig(p, setup.GenerateConfig(111111111, t.TempDir(), "Firefox", fakeClaude(t))); err != nil {
		t.Fatal(err)
	}
	return p
}

func load(t *testing.T, p string) *config.Config {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	c, err := config.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func never(string) bool { return false }

func ptr[T any](v T) *T { return &v }

func TestEveryKeyRoundTrips(t *testing.T) {
	p := writeConfig(t)
	state, logs := t.TempDir(), t.TempDir()
	child := []string{filepath.Dir(fakeClaude(t)), t.TempDir()}
	patch := control.SettingsPatch{
		PollTimeoutSeconds: ptr(40), SettingSources: ptr([]string{"user", "project"}), StrictMCPConfig: ptr(false),
		MaxConcurrentJobs: ptr(3), JobTimeoutMinutes: ptr(20), MaxTurns: ptr(80), MaxBudgetUSD: ptr(5.0),
		MaxQueuePerSession: ptr(9), MaxOutputBytes: ptr(2 << 20), MaxChildRSSMB: ptr(1024), BgWaitCeilingMS: ptr(30000),
		ChildPath: &child, ExtraDisallowedTools: ptr([]string{"Bash(make deploy *)", "NotebookEdit"}),
		AutomationEnabled: ptr(true), ScreenControl: ptr(true), OpenURLs: ptr(false), BrowserApp: ptr("Google Chrome"),
		ShutdownCommand: ptr(true), StateDir: &state, LogDir: &logs, LogLevel: ptr("debug"),
		JobsDays: ptr(30), JobEventsDays: ptr(7), AuditDays: ptr(60), ProcessedUpdatesDays: ptr(3),
	}
	c, err := Save(p, patch, never)
	if err != nil {
		t.Fatal(err)
	}
	si := Info(load(t, p), p, "", false)
	got, _ := json.Marshal(si)
	for _, want := range []string{`"pollTimeoutSeconds":40`, `"settingSources":["user","project"]`, `"strictMcpConfig":false`,
		`"maxConcurrentJobs":3`, `"jobTimeoutMinutes":20`, `"maxTurns":80`, `"maxBudgetUsd":5`, `"maxQueuePerSession":9`,
		`"maxOutputBytes":2097152`, `"maxChildRssMb":1024`, `"bgWaitCeilingMs":30000`, `"automationEnabled":true`,
		`"screenControl":true`, `"openUrls":false`, `"browserApp":"Google Chrome"`, `"shutdownCommand":true`, `"logLevel":"debug"`,
		`"jobsDays":30`, `"jobEventsDays":7`, `"auditDays":60`, `"processedUpdatesDays":3`} {
		if !strings.Contains(string(got), want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Join(si.ChildPath, "|") != strings.Join(child, "|") || si.StateDir != state || si.LogDir != logs || c.Claude.ExtraDisallowedTools[1] != "NotebookEdit" {
		t.Fatalf("lists and folders: %+v", si)
	}
	if len(si.BuiltinDenied) == 0 || si.BuiltinDenied[0] != policy.BuiltinDenied()[0] {
		t.Fatal("built-in deny list missing")
	}
}

func TestRejectedValuesChangeNothing(t *testing.T) {
	p := writeConfig(t)
	before, _ := os.ReadFile(p)
	for name, patch := range map[string]control.SettingsPatch{
		"poll timeout":   {PollTimeoutSeconds: ptr(2)},
		"budget":         {MaxBudgetUSD: ptr(1.0)},
		"queue":          {MaxQueuePerSession: ptr(99)},
		"output":         {MaxOutputBytes: ptr(10)},
		"bg wait":        {BgWaitCeilingMS: ptr(900000)},
		"relative path":  {ChildPath: ptr([]string{"bin"})},
		"empty path":     {ChildPath: ptr([]string{})},
		"bad rule":       {ExtraDisallowedTools: ptr([]string{"rm -rf"})},
		"bad source":     {SettingSources: ptr([]string{"everything"})},
		"claude missing": {ClaudeBinary: ptr(filepath.Join(t.TempDir(), "nope"))},
		"state relative": {StateDir: ptr("state")},
	} {
		if _, err := Save(p, patch, never); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if after, _ := os.ReadFile(p); string(after) != string(before) {
		t.Fatal("a rejected patch changed the file")
	}
}

func TestWorkspaceRulesAndRename(t *testing.T) {
	p := writeConfig(t)
	c := load(t, p)
	name := c.Workspaces[0].Name
	allowed := append([]string{}, c.Workspaces[0].AllowedCommands...)
	allowed = append(allowed, "Bash(npm test *)")
	if _, err := Save(p, control.SettingsPatch{WorkspaceRules: map[string]control.WorkspaceRulesPatch{name: {Allowed: &allowed}}}, never); err != nil {
		t.Fatal(err)
	}
	if !contains(load(t, p).Workspaces[0].AllowedCommands, "Bash(npm test *)") {
		t.Fatal("allowed rule not saved")
	}
	// Removing a minimum rule without the opt-out is refused by the worker's validation.
	_, err := Save(p, control.SettingsPatch{WorkspaceRules: map[string]control.WorkspaceRulesPatch{name: {Denied: ptr([]string{"Bash(sudo *)", "Bash(git push *)"})}}}, never)
	if err == nil || !strings.Contains(err.Error(), "must include Bash(rm *)") {
		t.Fatalf("minimum deny rules: %v", err)
	}
	if _, err := Save(p, control.SettingsPatch{WorkspaceRules: map[string]control.WorkspaceRulesPatch{name: {Denied: ptr([]string{"Bash(curl *)"}), IKnowWhatIAmDoing: ptr(true)}}}, never); err != nil {
		t.Fatalf("with the opt-out: %v", err)
	}
	if _, err := Save(p, control.SettingsPatch{RenameWorkspace: &control.RenameParams{From: name, To: "renamed"}}, func(string) bool { return true }); err == nil {
		t.Fatal("renamed a workspace in use")
	}
	c2, err := Save(p, control.SettingsPatch{RenameWorkspace: &control.RenameParams{From: name, To: "renamed"}}, never)
	if err != nil || c2.Workspaces[0].Name != "renamed" || c2.DefaultWorkspace != "renamed" {
		t.Fatalf("rename: %v", err)
	}
	if _, err := Save(p, control.SettingsPatch{RenameWorkspace: &control.RenameParams{From: "renamed", To: "Bad Name"}}, never); err == nil {
		t.Fatal("invalid name accepted")
	}
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func TestResetSections(t *testing.T) {
	p := writeConfig(t)
	if _, err := Save(p, control.SettingsPatch{MaxTurns: ptr(99), MaxConcurrentJobs: ptr(4), LogLevel: ptr("debug"), ScreenControl: ptr(true)}, never); err != nil {
		t.Fatal(err)
	}
	c, err := ResetFile(p, control.SectionLimits)
	if err != nil {
		t.Fatal(err)
	}
	d := config.Defaults()
	if c.Claude.MaxTurns != d.Claude.MaxTurns || c.Claude.MaxConcurrentJobs != d.Claude.MaxConcurrentJobs {
		t.Fatal("limits not reset")
	}
	if c.Logging.Level != "debug" || !c.Desktop.ScreenControl {
		t.Fatal("other sections must stay")
	}
	c, err = ResetFile(p, control.SectionAll)
	if err != nil || c.Logging.Level != "info" || c.Desktop.ScreenControl || c.Telegram.OwnerUserID != 111111111 || len(c.Workspaces) != 1 {
		t.Fatalf("all: %v %+v", err, c)
	}
	if _, err := ResetFile(p, "bogus"); err == nil {
		t.Fatal("unknown section accepted")
	}
}

func TestExportImport(t *testing.T) {
	p := writeConfig(t)
	ex, err := Export(p)
	if err != nil || !strings.HasPrefix(ex.Filename, "clawdyx-settings-") || config.ContainsToken(ex.JSON) {
		t.Fatalf("export: %v", err)
	}
	// Importing the exported file is a no-op.
	res, _, err := Import(p, control.SettingsImportParams{JSON: ex.JSON, Apply: true})
	if err != nil || len(res.Changes) != 0 || res.OwnerChanged {
		t.Fatalf("no-op import: %v %+v", err, res)
	}
	// A changed document lists what changes and needs Apply.
	var doc map[string]any
	_ = json.Unmarshal([]byte(ex.JSON), &doc)
	doc["claude"].(map[string]any)["max_turns"] = 77
	changed, _ := json.Marshal(doc)
	res, _, err = Import(p, control.SettingsImportParams{JSON: string(changed)})
	if err != nil || len(res.Changes) != 1 || res.Changes[0] != "claude.max_turns: 60 → 77" || res.Applied {
		t.Fatalf("preview: %v %+v", err, res)
	}
	if load(t, p).Claude.MaxTurns != 60 {
		t.Fatal("a preview changed the file")
	}
	// Another owner needs confirmation.
	doc["telegram"].(map[string]any)["owner_user_id"] = 222222222
	other, _ := json.Marshal(doc)
	if _, _, err := Import(p, control.SettingsImportParams{JSON: string(other), Apply: true}); err == nil {
		t.Fatal("owner change imported without confirmation")
	}
	if res, _, err := Import(p, control.SettingsImportParams{JSON: string(other), Apply: true, AllowOwnerChange: true}); err != nil || !res.Applied || !res.OwnerChanged {
		t.Fatalf("confirmed: %v %+v", err, res)
	}
	// An invalid document changes nothing.
	before, _ := os.ReadFile(p)
	for _, bad := range []string{"{", `{"bogus":1}`, strings.Replace(string(changed), `"max_turns":77`, `"max_turns":9999`, 1)} {
		if _, _, err := Import(p, control.SettingsImportParams{JSON: bad, Apply: true}); err == nil {
			t.Errorf("invalid import accepted: %.40s", bad)
		}
	}
	if after, _ := os.ReadFile(p); string(after) != string(before) {
		t.Fatal("a rejected import changed the file")
	}
	if _, _, err := Import(p, control.SettingsImportParams{JSON: `{"x":"123456789:` + strings.Repeat("A", 35) + `"}`}); err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("a token in the file must be refused: %v", err)
	}
}

func TestLoadAndRepairAnInvalidFile(t *testing.T) {
	p := writeConfig(t)
	c := load(t, p)
	// Break it: claude.binary points nowhere (the worker refuses to start).
	raw, _ := os.ReadFile(p)
	b, _ := json.Marshal(c.Claude.Binary)
	broken := strings.Replace(string(raw), string(b), `"`+filepath.ToSlash(filepath.Join(t.TempDir(), "gone"))+`"`, 1)
	os.WriteFile(p, []byte(broken), 0o600)
	lc, invalid, err := Load(p)
	if err != nil || invalid == nil || !strings.Contains(invalid.Error(), "claude.binary") || lc == nil {
		t.Fatalf("lenient load: %v %v", err, invalid)
	}
	// Repairing it through Save works although the current file is invalid.
	if _, err := Save(p, control.SettingsPatch{ClaudeBinary: &c.Claude.Binary}, never); err != nil {
		t.Fatalf("repair: %v", err)
	}
	if _, invalid, _ := Load(p); invalid != nil {
		t.Fatalf("still invalid: %v", invalid)
	}
}
