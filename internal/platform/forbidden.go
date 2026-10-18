package platform

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

// ForbiddenJSON is the shared word list of the wording guards (Go tests and
// the desktop app's Vitest guard read the same file).
//
//go:embed forbidden.json
var ForbiddenJSON []byte

type forbiddenFile struct {
	Forbidden map[string][]string          `json:"forbidden"`
	Allowed   map[string]map[string]string `json:"allowed"`
}

type guard struct {
	words   []string
	res     []*regexp.Regexp
	allowed []string
}

var guards = func() map[string]guard {
	var f forbiddenFile
	if err := json.Unmarshal(ForbiddenJSON, &f); err != nil {
		panic("platform: forbidden.json: " + err.Error())
	}
	out := map[string]guard{}
	for goos, words := range f.Forbidden {
		g := guard{words: words}
		for _, w := range words {
			g.res = append(g.res, WordRegexp(w))
		}
		for a := range f.Allowed["all"] {
			g.allowed = append(g.allowed, a)
		}
		for a := range f.Allowed[goos] {
			g.allowed = append(g.allowed, a)
		}
		// Longer phrases first, so "clawdyX.app" goes before a shorter one inside it.
		sort.Slice(g.allowed, func(i, j int) bool { return len(g.allowed[i]) > len(g.allowed[j]) })
		out[goos] = g
	}
	return out
}()

// WordRegexp matches w as a whole word: a word boundary is required only on
// a side where w starts or ends with a letter or digit, so ".exe" matches
// "clawdyx.exe" and "Mac" does not match "Macs".
func WordRegexp(w string) *regexp.Regexp {
	isWord := func(b byte) bool {
		return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
	}
	expr := regexp.QuoteMeta(w)
	if isWord(w[0]) {
		expr = `\b` + expr
	}
	if isWord(w[len(w)-1]) {
		expr += `\b`
	}
	return regexp.MustCompile(expr)
}

// ForbiddenWords returns the words of goos's forbidden list found in text,
// after removing the allowed phrases. An empty result means text is fine
// to show on that OS.
func ForbiddenWords(goos, text string) []string {
	g, ok := guards[goos]
	if !ok {
		return nil
	}
	for _, a := range g.allowed {
		text = strings.ReplaceAll(text, a, " ")
	}
	var found []string
	for i, re := range g.res {
		if re.MatchString(text) {
			found = append(found, g.words[i])
		}
	}
	return found
}

// AnyForbidden returns every word of every OS list found in text (used by
// the source scan, which does not know which OS a shared string is for).
func AnyForbidden(text string) []string {
	seen := map[string]bool{}
	var found []string
	for _, w := range All {
		for _, f := range ForbiddenWords(w.OS, text) {
			if !seen[f] {
				seen[f] = true
				found = append(found, f)
			}
		}
	}
	sort.Strings(found)
	return found
}
