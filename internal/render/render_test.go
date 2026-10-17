package render

import (
	"strings"
	"testing"
)

func TestEscapesModelOutput(t *testing.T) {
	out := Chunk(`<script>alert(1)</script> & <b>not bold</b>`, Limit)
	if len(out) != 1 || strings.Contains(out[0], "<script>") || strings.Contains(out[0], "<b>not") {
		t.Fatalf("unsafe: %q", out)
	}
}

func TestMarkdownConversion(t *testing.T) {
	md := "# Title\nUse **bold** and `x < y` and [docs](https://example.com/a?b=1&c=2).\n```go\nfunc main() { if a < b {} }\n```\nend `unclosed"
	out := Chunk(md, Limit)
	if len(out) != 1 {
		t.Fatalf("chunks %d", len(out))
	}
	h := out[0]
	for _, want := range []string{"<b>Title</b>", "<b>bold</b>", "<code>x &lt; y</code>", `<a href="https://example.com/a?b=1&amp;c=2">docs</a>`, `<pre><code class="language-go">func main() { if a &lt; b {} }</code></pre>`, "end `unclosed"} {
		if !strings.Contains(h, want) {
			t.Errorf("missing %q in %s", want, h)
		}
	}
	if strings.Contains(Chunk("[x](javascript:alert(1))", Limit)[0], "<a") {
		t.Fatal("non-http link rendered")
	}
}

func TestChunkingLimitsAndFences(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("intro\n```python\n")
	for i := 0; i < 600; i++ {
		sb.WriteString("print('line with <tag> & stuff')\n")
	}
	sb.WriteString("```\n")
	sb.WriteString(strings.Repeat("word ", 3000))
	out := Chunk(sb.String(), Limit)
	if len(out) < 4 {
		t.Fatalf("expected several chunks, got %d", len(out))
	}
	for i, c := range out {
		if len(c) > Limit {
			t.Fatalf("chunk %d too long: %d", i, len(c))
		}
		if strings.Count(c, "<pre") != strings.Count(c, "</pre>") || strings.Count(c, "<code") != strings.Count(c, "</code>") {
			t.Fatalf("chunk %d has unbalanced pre: %q", i, c[:80])
		}
		if strings.Contains(c, "&amp") && strings.HasSuffix(c, "&am") {
			t.Fatal("entity split")
		}
	}
	if !strings.Contains(out[1], `class="language-python"`) {
		t.Fatal("code fence not reopened in a later chunk")
	}
}

func TestPlainAndTail(t *testing.T) {
	p := ChunkPlain(strings.Repeat("é", 9000), 4000)
	if len(p) != 3 || len([]rune(p[0])) != 4000 {
		t.Fatalf("plain chunks %d", len(p))
	}
	if StripHTML("<b>a &amp; b</b>") != "a & b" {
		t.Fatal("strip")
	}
	if !strings.HasPrefix(Tail(strings.Repeat("x", 100), 10), "…") {
		t.Fatal("tail")
	}
	if Chunk("   ", Limit) != nil {
		t.Fatal("empty input")
	}
}
