//go:build !windows

package camera

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeHelper imitates clawdyx-camera's command line and JSON output.
const fakeHelper = `#!/bin/sh
case "$1" in
list) echo '{"ok":true,"cameras":[{"id":"ext","name":"USB Cam","builtin":false},{"id":"fhd","name":"FaceTime HD Camera","builtin":true}]}' ;;
preflight) echo '{"ok":true,"status":"notDetermined"}' ;;
photo) [ "$2" = fhd ] || { echo '{"ok":false,"error":"camera permission missing (denied)"}'; exit 3; }
       printf 'JPEG' > "$3"; echo "$3" > "$(dirname "$0")/last"; echo '{"ok":true,"bytes":4}' ;;
video) printf "MP4-$3" > "$4"; echo '{"ok":true}' ;;
esac
`

func TestMacHelperProtocol(t *testing.T) {
	dir := t.TempDir()
	h := filepath.Join(dir, HelperName)
	if err := os.WriteFile(h, []byte(fakeHelper), 0o755); err != nil {
		t.Fatal(err)
	}
	if FindHelper(filepath.Join(dir, "clawdyx")) != h {
		t.Fatal("helper next to the worker not found")
	}
	m := Mac{TmpDir: t.TempDir(), Helper: h}
	ctx := context.Background()
	cs, err := m.Cameras(ctx)
	if err != nil || len(cs) != 2 || cs[0].ID != "fhd" || cs[0].Index != 1 {
		t.Fatalf("cameras %+v %v", cs, err)
	}
	if st, err := m.Preflight(ctx); err != nil || st != "notDetermined" {
		t.Fatalf("preflight %q %v", st, err)
	}
	b, err := m.Photo(ctx, cs[0])
	if err != nil || string(b) != "JPEG" {
		t.Fatalf("photo %q %v", b, err)
	}
	last, _ := os.ReadFile(filepath.Join(dir, "last"))
	if _, err := os.Stat(filepath.Dir(string(last[:len(last)-1]))); !os.IsNotExist(err) {
		t.Fatal("the temporary capture folder was not removed")
	}
	if _, err := m.Photo(ctx, cs[1]); !errors.Is(err, ErrNoCameraAccess) {
		t.Fatalf("denied photo: %v", err)
	}
	v, err := m.Video(ctx, cs[0], 7*time.Second)
	if err != nil || string(v) != "MP4-7" {
		t.Fatalf("video %q %v", v, err)
	}
}

func TestMacWithoutHelperExplainsWhy(t *testing.T) {
	_, err := Mac{}.Cameras(context.Background())
	if !errors.Is(err, ErrNoHelper) {
		t.Fatalf("no helper: %v", err)
	}
}
