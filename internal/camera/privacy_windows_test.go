package camera

import (
	"os"
	"path/filepath"
	"testing"
)

// On Windows captures land under the state folder (which inherits the
// profile's ACL), not the shared %TEMP%.
func TestCaptureFolderIsUnderTheStateFolder(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "state", "tmp")
	p, ok := NewProvider(`C:\nowhere\clawdyx.exe`, tmp).(HelperCamera)
	if !ok || p.TmpDir != tmp {
		t.Fatalf("provider %#v", p)
	}
	if st, err := os.Stat(tmp); err != nil || !st.IsDir() {
		t.Fatalf("the capture folder was not created: %v", err)
	}
}
