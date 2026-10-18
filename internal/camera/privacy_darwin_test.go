package camera

import (
	"os"
	"testing"
)

// On macOS captures land in the per-user $TMPDIR (mode 0700), not /tmp.
func TestCaptureFolderIsThePerUserTemp(t *testing.T) {
	p, ok := NewProvider("/nowhere/clawdyx", "").(HelperCamera)
	if !ok || p.TmpDir != os.TempDir() {
		t.Fatalf("provider %#v", p)
	}
}
