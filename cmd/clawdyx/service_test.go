package main

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestPlistIsValidXMLAndHasNoSecrets(t *testing.T) {
	p := Plist("/Users/me/go/bin/clawdyx", "/Users/me/.config/clawdyx/config.json", "/Users/me/Library/Logs/clawdyx", "/usr/bin:/bin")
	d := xml.NewDecoder(strings.NewReader(p))
	d.Strict = false
	for {
		if _, err := d.Token(); err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("invalid xml: %v", err)
		}
	}
	for _, want := range []string{"local.clawdyx", "<string>run</string>", "LimitLoadToSessionType", "Aqua", "SuccessfulExit"} {
		if !strings.Contains(p, want) {
			t.Errorf("plist missing %s", want)
		}
	}
	if strings.Contains(p, "TELEGRAM_BOT_TOKEN") {
		t.Fatal("token must not be in the plist")
	}
}
