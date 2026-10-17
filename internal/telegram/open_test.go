package telegram

import "testing"

func TestNormalizeURL(t *testing.T) {
	ok := map[string]string{
		"youtube.com":                 "https://youtube.com",
		"www.youtube.com/watch?v=abc": "https://www.youtube.com/watch?v=abc",
		"https://www.youtube.com":     "https://www.youtube.com",
		"http://example.org/a#b":      "http://example.org/a#b",
		"localhost:8080/x":            "https://localhost:8080/x",
	}
	for in, want := range ok {
		got, err := NormalizeURL(in)
		if err != nil || got != want {
			t.Errorf("NormalizeURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "youtube", "javascript:alert(1)", "file:///etc/passwd", "ftp://x.com",
		"https://user:pw@x.com", "-a", "--args", "x.com\nfoo", "x .com", "mailto:a@b.com", "https://-evil.com"} {
		if got, err := NormalizeURL(bad); err == nil {
			t.Errorf("NormalizeURL(%q) accepted as %q", bad, got)
		}
	}
}
