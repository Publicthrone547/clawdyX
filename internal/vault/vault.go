// Package vault stores the bot token outside ordinary files. On macOS it
// uses the login Keychain through Apple's /usr/bin/security tool (the secret
// is passed on stdin, never in argv). Items created that way belong to the
// security tool, so the worker and the desktop app can both read them and
// rebuilding an ad-hoc-signed clawdyx binary does not trigger Keychain
// prompts. The trade-off, documented for users: any process running as the
// same macOS user can read the item with the same tool.
package vault

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"sync"

	"github.com/zalando/go-keyring"
)

// Service is the Keychain service name of every clawdyX item.
const Service = "clawdyx"

// ErrNotFound means no item is stored.
var ErrNotFound = errors.New("vault: not found")

// Store is a small secret store.
type Store interface {
	Get(account string) (string, error)
	Set(account, value string) error
	Delete(account string) error
	Kind() string // "keychain" | "memory"
}

// TokenAccount names the bot-token item for one config file, so separate
// configurations (tests, a second bot) never share a token.
func TokenAccount(configPath string) string {
	abs, err := filepath.Abs(configPath)
	if err != nil {
		abs = configPath
	}
	h := sha256.Sum256([]byte(abs))
	return "telegram-bot-token@" + hex.EncodeToString(h[:6])
}

// Keychain is the macOS login Keychain (via /usr/bin/security).
type Keychain struct{}

// Get reads an item.
func (Keychain) Get(account string) (string, error) {
	v, err := keyring.Get(Service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return v, err
}

// Set creates or replaces an item.
func (Keychain) Set(account, value string) error { return keyring.Set(Service, account, value) }

// Delete removes an item.
func (Keychain) Delete(account string) error {
	err := keyring.Delete(Service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

// Kind implements Store.
func (Keychain) Kind() string { return "keychain" }

// Memory is an in-process store for tests.
type Memory struct {
	mu sync.Mutex
	m  map[string]string
}

// NewMemory returns an empty memory store.
func NewMemory() *Memory { return &Memory{m: map[string]string{}} }

// Get implements Store.
func (s *Memory) Get(a string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[a]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

// Set implements Store.
func (s *Memory) Set(a, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[a] = v
	return nil
}

// Delete implements Store.
func (s *Memory) Delete(a string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[a]; !ok {
		return ErrNotFound
	}
	delete(s.m, a)
	return nil
}

// Kind implements Store.
func (s *Memory) Kind() string { return "memory" }
