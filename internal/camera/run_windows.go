package camera

import (
	"context"
	"errors"
	"time"
)

// ErrUnsupported is returned by every camera action on Windows.
var ErrUnsupported = errors.New("camera capture is not supported on Windows (macOS only)")

// Supported reports whether this platform has a camera backend (macOS).
const Supported = false

func run(context.Context, time.Duration, string, ...string) (string, error) {
	return "", ErrUnsupported
}
