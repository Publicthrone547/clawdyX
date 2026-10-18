package platform

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The user docs keep one section per OS (### macOS, ### Windows, ### Linux,
// and in the RUNBOOK ## macOS and so on); a section for one OS must not
// name the others. Sections for every OS (tables, the comparisons) may.
var docFiles = []string{"README.md", "docs/RUNBOOK.md", "docs/PRIVACY.md"}

var osHeading = regexp.MustCompile(`^(#{2,4}) (macOS|Windows|Linux)\b(.*)$`)

var headingOS = map[string]string{"macOS": "darwin", "Windows": "windows", "Linux": "linux"}

// osSections returns the text of each OS section of a Markdown document,
// keyed by "heading (line N)", with the OS it belongs to.
func osSections(doc string) map[string][2]string {
	out := map[string][2]string{}
	lines := strings.Split(doc, "\n")
	inCode := false
	for i := 0; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "```") {
			inCode = !inCode
		}
		m := osHeading.FindStringSubmatch(lines[i])
		if inCode || m == nil {
			continue
		}
		level := len(m[1])
		var body []string
		code := false
		for j := i + 1; j < len(lines); j++ {
			l := lines[j]
			if strings.HasPrefix(l, "```") {
				code = !code
			}
			if !code && strings.HasPrefix(l, "#") {
				if n := len(l) - len(strings.TrimLeft(l, "#")); n <= level {
					break
				}
			}
			body = append(body, l)
		}
		out[lines[i]+" (line "+strconv.Itoa(i+1)+")"] = [2]string{headingOS[m[2]], strings.Join(body, "\n")}
	}
	return out
}

func TestDocsOSSectionsNameOnlyTheirOS(t *testing.T) {
	for _, f := range docFiles {
		b, err := os.ReadFile(filepath.Join("..", "..", f))
		if err != nil {
			t.Fatal(err)
		}
		secs := osSections(string(b))
		if f == "README.md" && len(secs) < 9 {
			t.Errorf("%s: %d OS sections; Install, Uninstall and Build from source need one per OS", f, len(secs))
		}
		for name, s := range secs {
			if bad := ForbiddenWords(s[0], s[1]); len(bad) > 0 {
				t.Errorf("%s %s: %v", f, name, bad)
			}
		}
	}
}

func TestOSSections(t *testing.T) {
	doc := "## Install\n### macOS\nopen the dmg\n```sh\n# not a heading\n```\n### Windows\nrun setup.exe\n#### Notes\nmore\n## Next\nall\n"
	s := osSections(doc)
	if len(s) != 2 {
		t.Fatalf("%d sections: %v", len(s), s)
	}
	for name, v := range s {
		switch {
		case strings.HasPrefix(name, "### macOS"):
			if v[0] != "darwin" || !strings.Contains(v[1], "# not a heading") || strings.Contains(v[1], "setup.exe") {
				t.Errorf("macOS: %q", v[1])
			}
		case strings.HasPrefix(name, "### Windows"):
			if v[0] != "windows" || !strings.Contains(v[1], "more") || strings.Contains(v[1], "all") {
				t.Errorf("Windows: %q", v[1])
			}
		}
	}
}
