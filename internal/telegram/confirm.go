package telegram

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"strings"
	"sync"
	"time"
)

// ConfirmTTL is how long a confirmation stays valid.
const ConfirmTTL = 120 * time.Second

// Confirmation errors.
var (
	ErrConfirmUnknown  = errors.New("unknown or expired confirmation")
	ErrConfirmUsed     = errors.New("confirmation already used")
	ErrConfirmMismatch = errors.New("confirmation does not match the action")
)

type pendingConfirm struct {
	action  string
	args    string
	mac     []byte
	expires time.Time
	used    bool
}

// Confirms holds one-time confirmation tokens bound to an exact action and
// arguments. Tokens live in memory only: a restart invalidates them, which
// is the safe failure mode.
type Confirms struct {
	mu  sync.Mutex
	key []byte
	m   map[string]*pendingConfirm
	now func() time.Time
}

// NewConfirms creates a store with a random per-process HMAC key.
func NewConfirms() *Confirms {
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	return &Confirms{key: k, m: map[string]*pendingConfirm{}, now: time.Now}
}

func (c *Confirms) mac(token, action, args string) []byte {
	h := hmac.New(sha256.New, c.key)
	h.Write([]byte(token + "\x00" + action + "\x00" + args))
	return h.Sum(nil)
}

// New issues a token for action+args.
func (c *Confirms) New(action, args string) string {
	b := make([]byte, 5)
	_, _ = rand.Read(b)
	tok := strings.TrimRight(base32.StdEncoding.EncodeToString(b), "=")[:6]
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[tok] = &pendingConfirm{action: action, args: args, mac: c.mac(tok, action, args), expires: c.now().Add(ConfirmTTL)}
	return tok
}

// Take consumes a token. If action is non-empty it must match. It returns
// the bound action and arguments.
func (c *Confirms) Take(token, action string) (string, string, error) {
	token = strings.ToUpper(strings.TrimSpace(token))
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.m[token]
	if !ok || c.now().After(p.expires) {
		delete(c.m, token)
		return "", "", ErrConfirmUnknown
	}
	if p.used {
		return "", "", ErrConfirmUsed
	}
	if action != "" && action != p.action {
		return "", "", ErrConfirmMismatch
	}
	if !hmac.Equal(p.mac, c.mac(token, p.action, p.args)) {
		return "", "", ErrConfirmMismatch
	}
	p.used = true
	return p.action, p.args, nil
}

// Deny discards a token.
func (c *Confirms) Deny(token string) bool {
	token = strings.ToUpper(strings.TrimSpace(token))
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.m[token]
	if !ok || p.used {
		return false
	}
	p.used = true
	return true
}

// Sweep drops expired tokens.
func (c *Confirms) Sweep() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, p := range c.m {
		if c.now().After(p.expires) || p.used {
			delete(c.m, k)
		}
	}
}
