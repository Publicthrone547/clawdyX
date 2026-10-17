package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const releasesURL = "https://github.com/Publicthrone547/clawdyX/releases"

// latestReleaseAPI is a variable so tests can point it at a local server.
var latestReleaseAPI = "https://api.github.com/repos/Publicthrone547/clawdyX/releases/latest"

// UpdateInfo is the result of a manual update check.
type UpdateInfo struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
	Newer   bool   `json:"newer"` // Latest is newer than Current
	URL     string `json:"url"`   // release page of Latest
}

// CheckForUpdate asks GitHub for the latest published release. It runs only
// when the user clicks the button; nothing is sent besides the request itself.
func (s *SystemService) CheckForUpdate() (UpdateInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseAPI, nil)
	if err != nil {
		return UpdateInfo{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "clawdyX/"+version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return UpdateInfo{}, fmt.Errorf("could not reach GitHub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return UpdateInfo{}, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var r struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
		return UpdateInfo{}, fmt.Errorf("unreadable answer from GitHub: %w", err)
	}
	latest := strings.TrimPrefix(r.TagName, "v")
	if _, ok := parseVersion(latest); !ok {
		return UpdateInfo{}, fmt.Errorf("unexpected release tag %q", r.TagName)
	}
	u := releasesURL + "/tag/v" + latest
	if strings.HasPrefix(r.HTMLURL, releasesURL+"/") {
		u = r.HTMLURL
	}
	return UpdateInfo{Current: version, Latest: latest, Newer: newerVersion(latest, version), URL: u}, nil
}

// OpenRelease opens the release page of this version, or of the given
// version from a previous update check.
func (s *SystemService) OpenRelease(v string) error {
	if v == "" {
		v = version
	}
	if _, ok := parseVersion(v); !ok {
		return fmt.Errorf("invalid version %q", v)
	}
	return openURL(releasesURL + "/tag/v" + v)
}

// parseVersion reads "MAJOR.MINOR.PATCH".
func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || p != strconv.Itoa(n) {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// newerVersion reports whether a is a newer version than b.
func newerVersion(a, b string) bool {
	x, ok1 := parseVersion(a)
	y, ok2 := parseVersion(b)
	if !ok1 || !ok2 {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return x[i] > y[i]
		}
	}
	return false
}
