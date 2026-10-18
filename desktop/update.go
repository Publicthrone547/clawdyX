package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/platform"
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
	// The download for this OS and architecture (empty when the release has
	// none): its file name, URL, size and kind (dmg, setup, zip, deb, rpm,
	// appimage, tar).
	AssetName  string `json:"assetName"`
	AssetURL   string `json:"assetUrl"`
	AssetBytes int64  `json:"assetBytes"`
	Kind       string `json:"kind"`
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
		TagName string         `json:"tag_name"`
		HTMLURL string         `json:"html_url"`
		Assets  []releaseAsset `json:"assets"`
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
	info := UpdateInfo{Current: version, Latest: latest, Newer: newerVersion(latest, version), URL: u}
	if a, kind, ok := pickAsset(r.Assets, platform.Current()); ok {
		info.AssetName, info.AssetURL, info.AssetBytes, info.Kind = a.Name, a.URL, a.Size, kind
	}
	return info, nil
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

// releaseAsset is one file of a GitHub release.
type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// downloadPrefix is where every release file must live; anything else in
// an answer is ignored.
const downloadPrefix = releasesURL + "/download/"

var archNames = map[string][]string{
	"amd64": {"amd64", "x86_64"},
	"arm64": {"arm64", "aarch64"},
}

// assetKind classifies a release file name by its suffix.
func assetKind(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.HasSuffix(n, ".dmg"):
		return "dmg"
	case strings.HasSuffix(n, "-setup.exe"):
		return "setup"
	case strings.HasSuffix(n, ".zip"):
		return "zip"
	case strings.HasSuffix(n, ".deb"):
		return "deb"
	case strings.HasSuffix(n, ".rpm"):
		return "rpm"
	case strings.HasSuffix(n, ".appimage"):
		return "appimage"
	case strings.HasSuffix(n, ".tar.gz"):
		return "tar"
	}
	return ""
}

// assetOrder is the preferred kinds per OS; on Linux the package format
// follows the distribution (os-release ID and ID_LIKE).
func assetOrder(in platform.Info) []string {
	switch in.OS {
	case "darwin":
		return []string{"dmg"}
	case "windows":
		return []string{"setup", "zip"}
	}
	d := " " + strings.ToLower(in.DistroID) + " "
	switch {
	case strings.Contains(d, " debian ") || strings.Contains(d, " ubuntu "):
		return []string{"deb", "appimage", "tar"}
	case strings.Contains(d, " fedora ") || strings.Contains(d, " rhel ") || strings.Contains(d, " suse ") || strings.Contains(d, " centos "):
		return []string{"rpm", "appimage", "tar"}
	}
	return []string{"appimage", "tar"}
}

// forThisComputer: the file names this OS and this architecture. A .dmg
// is for Apple silicon (the only Mac build).
func forThisComputer(name string, in platform.Info) bool {
	n := strings.ToLower(name)
	if in.OS == "darwin" {
		return strings.HasSuffix(n, ".dmg") && in.Arch == "arm64"
	}
	switch k := assetKind(n); in.OS {
	case "windows":
		if !strings.Contains(n, "windows") {
			return false
		}
	case "linux":
		// Packages and AppImages are Linux by kind; a tar.gz must say so.
		if k == "tar" && !strings.Contains(n, "linux") || strings.Contains(n, "windows") || strings.Contains(n, "darwin") {
			return false
		}
	default:
		return false
	}
	for _, a := range archNames[in.Arch] {
		if strings.Contains(n, a) {
			return true
		}
	}
	return false
}

// pickAsset chooses the download for this computer.
func pickAsset(assets []releaseAsset, in platform.Info) (releaseAsset, string, bool) {
	for _, kind := range assetOrder(in) {
		for _, a := range assets {
			if path.Base(a.Name) != a.Name || !strings.HasPrefix(a.URL, downloadPrefix) {
				continue
			}
			if assetKind(a.Name) == kind && forThisComputer(a.Name, in) {
				return a, kind, true
			}
		}
	}
	return releaseAsset{}, "", false
}

// OpenDownload opens a release file from an update check in the browser
// (the browser downloads it; clawdyX downloads nothing itself).
func (s *SystemService) OpenDownload(u string) error {
	if !strings.HasPrefix(u, downloadPrefix) || strings.ContainsAny(u, " \"'<>\\") {
		return fmt.Errorf("not a clawdyX release file: %q", u)
	}
	return openURL(u)
}
