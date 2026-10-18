package camera

import (
	"errors"
	"os"
)

// Supported reports whether this OS has a camera backend in this build.
const Supported = true

// ErrUnsupported is returned where there is no camera backend.
var ErrUnsupported = errors.New("camera capture is not available on this Mac")

// ErrNoCameraAccess means macOS refused the camera.
var ErrNoCameraAccess = Denied("macOS Camera permission is missing for clawdyX (System Settings → Privacy & Security → Camera)")

const timeoutHint = "(if macOS is asking about camera access, answer it on the Mac)"

// NewProvider returns this OS's camera provider: the AVFoundation helper
// next to exe (inside clawdyX.app).
func NewProvider(exe, _ string) Provider {
	return HelperCamera{TmpDir: os.TempDir(), Path: FindHelper(exe)}
}
