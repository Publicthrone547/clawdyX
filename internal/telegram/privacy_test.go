package telegram

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/config"
)

// Nothing is captured until the owner asks: starting the bot with screen
// and camera features on touches neither provider (on every OS, since the
// providers are fakes and only the bot's own logic runs).
func TestNoCaptureAtStartup(t *testing.T) {
	var shots atomic.Int64
	before := fakeCaptures.Load()
	h := startOpts(t, func(c *config.Config) { screenOn(c); cameraOn(c) }, Options{Desktop: fakeDesk{}, Cameras: fakeCamera{shots: &shots}})
	time.Sleep(2 * time.Second)
	if n := fakeCaptures.Load() - before; n != 0 {
		t.Fatalf("%d screen captures at startup", n)
	}
	if n := shots.Load(); n != 0 {
		t.Fatalf("%d camera captures at startup", n)
	}
	if h.app.broker.Held() != 0 || h.app.cams.Held() != 0 {
		t.Fatal("media held at startup")
	}
}
