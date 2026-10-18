package telegram

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/go-telegram/bot/models"
)

var (
	schemeRe = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9+.-]*):(.*)$`)
	portRe   = regexp.MustCompile(`^[0-9]{1,5}(/.*)?$`)
)

// NormalizeURL turns owner input such as "youtube.com" into an absolute
// http(s) URL and rejects anything else (other schemes, credentials, hosts
// without a dot, whitespace, leading dashes that `open` could read as flags).
func NormalizeURL(in string) (string, error) {
	s := strings.TrimSpace(in)
	if s == "" || len(s) > 2048 {
		return "", errors.New("give an address like youtube.com")
	}
	for _, r := range s {
		if r <= ' ' || r == 0x7f {
			return "", errors.New("the address must not contain spaces or control characters")
		}
	}
	if !strings.Contains(s, "://") {
		// "javascript:…" or "mailto:…" look like a scheme; "localhost:8080" is host:port.
		if m := schemeRe.FindStringSubmatch(s); m != nil && !portRe.MatchString(m[2]) {
			return "", errors.New("only http and https addresses are allowed")
		}
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", errors.New("that is not a valid address")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", errors.New("only http and https addresses are allowed")
	}
	if u.User != nil {
		return "", errors.New("addresses with a username or password are not allowed")
	}
	host := u.Hostname()
	if host == "" || strings.HasPrefix(host, "-") || (!strings.Contains(host, ".") && host != "localhost") {
		return "", errors.New("give a full address like youtube.com")
	}
	return u.String(), nil
}

func (a *App) cmdOpen(m *models.Message, args []string) {
	if !a.cfg.Desktop.OpenEnabled() {
		a.replyTo(m, "/open is disabled in the config (desktop.open_urls).")
		return
	}
	if len(args) != 1 {
		a.replyTo(m, "Usage: /open ADDRESS — for example /open youtube.com")
		return
	}
	u, err := NormalizeURL(args[0])
	if err != nil {
		a.replyTo(m, "Not opened: "+Esc(err.Error())+".")
		return
	}
	app := a.cfg.Desktop.BrowserApp
	parsed, _ := url.Parse(u)
	if err := a.opener(a.ctx, app, u); err != nil {
		a.st.Audit("desktop", "open", parsed.Hostname(), "failed", 0)
		a.replyTo(m, fmt.Sprintf("❌ Could not open it in %s: %s", Esc(browserName(app)), Esc(err.Error())))
		return
	}
	a.st.Audit("desktop", "open", parsed.Hostname(), "ok", 0)
	a.replyTo(m, fmt.Sprintf("🌐 Opened %s in %s on %s.", Esc(u), Esc(browserName(app)), a.words.TheComputer))
}
