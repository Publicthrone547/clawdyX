package observability

import (
	"strings"
	"testing"
)

func TestRedactTokenInsideBotURL(t *testing.T) {
	tok := "123456789:" + strings.Repeat("Q", 35)
	in := `Post "https://api.telegram.org/bot` + tok + `/getUpdates": dial tcp: lookup api.telegram.org: no such host`
	if got := Redact(in); strings.Contains(got, strings.Repeat("Q", 35)) {
		t.Fatalf("token leaked: %s", got)
	}
}
