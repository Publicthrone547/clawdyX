package telegram

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/render"
)

// Esc escapes text for HTML parse mode.
func Esc(s string) string { return render.Esc(s) }

func dur(d time.Duration) string {
	switch {
	case d < 0:
		return "0s"
	case d < 10*time.Second:
		return fmt.Sprintf("%.1fs", d.Seconds())
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd%02dh", int(d.Hours())/24, int(d.Hours())%24)
	}
}

func ago(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return dur(time.Since(t)) + " ago"
}

func tokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 10_000:
		return fmt.Sprintf("%.0fk", float64(n)/1e3)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return fmt.Sprint(n)
}

func mb(kb int64) string { return fmt.Sprintf("%d MB", kb/1024) }

func gib(b uint64) string { return fmt.Sprintf("%.1f GiB", float64(b)/(1<<30)) }

func shortID(u string) string {
	if len(u) >= 8 {
		return u[:8]
	}
	return u
}

// homeRel shortens a path under $HOME to ~/..., truncated to n runes.
func homeRel(p string, n int) string {
	if home, err := os.UserHomeDir(); err == nil {
		if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
			p = "~/" + rel
		}
	}
	if r := []rune(p); len(r) > n {
		p = "…" + string(r[len(r)-n+1:])
	}
	return p
}

func modelEffort(model, effort string) string {
	if effort == "" {
		effort = "auto"
	}
	return model + "/" + effort
}

var pathRe = regexp.MustCompile(`/(?:Users|private|var)/[^\s"'<>()\[\]{}` + "`" + `]+`)
