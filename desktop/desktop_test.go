package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenOnlyKnownLinks(t *testing.T) {
	s := &SystemService{}
	if err := s.Open("file:///etc/passwd"); err == nil {
		t.Fatal("arbitrary URL accepted")
	}
	if err := s.Open("../../x"); err == nil {
		t.Fatal("arbitrary key accepted")
	}
	for k, u := range links {
		if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "x-apple.systempreferences:") {
			t.Errorf("%s: unexpected scheme %s", k, u)
		}
	}
}

func TestSanitizeRedactsTokensAndHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	tok := "123456789:" + strings.Repeat("A", 35)
	out := sanitize("token "+tok+" path "+home+"/Library/x", false, &WorkerService{})
	if strings.Contains(out, tok) || strings.Contains(out, home) || !strings.Contains(out, "~/Library/x") {
		t.Fatalf("not sanitized: %q", out)
	}
}

func TestWorkerCallsFailCleanlyWhenDown(t *testing.T) {
	w := &WorkerService{}
	if _, err := w.Status(); err != ErrWorkerDown {
		t.Fatalf("want ErrWorkerDown, got %v", err)
	}
}

func TestNewerVersion(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.3.2", "0.3.1", true}, {"0.10.0", "0.9.9", true}, {"1.0.0", "0.99.99", true},
		{"0.3.1", "0.3.1", false}, {"0.3.0", "0.3.1", false},
		{"0.3", "0.3.1", false}, {"0.3.1-rc1", "0.3.0", false}, {"01.0.0", "0.9.0", false}, {"", "0.3.1", false},
	} {
		if got := newerVersion(c.a, c.b); got != c.want {
			t.Errorf("newerVersion(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func TestCheckForUpdate(t *testing.T) {
	answer := `{"tag_name":"v99.0.0","html_url":"https://github.com/Publicthrone547/clawdyX/releases/tag/v99.0.0"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("update check must not send credentials")
		}
		fmt.Fprint(w, answer)
	}))
	defer srv.Close()
	old := latestReleaseAPI
	latestReleaseAPI = srv.URL
	defer func() { latestReleaseAPI = old }()
	s := &SystemService{}

	u, err := s.CheckForUpdate()
	if err != nil || !u.Newer || u.Latest != "99.0.0" || u.Current != version || !strings.HasSuffix(u.URL, "/tag/v99.0.0") {
		t.Fatalf("newer release: %+v %v", u, err)
	}
	answer = `{"tag_name":"v` + version + `","html_url":"https://evil.example/x"}`
	if u, err = s.CheckForUpdate(); err != nil || u.Newer || u.URL != releasesURL+"/tag/v"+version {
		t.Fatalf("same release / foreign URL: %+v %v", u, err)
	}
	answer = `{"tag_name":"nightly"}`
	if _, err = s.CheckForUpdate(); err == nil {
		t.Fatal("non-version tag accepted")
	}
	if err := s.OpenRelease("1.0.0; rm -rf /"); err == nil {
		t.Fatal("invalid version accepted")
	}
}
