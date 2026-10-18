package camera

import "errors"

// Supported reports whether this OS has a camera backend in this build.
const Supported = false

// ErrUnsupported is returned where there is no camera backend.
var ErrUnsupported = errors.New("camera capture is not available on this computer yet")

// ErrNoCameraAccess means the camera device could not be opened.
var ErrNoCameraAccess = Denied("no access to the camera device: add your user to the video group, then log in again")

const timeoutHint = "(another program may be using the camera)"

// NewProvider returns this OS's camera provider.
func NewProvider(string, string) Provider { return unsupported{} }
