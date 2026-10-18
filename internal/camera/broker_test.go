package camera

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fakeCam struct {
	cs     []Camera
	shots  atomic.Int64
	denied bool
	size   int
	last   time.Duration
}

func (f *fakeCam) Cameras(context.Context) ([]Camera, error) { return f.cs, nil }

func (f *fakeCam) Photo(context.Context, Camera) ([]byte, error) {
	f.shots.Add(1)
	if f.denied {
		return nil, ErrNoCameraAccess
	}
	return []byte{0xff, 0xd8, 0xff, 0xd9}, nil
}

func (f *fakeCam) Video(_ context.Context, _ Camera, d time.Duration) ([]byte, error) {
	f.shots.Add(1)
	f.last = d
	n := f.size
	if n == 0 {
		n = 16
	}
	return make([]byte, n), nil
}

func twoCams() *fakeCam {
	return &fakeCam{cs: Order([]Camera{{ID: "ext", Name: "USB Cam"}, {ID: "fhd", Name: "FaceTime HD Camera", Builtin: true}})}
}

func TestOrderAndSelect(t *testing.T) {
	cs := twoCams().cs
	if cs[0].ID != "fhd" || cs[0].Index != 1 || cs[1].Index != 2 {
		t.Fatalf("built-in camera must be number 1: %+v", cs)
	}
	if c, err := Select(cs, ""); err != nil || c.ID != "fhd" {
		t.Fatalf("default camera %+v %v", c, err)
	}
	if c, err := Select(cs, "2"); err != nil || c.ID != "ext" {
		t.Fatalf("camera 2 %+v %v", c, err)
	}
	if _, err := Select(cs, "3"); !errors.Is(err, ErrNoSuchCamera) {
		t.Fatalf("camera 3: %v", err)
	}
	if _, err := Select(nil, ""); err == nil {
		t.Fatal("no camera must be an error")
	}
}

func TestBrokerSwitchLimitsAuditAndTTL(t *testing.T) {
	p := twoCams()
	on := true
	var audit []AuditRecord
	b := NewBroker(p, func() error {
		if !on {
			return ErrOff
		}
		return nil
	}, func(r AuditRecord) { audit = append(audit, r) })
	ctx := context.Background()

	for _, secs := range []int{0, -1, 61, 600} {
		if _, err := b.Video(ctx, "", secs, "telegram", "t"); err == nil {
			t.Fatalf("%d s accepted", secs)
		}
	}
	if _, err := b.Video(ctx, "", 61, "telegram", "t"); err == nil || !strings.Contains(err.Error(), "at most 60 seconds") {
		t.Fatalf("limit message: %v", err)
	}
	if p.shots.Load() != 0 || len(audit) != 0 {
		t.Fatal("rejected lengths must not touch the camera")
	}

	m, err := b.Photo(ctx, "2", "telegram", "telegram-owner")
	if err != nil || m.Kind != "photo" || m.Camera.ID != "ext" {
		t.Fatalf("photo %+v %v", m, err)
	}
	v, err := b.Video(ctx, "", 60, "telegram", "telegram-owner")
	if err != nil || v.Duration != 60*time.Second || p.last != 60*time.Second {
		t.Fatalf("video %+v %v", v, err)
	}
	if len(audit) != 2 || audit[0].Kind != "camera-photo" || audit[0].Camera.ID != "ext" || audit[0].Bytes != 4 ||
		audit[1].Kind != "camera-video" || audit[1].Duration != time.Minute || audit[1].Destination != "telegram" || audit[1].Requester != "telegram-owner" {
		t.Fatalf("audit %+v", audit)
	}
	if b.Held() != 2 {
		t.Fatalf("held %d", b.Held())
	}

	// Switched off: nothing is captured, not even a photo.
	on = false
	if _, err := b.Photo(ctx, "", "telegram", "t"); !errors.Is(err, ErrOff) {
		t.Fatalf("off: %v", err)
	}
	if p.shots.Load() != 2 {
		t.Fatal("captured while switched off")
	}

	// Held media expires after five minutes.
	t0 := time.Now()
	b.now = func() time.Time { return t0.Add(HoldTTL + time.Second) }
	if b.Held() != 0 {
		t.Fatal("media kept longer than HoldTTL")
	}
	if _, ok := b.Get(m.ID); ok {
		t.Fatal("expired photo still readable")
	}
}

func TestBrokerOversizeClipIsAuditedNotHeld(t *testing.T) {
	p := twoCams()
	p.size = MaxVideoBytes + 1
	var audit []AuditRecord
	b := NewBroker(p, nil, func(r AuditRecord) { audit = append(audit, r) })
	if _, err := b.Video(context.Background(), "", 30, "telegram", "t"); err == nil || !strings.Contains(err.Error(), "50 MB") {
		t.Fatalf("oversize clip: %v", err)
	}
	if b.Held() != 0 || len(audit) != 1 || audit[0].Destination != "discarded" {
		t.Fatalf("held %d audit %+v", b.Held(), audit)
	}
}

func TestBrokerAccessFromLastCapture(t *testing.T) {
	p := twoCams()
	b := NewBroker(p, nil, nil)
	if acc, _ := b.Check(context.Background()); acc != "unknown" {
		t.Fatalf("before any capture: %s", acc)
	}
	p.denied = true
	_, err := b.Photo(context.Background(), "", "telegram", "t")
	if !errors.Is(err, ErrNoCameraAccess) || !strings.Contains(err.Error(), "Privacy & Security → Camera") {
		t.Fatalf("denied: %v", err)
	}
	if acc, _ := b.Check(context.Background()); acc != "denied" {
		t.Fatalf("after a denied capture: %s", acc)
	}
}

func TestMacWithoutHelperExplainsWhy(t *testing.T) {
	_, err := Mac{}.Cameras(context.Background())
	if !errors.Is(err, ErrNoHelper) {
		t.Fatalf("no helper: %v", err)
	}
}
