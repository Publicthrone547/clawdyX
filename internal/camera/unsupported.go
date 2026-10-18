package camera

import (
	"context"
	"time"
)

// unsupported is the Provider of an OS without a camera backend.
type unsupported struct{}

func (unsupported) Cameras(context.Context) ([]Camera, error)     { return nil, ErrUnsupported }
func (unsupported) Photo(context.Context, Camera) ([]byte, error) { return nil, ErrUnsupported }
func (unsupported) Video(context.Context, Camera, time.Duration) ([]byte, error) {
	return nil, ErrUnsupported
}
