package telegram

import (
	"strings"
	"testing"

	"github.com/Publicthrone547/clawdyX/internal/platform"
)

func TestSessionCheck(t *testing.T) {
	cases := map[platform.Session]string{
		platform.SessionX11:      "X11",
		platform.SessionWayland:  "screenshots only",
		platform.SessionHeadless: "none (a server)",
	}
	for s, want := range cases {
		c := sessionCheck(platform.Info{OS: "linux", Session: s})
		if !c.OK || !strings.Contains(c.Detail, want) {
			t.Errorf("%s: %+v", s, c)
		}
		if bad := platform.ForbiddenWords("linux", c.Label+" "+c.Detail+" "+c.Fix); len(bad) > 0 {
			t.Errorf("%s: %v", s, bad)
		}
	}
}
