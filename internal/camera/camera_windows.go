package camera

import "errors"

// Supported reports whether this OS has a camera backend in this build.
const Supported = false

// ErrUnsupported is returned where there is no camera backend.
var ErrUnsupported = errors.New("camera capture is not available on this PC yet")

// ErrNoCameraAccess means Windows refused the camera.
var ErrNoCameraAccess = Denied("camera access for desktop apps is off (Windows Settings → Privacy & security → Camera)")

const timeoutHint = "(another app may be using the camera)"

// NewProvider returns this OS's camera provider.
func NewProvider(string) Provider { return unsupported{} }
