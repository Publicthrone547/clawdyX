package main

import (
	"strings"
	"testing"

	"github.com/Publicthrone547/clawdyX/internal/platform"
)

func TestPickAsset(t *testing.T) {
	names := []string{
		"clawdyX-1.0.1.dmg", "clawdyx-1.0.1-darwin-arm64.tar.gz",
		"clawdyX-1.0.1-windows-amd64-setup.exe", "clawdyX-1.0.1-windows-amd64.zip", "clawdyX-1.0.1-windows-arm64.zip",
		"clawdyx_1.0.1_amd64.deb", "clawdyx_1.0.1_arm64.deb", "clawdyx-1.0.1-1.x86_64.rpm", "clawdyx-1.0.1-1.aarch64.rpm",
		"clawdyX-1.0.1-x86_64.AppImage", "clawdyX-1.0.1-linux-amd64.tar.gz", "clawdyX-1.0.1-linux-arm64.tar.gz", "SHA256SUMS",
	}
	var assets []releaseAsset
	for _, n := range names {
		assets = append(assets, releaseAsset{Name: n, URL: downloadPrefix + "v1.0.1/" + n, Size: 1})
	}
	cases := []struct {
		in         platform.Info
		want, kind string
	}{
		{platform.Info{OS: "darwin", Arch: "arm64"}, "clawdyX-1.0.1.dmg", "dmg"},
		{platform.Info{OS: "darwin", Arch: "amd64"}, "", ""},
		{platform.Info{OS: "windows", Arch: "amd64"}, "clawdyX-1.0.1-windows-amd64-setup.exe", "setup"},
		{platform.Info{OS: "windows", Arch: "arm64"}, "clawdyX-1.0.1-windows-arm64.zip", "zip"},
		{platform.Info{OS: "linux", Arch: "amd64", DistroID: "ubuntu debian"}, "clawdyx_1.0.1_amd64.deb", "deb"},
		{platform.Info{OS: "linux", Arch: "arm64", DistroID: "debian"}, "clawdyx_1.0.1_arm64.deb", "deb"},
		{platform.Info{OS: "linux", Arch: "amd64", DistroID: "fedora"}, "clawdyx-1.0.1-1.x86_64.rpm", "rpm"},
		{platform.Info{OS: "linux", Arch: "amd64", DistroID: "opensuse-tumbleweed suse opensuse"}, "clawdyx-1.0.1-1.x86_64.rpm", "rpm"},
		{platform.Info{OS: "linux", Arch: "amd64", DistroID: "arch"}, "clawdyX-1.0.1-x86_64.AppImage", "appimage"},
		{platform.Info{OS: "linux", Arch: "arm64", DistroID: "arch"}, "clawdyX-1.0.1-linux-arm64.tar.gz", "tar"},
	}
	for _, c := range cases {
		a, kind, ok := pickAsset(assets, c.in)
		if a.Name != c.want || kind != c.kind || ok != (c.want != "") {
			t.Errorf("%+v: got %q %q %v, want %q %q", c.in, a.Name, kind, ok, c.want, c.kind)
		}
	}
	// Files outside the project's release downloads are ignored.
	bad := []releaseAsset{{Name: "clawdyX-1.0.1.dmg", URL: "https://evil.example/clawdyX-1.0.1.dmg"}, {Name: "../x.dmg", URL: downloadPrefix + "x.dmg"}}
	if a, _, ok := pickAsset(bad, platform.Info{OS: "darwin", Arch: "arm64"}); ok {
		t.Fatalf("picked %+v", a)
	}
	s := &SystemService{}
	for _, u := range []string{"https://evil.example/x.exe", releasesURL + "/tag/v1", downloadPrefix + "v1/a b.exe"} {
		if err := s.OpenDownload(u); err == nil {
			t.Errorf("OpenDownload(%q) accepted", u)
		}
	}
}

func TestSystemLine(t *testing.T) {
	cases := map[string]platform.Info{
		"Windows 11 23H2 · amd64":                    {OS: "windows", OSName: "Windows", Pretty: "Windows 11 23H2", Arch: "amd64", Session: platform.SessionWindows},
		"macOS 14.6 · arm64":                         {OS: "darwin", OSName: "macOS", Pretty: "macOS 14.6", Arch: "arm64", Session: platform.SessionAqua},
		"Ubuntu 24.04 LTS · amd64 · Wayland (GNOME)": {OS: "linux", OSName: "Linux", Pretty: "Ubuntu 24.04 LTS", Arch: "amd64", Session: platform.SessionWayland, Desktop: "GNOME"},
		"Linux · arm64 · no graphical session":       {OS: "linux", OSName: "Linux", Arch: "arm64", Session: platform.SessionHeadless},
	}
	for want, in := range cases {
		if got := systemLine(in); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

func TestCopyDiagnosticsLeavesOutTelegramIDs(t *testing.T) {
	s := &SystemService{w: &WorkerService{}}
	out := s.CopyDiagnostics()
	if !strings.Contains(out, "## System\n\n"+systemLine(platform.Current())) || strings.Contains(out, "Allowed Telegram users") {
		t.Fatalf("report: %q", out)
	}
	if err := s.RevealFolder("../../etc"); err == nil {
		t.Fatal("an unknown folder was accepted")
	}
}
