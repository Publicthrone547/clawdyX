// Package render converts Claude's Markdown output to Telegram HTML and
// splits it into message-sized chunks. Everything that is not a tag the bot
// generates itself is HTML-escaped, so model output can never inject markup.
package render

import (
	"html"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Limit is the per-message budget in characters of HTML source. Telegram's
// limit is 4096 characters after entity parsing; raw HTML length is a
// conservative upper bound.
const Limit = 4000

// Esc escapes text for Telegram HTML.
func Esc(s string) string { return html.EscapeString(s) }

type block struct {
	code bool
	lang string
	text string // raw text (code content or a paragraph of lines)
}

var fenceRe = regexp.MustCompile("^\\s*```\\s*([A-Za-z0-9_+.#-]*)\\s*$")

func parse(md string) []block {
	var out []block
	var cur []string
	inCode := false
	lang := ""
	flushText := func() {
		if len(cur) > 0 {
			out = append(out, block{text: strings.Join(cur, "\n")})
			cur = nil
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n") {
		if m := fenceRe.FindStringSubmatch(line); m != nil {
			if !inCode {
				flushText()
				inCode, lang = true, m[1]
				continue
			}
			out = append(out, block{code: true, lang: lang, text: strings.Join(cur, "\n")})
			cur, inCode, lang = nil, false, ""
			continue
		}
		cur = append(cur, line)
	}
	if inCode {
		out = append(out, block{code: true, lang: lang, text: strings.Join(cur, "\n")})
	} else {
		flushText()
	}
	return out
}

var (
	boldRe    = regexp.MustCompile(`\*\*([^*\n]+?)\*\*`)
	linkRe    = regexp.MustCompile(`\[([^\]\n]+)\]\((https?://[^\s)]+)\)`)
	headingRe = regexp.MustCompile(`^#{1,6}\s+(.*)$`)
)

func inline(line string) string {
	if m := headingRe.FindStringSubmatch(line); m != nil {
		return "<b>" + inline(m[1]) + "</b>"
	}
	var sb strings.Builder
	parts := strings.Split(line, "`")
	for i, p := range parts {
		odd := i%2 == 1
		if odd && i < len(parts)-1 {
			sb.WriteString("<code>" + Esc(p) + "</code>")
			continue
		}
		if odd { // unmatched backtick: restore it
			sb.WriteString("`")
		}
		e := Esc(p)
		e = linkRe.ReplaceAllString(e, `<a href="$2">$1</a>`)
		e = boldRe.ReplaceAllString(e, "<b>$1</b>")
		sb.WriteString(e)
	}
	return sb.String()
}

func renderCode(lang, code string) string {
	if lang != "" {
		return `<pre><code class="language-` + Esc(lang) + `">` + Esc(code) + "</code></pre>"
	}
	return "<pre>" + Esc(code) + "</pre>"
}

// splitRunes splits s into pieces whose escaped size stays below n.
func splitRunes(s string, n int) []string {
	var out []string
	for len(s) > 0 {
		size, cut := 0, 0
		for cut < len(s) {
			r, w := utf8.DecodeRuneInString(s[cut:])
			add := len(Esc(string(r)))
			if size+add > n {
				break
			}
			size += add
			cut += w
		}
		if cut == 0 {
			_, cut = utf8.DecodeRuneInString(s)
		}
		out = append(out, s[:cut])
		s = s[cut:]
	}
	return out
}

// pieces renders blocks into HTML fragments each no larger than limit.
func pieces(blocks []block, limit int) []string {
	var out []string
	for _, b := range blocks {
		if b.code {
			overhead := len(renderCode(b.lang, ""))
			var cur []string
			size := 0
			flush := func() {
				if len(cur) > 0 {
					out = append(out, renderCode(b.lang, strings.Join(cur, "\n")))
					cur, size = nil, 0
				}
			}
			for _, line := range strings.Split(b.text, "\n") {
				for _, seg := range splitRunes(line, limit-overhead-1) {
					l := len(Esc(seg)) + 1
					if size+l+overhead > limit {
						flush()
					}
					cur = append(cur, seg)
					size += l
				}
			}
			flush()
			if len(out) == 0 || b.text == "" {
				out = append(out, renderCode(b.lang, b.text))
			}
			continue
		}
		var cur []string
		size := 0
		flush := func() {
			if len(cur) > 0 {
				out = append(out, strings.Join(cur, "\n"))
				cur, size = nil, 0
			}
		}
		for _, line := range strings.Split(b.text, "\n") {
			r := inline(line)
			if len(r) > limit {
				flush()
				for _, seg := range splitRunes(line, limit) {
					out = append(out, Esc(seg))
				}
				continue
			}
			if size+len(r)+1 > limit {
				flush()
			}
			cur = append(cur, r)
			size += len(r) + 1
		}
		flush()
	}
	return out
}

// Chunk converts Markdown to Telegram HTML messages, each at most limit long.
func Chunk(md string, limit int) []string {
	if limit <= 0 {
		limit = Limit
	}
	md = strings.TrimSpace(md)
	if md == "" {
		return nil
	}
	var msgs []string
	var cur strings.Builder
	for _, p := range pieces(parse(md), limit) {
		if strings.TrimSpace(p) == "" && cur.Len() == 0 {
			continue
		}
		if cur.Len() > 0 && cur.Len()+1+len(p) > limit {
			msgs = append(msgs, strings.TrimSpace(cur.String()))
			cur.Reset()
		}
		if cur.Len() > 0 {
			cur.WriteString("\n")
		}
		cur.WriteString(p)
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		msgs = append(msgs, s)
	}
	return msgs
}

// ChunkPlain splits plain text (no parse mode) into pieces of at most limit runes.
func ChunkPlain(s string, limit int) []string {
	if limit <= 0 {
		limit = Limit
	}
	return splitLinesBudget(s, limit)
}

func splitLinesBudget(s string, limit int) []string {
	var out []string
	var cur strings.Builder
	n := 0
	for _, line := range strings.Split(s, "\n") {
		for utf8.RuneCountInString(line) > limit {
			r := []rune(line)
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
				n = 0
			}
			out = append(out, string(r[:limit]))
			line = string(r[limit:])
		}
		l := utf8.RuneCountInString(line) + 1
		if n+l > limit && cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
			n = 0
		}
		if cur.Len() > 0 {
			cur.WriteString("\n")
		}
		cur.WriteString(line)
		n += l
	}
	if strings.TrimSpace(cur.String()) != "" {
		out = append(out, cur.String())
	}
	return out
}

var tagRe = regexp.MustCompile(`<[^>]+>`)

// StripHTML converts bot-generated HTML back to plain text.
func StripHTML(s string) string { return html.UnescapeString(tagRe.ReplaceAllString(s, "")) }

// Tail returns the last n runes of s, starting at a line boundary when possible.
func Tail(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	t := string(r[len(r)-n:])
	if i := strings.IndexByte(t, '\n'); i >= 0 && i < len(t)/3 {
		t = t[i+1:]
	}
	return "…" + t
}
