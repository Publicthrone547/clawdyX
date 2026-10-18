package main

import (
	"strings"
	"testing"

	"github.com/Publicthrone547/clawdyX/internal/config"
)

func TestUnitQuotesPathsWithSpaces(t *testing.T) {
	exe := "/home/jöhn doe/bin/clawdyx"
	cfg := `/home/jöhn doe/.config/clawdyx/my "odd" 100% $cfg.json`
	u := Unit(exe, cfg, "/home/jöhn doe/.local/bin:/usr/bin:/bin")
	want := `ExecStart="/home/jöhn doe/bin/clawdyx" run -config "/home/jöhn doe/.config/clawdyx/my \"odd\" 100%% $$cfg.json"`
	if !strings.Contains(u, want+"\n") {
		t.Fatalf("ExecStart line:\n%s", u)
	}
	for _, s := range []string{"Restart=on-failure", "RestartSec=10", "StartLimitIntervalSec=0", "WantedBy=default.target", `Environment="PATH=/home/jöhn doe/.local/bin:/usr/bin:/bin"`} {
		if !strings.Contains(u, s) {
			t.Errorf("unit misses %s", s)
		}
	}
	if strings.Contains(u, "TELEGRAM_BOT_TOKEN") {
		t.Fatal("the token must not be in the unit")
	}
	if got := config.UnitProgram(u); got != exe {
		t.Fatalf("unitProgram %q", got)
	}
	if got := config.UnitProgram("[Service]\nExecStart=/usr/bin/clawdyx run\n"); got != "/usr/bin/clawdyx" {
		t.Fatal(got)
	}
}

func TestDesktopEntryAndShowParsing(t *testing.T) {
	e := desktopEntry("/opt/clawd yx/clawdyx", "/home/a/.config/clawdyx/config.json")
	if !strings.Contains(e, `Exec="/opt/clawd yx/clawdyx" service supervise -config "/home/a/.config/clawdyx/config.json"`) {
		t.Fatal(e)
	}
	m := parseShow("ActiveState=active\nSubState=running\nMainPID=4242\nExecMainStartTimestamp=Sat 2026-10-18 10:00:00 UTC\n")
	if m["ActiveState"] != "active" || m["MainPID"] != "4242" || !strings.HasPrefix(m["ExecMainStartTimestamp"], "Sat") {
		t.Fatalf("%v", m)
	}
}

func TestUnitNameFollowsLabel(t *testing.T) {
	old := serviceLabel
	defer func() { serviceLabel = old }()
	for label, want := range map[string]string{"local.clawdyx": "clawdyx", "local.clawdyx.dev": "clawdyx-dev", "mybot": "mybot"} {
		serviceLabel = label
		if got := unitName(); got != want {
			t.Errorf("%s → %s want %s", label, got, want)
		}
	}
}
