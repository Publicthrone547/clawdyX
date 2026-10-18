package camera

import (
	"errors"
	"os"
)

// Supported reports whether this OS has a camera backend in this build.
const Supported = true

// ErrUnsupported is returned where there is no camera backend.
var ErrUnsupported = errors.New("camera capture is not available on this PC")

// ErrNoCameraAccess means Windows refused the camera.
var ErrNoCameraAccess = Denied("camera access for desktop apps is off (Windows Settings → Privacy & security → Camera)")

const timeoutHint = "(another app may be using the camera)"

// NewProvider returns this OS's camera provider: the Media Foundation
// helper clawdyx-camera.exe next to exe (helpers\ in the app's folder).
// Captures land briefly in a private folder under tmpDir (the state
// folder, which inherits the profile's ACL), else under %TEMP%.
func NewProvider(exe, tmpDir string) Provider {
	if tmpDir == "" {
		tmpDir = os.TempDir()
	} else {
		_ = os.MkdirAll(tmpDir, 0o700)
	}
	return HelperCamera{TmpDir: tmpDir, Path: FindHelper(exe)}
}
