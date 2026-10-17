package sessions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/Publicthrone547/clawdyX/internal/store"
)

// ErrBusy means the session has queued or running jobs.
var ErrBusy = errors.New("the session has a queued or running job")

// Busy reports whether a session has active jobs (set by the job layer).
type Busy func(sessionID int64) bool

func (s *Service) checkName(name string, except int64) error {
	if name == "" {
		return nil
	}
	if !ValidName(name) {
		return fmt.Errorf("session names may use letters, digits, '_' and '-' (max 32)")
	}
	if s.st.NameTaken(name, except) {
		return fmt.Errorf("another session is already called %q", name)
	}
	return nil
}

// CreateAndForget creates a new session exactly like Create and then forgets
// the previously active session, in one transaction. If there was no active
// session it behaves like Create. It refuses while the old session is busy.
func (s *Service) CreateAndForget(name string, busy Busy) (created, forgotten *store.Session, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkName(name, 0); err != nil {
		return nil, nil, err
	}
	old, err := s.Active()
	if err != nil {
		return nil, nil, err
	}
	ws := s.DefaultWorkspace()
	var oldID int64
	if old != nil {
		if busy != nil && busy(old.ID) {
			return nil, nil, fmt.Errorf("%s: %w", old.Label(), ErrBusy)
		}
		oldID = old.ID
	}
	x, err := s.st.CreateSessionAndForget(store.Session{Name: name, ClaudeSessionID: uuid.NewString(), WorkspaceName: ws.Name, WorkspacePath: ws.Canonical, Mode: ws.DefaultMode}, oldID, KeyActive)
	if err != nil {
		return nil, nil, err
	}
	s.st.Audit("session", "create", x.Label(), "ok", 0)
	if old != nil {
		s.st.Audit("session", "forget", old.Label(), "ok", 0)
		old, _ = s.st.GetSession(old.ID)
	}
	return x, old, nil
}

// Fresh starts a new session with the same workspace, tier, model and effort
// as the active one; the old session is kept. This is /clear: Claude Code's
// own /clear only exists inside a live interactive session.
func (s *Service) Fresh(busy Busy) (created, from *store.Session, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, err := s.Active()
	if err != nil {
		return nil, nil, err
	}
	if cur == nil {
		x, err := s.create("", s.DefaultWorkspace())
		return x, nil, err
	}
	if busy != nil && busy(cur.ID) {
		return nil, nil, fmt.Errorf("%s: %w", cur.Label(), ErrBusy)
	}
	name := ""
	if cur.Name != "" {
		base := cur.Name
		if len(base) > 26 {
			base = base[:26]
		}
		name = base + "-fresh"
		for i := 2; s.st.NameTaken(name, 0) && i < 100; i++ {
			name = base + "-fresh" + strconv.Itoa(i)
		}
	}
	x, err := s.st.CreateSession(store.Session{Name: name, ClaudeSessionID: uuid.NewString(), WorkspaceName: cur.WorkspaceName, WorkspacePath: cur.WorkspacePath,
		Mode: cur.Mode, Model: cur.Model, Effort: cur.Effort})
	if err != nil {
		return nil, nil, err
	}
	if err := s.st.SetSetting(KeyActive, strconv.FormatInt(x.ID, 10)); err != nil {
		return nil, nil, err
	}
	s.st.Audit("session", "fresh", x.Label(), "from "+cur.Label(), 0)
	return x, cur, nil
}

// Forget turns a session into a tombstone (hidden; restorable for 30 days).
// If it was active, no session is active afterwards.
func (s *Service) Forget(x *store.Session, busy Busy) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if busy != nil && busy(x.ID) {
		return fmt.Errorf("%s: %w", x.Label(), ErrBusy)
	}
	if err := s.st.MarkForgotten(x.ID, false); err != nil {
		return err
	}
	if s.st.Setting(KeyActive, "") == strconv.FormatInt(x.ID, 10) {
		_ = s.st.SetSetting(KeyActive, "0")
	}
	s.st.Audit("session", "forget", x.Label(), "ok", 0)
	return nil
}

