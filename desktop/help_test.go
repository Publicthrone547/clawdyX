package main

import (
	"html"
	"io/fs"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Publicthrone547/clawdyX/desktop/host"
	"github.com/Publicthrone547/clawdyX/internal/platform"
)

// helpLinkKeys are the open: keys a topic may use per OS: the app's own
// links plus that OS's settings pages (host.SettingsLinks there).
var helpLinkKeys = map[string][]string{
	"darwin":  {"screen-recording", "accessibility", "automation", "camera", "login-items"},
	"windows": {"camera", "startup-apps"},
	"linux":   {},
}

var (
	hrefRe = regexp.MustCompile(`href="([^"]*)"`)
	tagRe  = regexp.MustCompile(`<[^>]+>`)
)

func helpSessions(os string) []string {
	if os == "linux" {
		return []string{"x11", "wayland", "headless"}
	}
	return []string{""}
}

func TestHelpRendersForEveryOS(t *testing.T) {
	files, _ := fs.Glob(helpFS, "help/*.md")
	if len(files) != len(helpTopics) {
		t.Fatalf("%d files for %d topics", len(files), len(helpTopics))
	}
	for _, w := range platform.All {
		keys := map[string]bool{}
		for k := range links {
			if _, settings := host.SettingsLinks[k]; !settings {
				keys[k] = true // the app's own links, the same on every OS
			}
		}
		for _, k := range helpLinkKeys[w.OS] {
			keys[k] = true
		}
		for _, session := range helpSessions(w.OS) {
			for _, topic := range helpTopics {
				name := w.OS + "/" + session + "/" + topic.ID
				start := time.Now()
				p, err := renderHelp(topic.ID, newHelpData(w, session))
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				if d := time.Since(start); d > 100*time.Millisecond {
					t.Errorf("%s: rendering took %v", name, d)
				}
				if strings.Contains(p.HTML, "raw HTML omitted") || strings.Contains(p.HTML, "<no value>") {
					t.Errorf("%s: raw HTML or a missing value", name)
				}
				for _, m := range hrefRe.FindAllStringSubmatch(p.HTML, -1) {
					href := html.UnescapeString(m[1])
					switch {
					case strings.HasPrefix(href, "help:"):
						if _, err := renderHelp(strings.TrimPrefix(href, "help:"), newHelpData(w, session)); err != nil {
							t.Errorf("%s: link to %s: %v", name, href, err)
						}
					case strings.HasPrefix(href, "open:"):
						if !keys[strings.TrimPrefix(href, "open:")] {
							t.Errorf("%s: unknown link %s", name, href)
						}
					default:
						t.Errorf("%s: link %q is neither help: nor open:", name, href)
					}
				}
				text := html.UnescapeString(tagRe.ReplaceAllString(p.HTML, " "))
				if bad := platform.ForbiddenWords(w.OS, text); len(bad) > 0 {
					t.Errorf("%s: %v", name, bad)
				}
			}
		}
	}
}

// The settings pages a topic links to exist on the OS that runs the test.
func TestHelpLinkKeysMatchThisOS(t *testing.T) {
	for _, k := range helpLinkKeys[platform.W.OS] {
		if _, ok := host.SettingsLinks[k]; !ok {
			t.Errorf("Help links to %q, which this OS does not have", k)
		}
	}
}

func TestHelpUnknownTopic(t *testing.T) {
	if _, err := renderHelp("nope", newHelpData(platform.W, "")); err == nil {
		t.Fatal("an unknown topic rendered")
	}
}
