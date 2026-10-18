package screen

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Provider enumerates and captures displays.
type Provider interface {
	Displays(ctx context.Context) ([]Display, error)
	CapturePNG(ctx context.Context, d Display) ([]byte, error)
}

// Capture limits.
const (
	HoldMax         = 6               // captures kept in memory at once
	HoldTTL         = 5 * time.Minute // a held capture is dropped after this
	ClaudeMaxEdge   = 1568            // Claude's standard image tier
	TelegramMaxEdge = 2560
	PreviewMaxEdge  = 1568
	MaxDisplays     = 8
)

// ErrScreenOff means the screen features are switched off.
var ErrScreenOff = errors.New("screen features are switched off")

// Capture is one display image held in memory. It never touches the disk.
type Capture struct {
	ID      string    `json:"id"`
	Display Display   `json:"display"`
	TakenAt time.Time `json:"takenAt"`
	PNG     []byte    `json:"-"`
	Preview Encoded   `json:"-"` // JPEG ≤ PreviewMaxEdge, also used for Claude
}

// AuditRecord describes a capture for the audit log (never pixels).
type AuditRecord struct {
	Display     Display
	Width       int
	Height      int
	Bytes       int
	Destination string // preview | telegram | claude | mcp
	Requester   string
}

// Broker is the single gate for screen captures from Telegram, the desktop
// app and Claude's screenshot tool.
type Broker struct {
	P Provider
	// Allowed returns nil when captures may happen now (configured and not
	// switched off). It is checked before every capture.
	Allowed func() error
	// Audit records each capture.
	Audit func(AuditRecord)
	// OnChange is called when a capture starts or ends (UI indicator).
	OnChange func(capturing bool, display string)

	mu     sync.Mutex
	access string // granted | denied | "" (no capture yet)
	accAt  time.Time
	held   map[string]*Capture
	ord    []string
	now    func() time.Time
	busy   sync.Mutex // one capture at a time
}

// NewBroker returns a broker.
func NewBroker(p Provider, allowed func() error, audit func(AuditRecord)) *Broker {
	return &Broker{P: p, Allowed: allowed, Audit: audit, held: map[string]*Capture{}, now: time.Now}
}

// Displays lists the connected displays (no capture, allowed even when off).
func (b *Broker) Displays(ctx context.Context) ([]Display, error) {
	if b.P == nil {
		return nil, errors.New("screen capture is not available on this platform")
	}
	ds, err := b.P.Displays(ctx)
	if len(ds) > MaxDisplays {
		ds = ds[:MaxDisplays]
	}
	return ds, err
}

// Capture captures the selected display(s): "" or "1" (main), an index, a
// key prefix, or "all". Each display becomes its own capture, never
// stitched. dest and requester go to the audit log.
func (b *Broker) Capture(ctx context.Context, sel, dest, requester string) ([]*Capture, error) {
	if b.Allowed != nil {
		if err := b.Allowed(); err != nil {
			return nil, err
		}
	}
	ds, err := b.Displays(ctx)
	if err != nil {
		return nil, err
	}
	if len(ds) == 0 {
		return nil, errors.New("no display is connected")
	}
	var targets []Display
	if strings.EqualFold(strings.TrimSpace(sel), "all") {
		targets = ds
	} else {
		d, err := Select(ds, sel)
		if err != nil {
			return nil, err
		}
		targets = []Display{d}
	}
	b.busy.Lock()
	defer b.busy.Unlock()
	var out []*Capture
	for _, d := range targets {
		if b.OnChange != nil {
			b.OnChange(true, d.Name)
		}
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		raw, err := b.P.CapturePNG(cctx, d)
		cancel()
		if b.OnChange != nil {
			b.OnChange(false, d.Name)
		}
		if err != nil {
			if errors.Is(err, ErrCaptureDenied) {
				b.setAccess("denied")
			}
			return out, fmt.Errorf("display %d (%s): %w", d.Index, d.Name, err)
		}
		b.setAccess("granted")
		pv, err := EncodeJPEG(raw, PreviewMaxEdge, 75)
		if err != nil {
			return out, err
		}
		c := &Capture{ID: newID(), Display: d, TakenAt: b.now(), PNG: raw, Preview: pv}
		if c.Display.PixelW == 0 {
			c.Display.PixelW, c.Display.PixelH = pv.SrcW, pv.SrcH
		}
		b.hold(c)
		if b.Audit != nil {
			b.Audit(AuditRecord{Display: d, Width: pv.SrcW, Height: pv.SrcH, Bytes: len(raw), Destination: dest, Requester: requester})
		}
		out = append(out, c)
	}
	return out, nil
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (b *Broker) hold(c *Capture) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sweepLocked()
	b.held[c.ID] = c
	b.ord = append(b.ord, c.ID)
	for len(b.ord) > HoldMax {
		delete(b.held, b.ord[0])
		b.ord = b.ord[1:]
	}
}

func (b *Broker) sweepLocked() {
	keep := b.ord[:0]
	for _, id := range b.ord {
		c := b.held[id]
		if c == nil || b.now().Sub(c.TakenAt) > HoldTTL {
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

// Access reports the Screen Recording permission as seen by the last real
// capture: "granted", "denied", or "unknown" before the first capture.
// (Inferring it without capturing proved unreliable on macOS 27.)
func (b *Broker) Access() (string, time.Time) {
	b.mu.Lock()
	acc, at := b.access, b.accAt
	b.mu.Unlock()
	if acc != "" {
		return acc, at
	}
	return "unknown", time.Time{}
}

// Preflighter can check the permission without capturing (bundled helper).
type Preflighter interface {
	Preflight(ctx context.Context) (granted, known bool)
}

// Check refreshes the permission state without capturing when the provider
// supports it, and returns Access().
func (b *Broker) Check(ctx context.Context) (string, time.Time) {
	if p, ok := b.P.(Preflighter); ok {
		if g, known := p.Preflight(ctx); known {
			if g {
				b.setAccess("granted")
			} else {
				b.setAccess("denied")
			}
		}
	}
	return b.Access()
}

// Get returns a held capture.
func (b *Broker) Get(id string) (*Capture, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sweepLocked()
	c, ok := b.held[id]
	return c, ok
}

// Held returns how many captures are in memory.
func (b *Broker) Held() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sweepLocked()
	return len(b.ord)
}

// Discard drops one capture.
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

// Clear drops every held capture (switch off, shutdown).
func (b *Broker) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.held = map[string]*Capture{}
	b.ord = nil
}
