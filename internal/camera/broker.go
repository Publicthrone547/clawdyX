package camera

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrOff means the screen and camera features are switched off.
var ErrOff = errors.New("screen and camera features are switched off")

// Media is one photo or clip held in memory. It never touches the disk
// after the helper's temporary file is read and removed.
type Media struct {
	ID       string
	Kind     string // photo | video
	Camera   Camera
	TakenAt  time.Time
	Duration time.Duration // videos
	Data     []byte        // JPEG or MP4
}

// AuditRecord describes a capture for the audit log (never pixels).
type AuditRecord struct {
	Kind        string // camera-photo | camera-video
	Camera      Camera
	Duration    time.Duration
	Bytes       int
	Destination string // telegram
	Requester   string
}

// Broker is the single gate for camera captures.
type Broker struct {
	P Provider
	// Allowed returns nil when captures may happen now; checked before each.
	Allowed func() error
	Audit   func(AuditRecord)
	// OnChange is called when a capture starts or ends (UI indicator).
	OnChange func(capturing bool, camera string)

	mu     sync.Mutex
	access string // granted | denied | "" (no capture yet)
	accAt  time.Time
	held   map[string]*Media
	ord    []string
	now    func() time.Time
	busy   sync.Mutex // one capture at a time
}

// NewBroker returns a broker.
func NewBroker(p Provider, allowed func() error, audit func(AuditRecord)) *Broker {
	return &Broker{P: p, Allowed: allowed, Audit: audit, held: map[string]*Media{}, now: time.Now}
}

// Cameras lists the cameras (no capture; allowed even when switched off).
func (b *Broker) Cameras(ctx context.Context) ([]Camera, error) {
	if b.P == nil {
		return nil, ErrUnsupported
	}
	cs, err := b.P.Cameras(ctx)
	if len(cs) > MaxCameras {
		cs = cs[:MaxCameras]
	}
	return cs, err
}

// CheckSeconds validates a clip length.
func CheckSeconds(secs int) error {
	if secs < 1 {
		return errors.New("give the clip length in seconds, for example /camera video 10")
	}
	if secs > MaxVideoSeconds {
		return fmt.Errorf("clips can be at most %d seconds (you asked for %d)", MaxVideoSeconds, secs)
	}
	return nil
}

// Photo takes one photo with the selected camera ("" = built-in, or a number).
func (b *Broker) Photo(ctx context.Context, sel, dest, requester string) (*Media, error) {
	return b.capture(ctx, sel, "photo", 0, dest, requester)
}

// Video records a clip of secs seconds (1..MaxVideoSeconds) without sound.
func (b *Broker) Video(ctx context.Context, sel string, secs int, dest, requester string) (*Media, error) {
	if err := CheckSeconds(secs); err != nil {
		return nil, err
	}
	return b.capture(ctx, sel, "video", time.Duration(secs)*time.Second, dest, requester)
}

func (b *Broker) capture(ctx context.Context, sel, kind string, d time.Duration, dest, requester string) (*Media, error) {
	if b.Allowed != nil {
		if err := b.Allowed(); err != nil {
			return nil, err
		}
	}
	cs, err := b.Cameras(ctx)
	if err != nil {
		return nil, err
	}
	c, err := Select(cs, sel)
	if err != nil {
		return nil, err
	}
	b.busy.Lock()
	defer b.busy.Unlock()
	// The switch may have flipped while waiting for another capture.
	if b.Allowed != nil {
		if err := b.Allowed(); err != nil {
			return nil, err
		}
	}
	if b.OnChange != nil {
		b.OnChange(true, c.Name)
	}
	var data []byte
	if kind == "video" {
		data, err = b.P.Video(ctx, c, d)
	} else {
		data, err = b.P.Photo(ctx, c)
	}
	if b.OnChange != nil {
		b.OnChange(false, c.Name)
	}
	if err != nil {
		if errors.Is(err, ErrNoCameraAccess) {
			b.setAccess("denied")
		}
		return nil, fmt.Errorf("camera %d (%s): %w", c.Index, c.Name, err)
	}
	b.setAccess("granted")
	tooLarge := kind == "video" && len(data) > MaxVideoBytes
	if b.Audit != nil {
		to := dest
		if tooLarge {
			to = "discarded"
		}
		b.Audit(AuditRecord{Kind: "camera-" + kind, Camera: c, Duration: d, Bytes: len(data), Destination: to, Requester: requester})
	}
	if tooLarge {
		return nil, fmt.Errorf("the clip is %d MB, more than Telegram accepts (50 MB); record a shorter one", len(data)>>20)
	}
	m := &Media{ID: newID(), Kind: kind, Camera: c, TakenAt: b.now(), Duration: d, Data: data}
	b.hold(m)
	return m, nil
}

func (b *Broker) hold(m *Media) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sweepLocked()
	b.held[m.ID] = m
	b.ord = append(b.ord, m.ID)
	for len(b.ord) > HoldMax {
		delete(b.held, b.ord[0])
		b.ord = b.ord[1:]
	}
}

func (b *Broker) sweepLocked() {
	keep := b.ord[:0]
	for _, id := range b.ord {
		m := b.held[id]
		if m == nil || b.now().Sub(m.TakenAt) > HoldTTL {
			delete(b.held, id)
			continue
		}
		keep = append(keep, id)
	}
	b.ord = keep
}

func (b *Broker) setAccess(v string) {
	b.mu.Lock()
	b.access, b.accAt = v, b.now()
	b.mu.Unlock()
}

// Check reports the Camera permission: the helper's answer when it can
// give one without capturing, else the outcome of the last capture.
// Values: granted, denied, not asked yet, unknown.
func (b *Broker) Check(ctx context.Context) (string, time.Time) {
	if p, ok := b.P.(Preflighter); ok {
		if st, err := p.Preflight(ctx); err == nil {
			switch st {
			case "authorized":
				return "granted", b.now()
			case "denied", "restricted":
				return "denied", b.now()
			case "notDetermined":
				return "not asked yet", time.Time{}
			}
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.access != "" {
		return b.access, b.accAt
	}
	return "unknown", time.Time{}
}

// Get returns held media.
func (b *Broker) Get(id string) (*Media, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sweepLocked()
	m, ok := b.held[id]
	return m, ok
}

// Held returns how many photos and clips are in memory.
func (b *Broker) Held() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sweepLocked()
	return len(b.ord)
}

// Discard drops one item.
func (b *Broker) Discard(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.held, id)
	keep := b.ord[:0]
	for _, x := range b.ord {
		if x != id {
			keep = append(keep, x)
		}
	}
	b.ord = keep
}

// Clear drops everything held (switch off, shutdown).
func (b *Broker) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.held = map[string]*Media{}
	b.ord = nil
}
