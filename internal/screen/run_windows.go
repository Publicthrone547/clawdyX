package screen

import (
	"context"
	"errors"
	"time"
)

// ErrUnsupported is returned by every capture and input action on Windows.
var ErrUnsupported = errors.New("screen capture and screen control are not supported on Windows yet (macOS only)")

// run backs every macOS tool the screen features use (osascript,
// screencapture, the ScreenCaptureKit helper); none exists on Windows.
func run(context.Context, time.Duration, string, ...string) (string, error) {
	return "", ErrUnsupported
}
