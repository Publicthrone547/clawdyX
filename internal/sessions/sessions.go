// Package sessions manages logical sessions: which one is active, how
// references like "S3" or a name resolve, and the effective model/effort.
package sessions

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/store"
)

// Setting keys.
const (
	KeyActive           = "active_session"
	KeyDefaultModel     = "default_model"
	KeyDefaultEffort    = "default_effort"
	KeyDefaultWorkspace = "default_workspace"
)

// ErrNoSession means a reference did not match any session.
var ErrNoSession = errors.New("no such session")

var nameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// ValidName reports whether a session name is acceptable.
func ValidName(s string) bool { return nameRe.MatchString(s) }

// Service manages sessions.
type Service struct {
	mu  sync.Mutex
	st  *store.Store
	cfg *config.Config
}

// New returns a service.
func New(st *store.Store, cfg *config.Config) *Service { return &Service{st: st, cfg: cfg} }

// DefaultModel is the chat-wide model default.
func (s *Service) DefaultModel() string {
	return s.st.Setting(KeyDefaultModel, s.cfg.Claude.DefaultModel)
}

// DefaultEffort is the chat-wide effort default.
func (s *Service) DefaultEffort() string {
	return s.st.Setting(KeyDefaultEffort, s.cfg.Claude.DefaultEffort)
}

// DefaultWorkspace is the workspace for new sessions.
func (s *Service) DefaultWorkspace() *config.Workspace {
	name := s.st.Setting(KeyDefaultWorkspace, s.cfg.DefaultWorkspace)
	if ws, ok := s.cfg.Workspace(name); ok {
		return ws
	}
	ws, _ := s.cfg.Workspace(s.cfg.DefaultWorkspace)
	return ws
}

// SetDefaultWorkspace changes the workspace for future sessions.
func (s *Service) SetDefaultWorkspace(name string) error {
	if _, ok := s.cfg.Workspace(name); !ok {
		return fmt.Errorf("unknown workspace %q", name)
	}
	return s.st.SetSetting(KeyDefaultWorkspace, name)
}

// SetDefaultModel stores the chat-wide model default.
func (s *Service) SetDefaultModel(m string) error { return s.st.SetSetting(KeyDefaultModel, m) }

// SetDefaultEffort stores the chat-wide effort default.
func (s *Service) SetDefaultEffort(e string) error { return s.st.SetSetting(KeyDefaultEffort, e) }

// Effective returns the model and effort a job in this session will use.
func (s *Service) Effective(x *store.Session) (model, effort string) {
	model, effort = x.Model, x.Effort
	if model == "" {
		model = s.DefaultModel()
	}
	if effort == "" {
		effort = s.DefaultEffort()
	}
	return
}

// Active returns the active session, or nil if none.
func (s *Service) Active() (*store.Session, error) {
	id, err := strconv.ParseInt(s.st.Setting(KeyActive, "0"), 10, 64)
	if err != nil || id == 0 {
		return nil, nil
	}
	x, err := s.st.GetSession(id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if x.Status != "active" {
		return nil, nil
	}
	return x, nil
}

// EnsureActive returns the active session, creating one if needed.
func (s *Service) EnsureActive() (*store.Session, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, err := s.Active()
	if err != nil || x != nil {
		return x, false, err
	}
	x, err = s.create("", s.DefaultWorkspace())
	return x, err == nil, err
}

// Create makes a new session in ws (nil = default) and activates it.
func (s *Service) Create(name string, ws *config.Workspace) (*store.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name != "" && !ValidName(name) {
		return nil, fmt.Errorf("session name must match %s", nameRe)
	}
	if ws == nil {
		ws = s.DefaultWorkspace()
	}
	return s.create(name, ws)
}

func (s *Service) create(name string, ws *config.Workspace) (*store.Session, error) {
	x, err := s.st.CreateSession(store.Session{
		Name: name, ClaudeSessionID: uuid.NewString(), WorkspaceName: ws.Name, WorkspacePath: ws.Canonical, Mode: ws.DefaultMode,
	})
	if err != nil {
		return nil, err
	}
	if err := s.st.SetSetting(KeyActive, strconv.FormatInt(x.ID, 10)); err != nil {
		return nil, err
	}
	s.st.Audit("session", "create", x.Label(), "ok", 0)
	return x, nil
}

// Resolve finds a session by "S3", "3", a name, or a Claude UUID prefix (>= 8 hex chars).
func (s *Service) Resolve(ref string) (*store.Session, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, ErrNoSession
	}
	num := strings.TrimPrefix(strings.TrimPrefix(ref, "S"), "s")
	if id, err := strconv.ParseInt(num, 10, 64); err == nil && id > 0 {
		x, err := s.st.GetSession(id)
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNoSession
		}
		return x, err
	}
	if ValidName(ref) {
		if x, err := s.st.FindSessionByName(ref); err == nil {
			return x, nil
		}
	}
	if len(ref) >= 8 {
		all, err := s.st.ListSessions(true, 1000)
		if err != nil {
			return nil, err
		}
		var match *store.Session
		for i := range all {
			if strings.HasPrefix(all[i].ClaudeSessionID, strings.ToLower(ref)) {
				if match != nil {
					return nil, fmt.Errorf("ambiguous session reference %q", ref)
				}
				match = &all[i]
			}
		}
		if match != nil {
			return match, nil
		}
	}
	return nil, ErrNoSession
}

// Switch activates a session.
func (s *Service) Switch(ref string) (*store.Session, error) {
	x, err := s.Resolve(ref)
	if err != nil {
		return nil, err
	}
	if x.Status == "archived" {
		return nil, fmt.Errorf("%s is archived", x.Label())
	}
	if x.Status == "missing" {
		return nil, fmt.Errorf("%s can no longer be resumed (Claude transcript missing); use /new", x.Label())
	}
	if err := s.st.SetSetting(KeyActive, strconv.FormatInt(x.ID, 10)); err != nil {
		return nil, err
	}
	return x, nil
}

// Archive hides a session; if it was active, no session is active afterwards.
func (s *Service) Archive(x *store.Session) error {
	if err := s.st.SetSessionField(x.ID, "status", "archived"); err != nil {
		return err
	}
	if s.st.Setting(KeyActive, "") == strconv.FormatInt(x.ID, 10) {
		return s.st.SetSetting(KeyActive, "0")
	}
	return nil
}

// Workspace returns the configured workspace for a session.
func (s *Service) Workspace(x *store.Session) (*config.Workspace, error) {
	ws, ok := s.cfg.Workspace(x.WorkspaceName)
	if !ok {
		return nil, fmt.Errorf("workspace %q is no longer configured", x.WorkspaceName)
	}
	if ws.Canonical != x.WorkspacePath {
		return nil, fmt.Errorf("workspace %q moved since the session was created", x.WorkspaceName)
	}
	return ws, nil
}