// Rename changes only the label.
func (s *Service) Rename(x *store.Session, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name == "" {
		return errors.New("a name is required")
	}
	if err := s.checkName(name, x.ID); err != nil {
		return err
	}
	if err := s.st.SetSessionField(x.ID, "name", name); err != nil {
		return err
	}
	s.st.Audit("session", "rename", x.Label(), "ok", 0)
	return nil
}

// ---- Claude Code history (transcripts) ----

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var slugRe = regexp.MustCompile(`[^A-Za-z0-9]`)

// ProjectSlug is Claude Code's documented project folder name: the working
// directory with every non-alphanumeric character replaced by '-'. Names
// over 200 characters are shortened by Claude Code with a hash; we refuse
// those rather than guess.
func ProjectSlug(dir string) (string, bool) {
	slug := slugRe.ReplaceAllString(dir, "-")
	return slug, len(slug) <= 200
}

// History is the set of Claude Code files that belong to one session.
type History struct {
	Paths     []string // the transcript file and, if present, its sibling folder
	Bytes     int64
	Deletable bool
	Reason    string
}

// FindHistory locates a session's transcript under projectsDir using the
// documented layout <projects>/<slug>/<uuid>.jsonl (+ <uuid>/). If the
// workspace moved since the session was created, the transcript is found by
// its UUID in exactly one project folder; anything else is not offered.
func FindHistory(projectsDir string, x *store.Session) History {
	h := History{}
	if x.Origin != "bot" {
		h.Reason = "only sessions created by clawdyX can have their history deleted here"
		return h
	}
	if !uuidRe.MatchString(x.ClaudeSessionID) {
		h.Reason = "the session ID is not a Claude Code UUID"
		return h
	}
	if projectsDir == "" || !filepath.IsAbs(projectsDir) {
		h.Reason = "Claude Code's projects folder is unknown"
		return h
	}
	var jsonl string
	if slug, ok := ProjectSlug(x.WorkspacePath); ok {
		p := filepath.Join(projectsDir, slug, x.ClaudeSessionID+".jsonl")
		if isRegular(p) {
			jsonl = p
		}
	}
	if jsonl == "" {
		matches, _ := filepath.Glob(filepath.Join(projectsDir, "*", x.ClaudeSessionID+".jsonl"))
		var regular []string
		for _, m := range matches {
			if isRegular(m) {
				regular = append(regular, m)
			}
		}
		switch len(regular) {
		case 0:
			h.Reason = "no Claude Code history exists for this session (it may never have run, or Claude Code already removed it)"
			return h
		case 1:
			jsonl = regular[0]
		default:
			h.Reason = "the session's transcript exists in several project folders; not touching any"
			return h
		}
	}
	h.Paths = append(h.Paths, jsonl)
	h.Bytes += size(jsonl)
	dir := strings.TrimSuffix(jsonl, ".jsonl")
	if st, err := os.Lstat(dir); err == nil && st.IsDir() {
		h.Paths = append(h.Paths, dir)
		h.Bytes += treeSize(dir)
	}
	h.Deletable = true
	return h
}

// DeleteHistory removes exactly the paths FindHistory reported.
func DeleteHistory(projectsDir string, x *store.Session) ([]string, error) {
	h := FindHistory(projectsDir, x)
	if !h.Deletable {
		return nil, errors.New(h.Reason)
	}
	var removed []string
	for _, p := range h.Paths {
		// Defence in depth: never act outside the projects folder.
		rel, err := filepath.Rel(projectsDir, p)
		if err != nil || strings.HasPrefix(rel, "..") || !strings.Contains(filepath.Base(p), x.ClaudeSessionID) {
			return removed, fmt.Errorf("refusing to delete %s", p)
		}
		if err := os.RemoveAll(p); err != nil {
			return removed, err
		}
		removed = append(removed, p)
	}
	return removed, nil
}

func isRegular(p string) bool {
	st, err := os.Lstat(p)
	return err == nil && st.Mode().IsRegular()
}

func size(p string) int64 {
	st, err := os.Lstat(p)
	if err != nil {
		return 0
	}
	return st.Size()
}

func treeSize(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, e := d.Info(); e == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}
