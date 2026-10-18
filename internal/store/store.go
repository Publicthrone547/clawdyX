// Package store persists bot state in SQLite (CGO-free modernc driver).
// It stores only what is needed to recover safely: sessions, jobs, bounded job
// events, aliases, settings, processed update IDs, audit rows and a capability
// cache. It never stores secrets, full prompts, or full Claude transcripts.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("not found")

// ErrDuplicate is returned when a job for the same Telegram update exists.
var ErrDuplicate = errors.New("duplicate")

// Store wraps the database.
type Store struct {
	db   *sql.DB
	path string
}

const schemaVersion = 4

var migrations = []string{
	// v1
	`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
	CREATE TABLE sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL DEFAULT '',
		claude_session_id TEXT NOT NULL UNIQUE,
		workspace_name TEXT NOT NULL,
		workspace_path TEXT NOT NULL,
		mode TEXT NOT NULL,
		model TEXT,
		effort TEXT,
		status TEXT NOT NULL DEFAULT 'active',
		origin TEXT NOT NULL DEFAULT 'bot',
		started INTEGER NOT NULL DEFAULT 0,
		turns INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL,
		last_used_at INTEGER NOT NULL,
		last_job_id INTEGER
	);
	CREATE TABLE jobs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id INTEGER NOT NULL REFERENCES sessions(id),
		kind TEXT NOT NULL DEFAULT 'prompt',
		state TEXT NOT NULL,
		tg_update_id INTEGER UNIQUE,
		tg_message_id INTEGER NOT NULL DEFAULT 0,
		tg_ack_message_id INTEGER NOT NULL DEFAULT 0,
		prompt_sha256 TEXT NOT NULL DEFAULT '',
		prompt_preview TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		effort TEXT NOT NULL DEFAULT '',
		perm_mode TEXT NOT NULL DEFAULT '',
		pid INTEGER NOT NULL DEFAULT 0,
		proc_start_unix INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL,
		started_at INTEGER NOT NULL DEFAULT 0,
		finished_at INTEGER NOT NULL DEFAULT 0,
		exit_code INTEGER NOT NULL DEFAULT 0,
		terminal_reason TEXT NOT NULL DEFAULT '',
		is_error INTEGER NOT NULL DEFAULT 0,
		num_turns INTEGER NOT NULL DEFAULT 0,
		in_tokens INTEGER NOT NULL DEFAULT 0,
		out_tokens INTEGER NOT NULL DEFAULT 0,
		cache_read_tokens INTEGER NOT NULL DEFAULT 0,
		cache_create_tokens INTEGER NOT NULL DEFAULT 0,
		cost_usd_est REAL NOT NULL DEFAULT 0,
		resolved_model TEXT NOT NULL DEFAULT '',
		result_preview TEXT NOT NULL DEFAULT '',
		error_summary TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX jobs_session ON jobs(session_id, id);
	CREATE INDEX jobs_state ON jobs(state);
	CREATE TABLE job_events (
		job_id INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
		seq INTEGER NOT NULL,
		ts INTEGER NOT NULL,
		type TEXT NOT NULL,
		payload TEXT NOT NULL,
		PRIMARY KEY (job_id, seq)
	);
	CREATE TABLE aliases (name TEXT PRIMARY KEY, model TEXT NOT NULL, canonical TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL);
	CREATE TABLE processed_updates (update_id INTEGER PRIMARY KEY, received_at INTEGER NOT NULL);
	CREATE TABLE audit (id INTEGER PRIMARY KEY AUTOINCREMENT, ts INTEGER NOT NULL, category TEXT NOT NULL, action TEXT NOT NULL, target TEXT NOT NULL DEFAULT '', outcome TEXT NOT NULL DEFAULT '', job_id INTEGER);
	CREATE TABLE capability_cache (key TEXT PRIMARY KEY, value TEXT NOT NULL, cli_version TEXT NOT NULL, checked_at INTEGER NOT NULL);`,
	// v2: session tombstones (forget / delete history) and the capture audit.
	`ALTER TABLE sessions ADD COLUMN forgotten_at INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE sessions ADD COLUMN deleted_at INTEGER NOT NULL DEFAULT 0;
	CREATE TABLE captures (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ts INTEGER NOT NULL,
		display_key TEXT NOT NULL,
		width INTEGER NOT NULL DEFAULT 0,
		height INTEGER NOT NULL DEFAULT 0,
		bytes INTEGER NOT NULL DEFAULT 0,
		destination TEXT NOT NULL,
		requester TEXT NOT NULL DEFAULT '',
		job_id INTEGER
	);`,
	// v3: the Telegram chat each job reports to (several allowed users).
	`ALTER TABLE jobs ADD COLUMN tg_chat_id INTEGER NOT NULL DEFAULT 0;`,
	// v4: camera captures share the capture audit (kind, clip length).
	`ALTER TABLE captures ADD COLUMN kind TEXT NOT NULL DEFAULT 'screen';
	ALTER TABLE captures ADD COLUMN duration_ms INTEGER NOT NULL DEFAULT 0;`,
}

// Open opens (creating) the database at path with private permissions.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, err
		}
		f.Close()
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, path: path}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	var v int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	if v > schemaVersion {
		return fmt.Errorf("store: database schema v%d is newer than this binary (v%d); refusing to run", v, schemaVersion)
	}
	if v > 0 && v < len(migrations) && s.path != "" {
		dir := filepath.Join(filepath.Dir(s.path), "backups")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		_ = os.Chmod(dir, 0o700)
		bk := filepath.Join(dir, fmt.Sprintf("state-pre-v%d-%s.db", len(migrations), time.Now().Format("20060102-150405")))
		if err := s.Backup(context.Background(), bk); err != nil {
			return fmt.Errorf("store: backup before migration: %w", err)
		}
	}
	for i := v; i < len(migrations); i++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("store: migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func now() int64 { return time.Now().Unix() }

// ---- settings ----

// Setting returns a setting value or def.
func (s *Store) Setting(key, def string) string {
	var v string
	if err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v); err != nil {
		return def
	}
	return v
}

// SetSetting stores a setting.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// ---- sessions ----

// Session is a logical conversation bound to one Claude session and workspace.
type Session struct {
	ID              int64
	Name            string
	ClaudeSessionID string
	WorkspaceName   string
	WorkspacePath   string
	Mode            string
	Model           string // empty = chat default
	Effort          string // empty = chat default
	Status          string // active | archived | missing | forgotten
	Origin          string
	Started         bool // a Claude transcript exists (first job ran)
	Turns           int
	CreatedAt       time.Time
	LastUsedAt      time.Time
	LastJobID       int64
	ForgottenAt     time.Time // set when forgotten from the app
	DeletedAt       time.Time // set when its Claude Code history was deleted
}

// Label returns "S<id>".
func (x Session) Label() string { return fmt.Sprintf("S%d", x.ID) }

const sessionCols = `id,name,claude_session_id,workspace_name,workspace_path,mode,COALESCE(model,''),COALESCE(effort,''),status,origin,started,turns,created_at,last_used_at,COALESCE(last_job_id,0),forgotten_at,deleted_at`

func scanSession(r interface{ Scan(...any) error }) (*Session, error) {
	var x Session
	var started int
	var c, l, f, d int64
	if err := r.Scan(&x.ID, &x.Name, &x.ClaudeSessionID, &x.WorkspaceName, &x.WorkspacePath, &x.Mode, &x.Model, &x.Effort, &x.Status, &x.Origin, &started, &x.Turns, &c, &l, &x.LastJobID, &f, &d); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	x.Started = started != 0
	x.CreatedAt, x.LastUsedAt = time.Unix(c, 0), time.Unix(l, 0)
	if f > 0 {
		x.ForgottenAt = time.Unix(f, 0)
	}
	if d > 0 {
		x.DeletedAt = time.Unix(d, 0)
	}
	return &x, nil
}

// CreateSession inserts a session and returns it with its ID.
func (s *Store) CreateSession(x Session) (*Session, error) {
	t := now()
	res, err := s.db.Exec(`INSERT INTO sessions(name,claude_session_id,workspace_name,workspace_path,mode,model,effort,status,origin,started,created_at,last_used_at)
		VALUES(?,?,?,?,?,NULLIF(?,''),NULLIF(?,''),'active',?,?,?,?)`,
		x.Name, x.ClaudeSessionID, x.WorkspaceName, x.WorkspacePath, x.Mode, x.Model, x.Effort, orDefault(x.Origin, "bot"), b2i(x.Started), t, t)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetSession(id)
}

// GetSession returns a session by ID.
func (s *Store) GetSession(id int64) (*Session, error) {
	return scanSession(s.db.QueryRow(`SELECT `+sessionCols+` FROM sessions WHERE id=?`, id))
}

// FindSessionByName returns the most recent session with that name.
func (s *Store) FindSessionByName(name string) (*Session, error) {
	return scanSession(s.db.QueryRow(`SELECT `+sessionCols+` FROM sessions WHERE name=? AND status NOT IN ('archived','forgotten') ORDER BY id DESC LIMIT 1`, name))
}

// ListSessions lists sessions, most recently used first.
func (s *Store) ListSessions(includeArchived bool, limit int) ([]Session, error) {
	q := `SELECT ` + sessionCols + ` FROM sessions WHERE status!='forgotten'`
	if !includeArchived {
		q += ` AND status!='archived'`
	}
	q += ` ORDER BY last_used_at DESC, id DESC LIMIT ?`
	rows, err := s.db.Query(q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		x, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}

// CountSessions returns active and total counts.
func (s *Store) CountSessions() (active, total int) {
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE status='active'`).Scan(&active)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE status!='forgotten'`).Scan(&total)
	return
}

// SetSessionField updates one whitelisted column of a session.
func (s *Store) SetSessionField(id int64, field, value string) error {
	switch field {
	case "model", "effort":
		_, err := s.db.Exec(`UPDATE sessions SET `+field+`=NULLIF(?,'') WHERE id=?`, value, id)
		return err
	case "mode", "status", "name":
		_, err := s.db.Exec(`UPDATE sessions SET `+field+`=? WHERE id=?`, value, id)
		return err
	}
	return fmt.Errorf("store: field %q not updatable", field)
}

// ListForgotten returns sessions forgotten within the last window, newest first.
func (s *Store) ListForgotten(window time.Duration) ([]Session, error) {
	rows, err := s.db.Query(`SELECT `+sessionCols+` FROM sessions WHERE status='forgotten' AND forgotten_at>=? ORDER BY forgotten_at DESC, id DESC LIMIT 200`, now()-int64(window.Seconds()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		x, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}

// MarkForgotten turns a session into a tombstone. withDeleted also records
// that its Claude Code history was deleted.
func (s *Store) MarkForgotten(id int64, withDeleted bool) error {
	t := now()
	d := int64(0)
	if withDeleted {
		d = t
	}
	_, err := s.db.Exec(`UPDATE sessions SET status='forgotten', forgotten_at=?, deleted_at=MAX(deleted_at,?) WHERE id=?`, t, d, id)
	return err
}

// MarkHistoryDeleted records that a session's Claude Code history is gone.
func (s *Store) MarkHistoryDeleted(id int64) error {
	_, err := s.db.Exec(`UPDATE sessions SET deleted_at=? WHERE id=?`, now(), id)
	return err
}

// Restore makes an archived or forgotten session active again.
func (s *Store) Restore(id int64) error {
	_, err := s.db.Exec(`UPDATE sessions SET status='active', forgotten_at=0 WHERE id=?`, id)
	return err
}

// CreateSessionAndForget inserts x, makes it the active session and turns
// oldID (if > 0) into a tombstone, all in one transaction: a failure
// leaves the previous state untouched.
func (s *Store) CreateSessionAndForget(x Session, oldID int64, activeKey string) (*Session, error) {
	var id int64
	err := s.Tx(func(tx *sql.Tx) error {
		t := now()
		res, err := tx.Exec(`INSERT INTO sessions(name,claude_session_id,workspace_name,workspace_path,mode,model,effort,status,origin,started,created_at,last_used_at)
			VALUES(?,?,?,?,?,NULLIF(?,''),NULLIF(?,''),'active',?,0,?,?)`,
			x.Name, x.ClaudeSessionID, x.WorkspaceName, x.WorkspacePath, x.Mode, x.Model, x.Effort, orDefault(x.Origin, "bot"), t, t)
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		if _, err := tx.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, activeKey, fmt.Sprint(id)); err != nil {
			return err
		}
		if oldID > 0 {
			r, err := tx.Exec(`UPDATE sessions SET status='forgotten', forgotten_at=? WHERE id=? AND status!='forgotten'`, t, oldID)
			if err != nil {
				return err
			}
			if n, _ := r.RowsAffected(); n != 1 {
				return fmt.Errorf("store: session %d could not be forgotten", oldID)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetSession(id)
}

// NameTaken reports whether a non-forgotten session other than except uses name.
func (s *Store) NameTaken(name string, except int64) bool {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE name=? AND status!='forgotten' AND id!=?`, name, except).Scan(&n)
	return n > 0
}

// Tx runs f in one transaction.
func (s *Store) Tx(f func(tx *sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if err := f(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// TouchSession records a completed turn.
func (s *Store) TouchSession(id, jobID int64, started bool, turnDelta int) error {
	_, err := s.db.Exec(`UPDATE sessions SET last_used_at=?, last_job_id=?, started=MAX(started,?), turns=turns+? WHERE id=?`, now(), jobID, b2i(started), turnDelta, id)
	return err
}

// ---- jobs ----

// Job states.
const (
	JobQueued      = "queued"
	JobRunning     = "running"
	JobCancelling  = "cancelling"
	JobCompleted   = "completed"
	JobFailed      = "failed"
	JobTimeout     = "timeout"
	JobCancelled   = "cancelled"
	JobInterrupted = "interrupted"
)

// Terminal reports whether a state is final.
func Terminal(state string) bool {
	switch state {
	case JobCompleted, JobFailed, JobTimeout, JobCancelled, JobInterrupted:
		return true
	}
	return false
}

// Job is one child-process run.
type Job struct {
	ID              int64
	SessionID       int64
	Kind            string
	State           string
	TgUpdateID      int64
	TgMessageID     int
	TgAckMessageID  int
	TgChatID        int64 // 0 = the owner's chat
	PromptSHA256    string
	PromptPreview   string
	Model           string
	Effort          string
	PermMode        string
	PID             int
	ProcStartUnix   int64
	CreatedAt       time.Time
	StartedAt       time.Time
	FinishedAt      time.Time
	ExitCode        int
	TerminalReason  string
	IsError         bool
	NumTurns        int
	InTokens        int64
	OutTokens       int64
	CacheReadTokens int64
	CacheCreate     int64
	CostUSDEst      float64
	ResolvedModel   string
	ResultPreview   string
	ErrorSummary    string
}

// Label returns "j-<id>".
func (j Job) Label() string { return fmt.Sprintf("j-%d", j.ID) }

const jobCols = `id,session_id,kind,state,COALESCE(tg_update_id,0),tg_message_id,tg_ack_message_id,prompt_sha256,prompt_preview,model,effort,perm_mode,pid,proc_start_unix,created_at,started_at,finished_at,exit_code,terminal_reason,is_error,num_turns,in_tokens,out_tokens,cache_read_tokens,cache_create_tokens,cost_usd_est,resolved_model,result_preview,error_summary,tg_chat_id`

func ts(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return time.Unix(v, 0)
}

func scanJob(r interface{ Scan(...any) error }) (*Job, error) {
	var j Job
	var c, st, fin int64
	var isErr int
	if err := r.Scan(&j.ID, &j.SessionID, &j.Kind, &j.State, &j.TgUpdateID, &j.TgMessageID, &j.TgAckMessageID, &j.PromptSHA256, &j.PromptPreview, &j.Model, &j.Effort, &j.PermMode, &j.PID, &j.ProcStartUnix, &c, &st, &fin, &j.ExitCode, &j.TerminalReason, &isErr, &j.NumTurns, &j.InTokens, &j.OutTokens, &j.CacheReadTokens, &j.CacheCreate, &j.CostUSDEst, &j.ResolvedModel, &j.ResultPreview, &j.ErrorSummary, &j.TgChatID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	j.CreatedAt, j.StartedAt, j.FinishedAt = ts(c), ts(st), ts(fin)
	j.IsError = isErr != 0
	return &j, nil
}

// InsertJob creates a queued job. A repeated Telegram update ID yields ErrDuplicate.
func (s *Store) InsertJob(j Job) (*Job, error) {
	var upd any
	if j.TgUpdateID != 0 {
		upd = j.TgUpdateID
	}
	res, err := s.db.Exec(`INSERT INTO jobs(session_id,kind,state,tg_update_id,tg_message_id,tg_chat_id,prompt_sha256,prompt_preview,model,effort,perm_mode,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, j.SessionID, orDefault(j.Kind, "prompt"), JobQueued, upd, j.TgMessageID, j.TgChatID, j.PromptSHA256, j.PromptPreview, j.Model, j.Effort, j.PermMode, now())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetJob(id)
}

// GetJob returns a job by ID.
func (s *Store) GetJob(id int64) (*Job, error) {
	return scanJob(s.db.QueryRow(`SELECT `+jobCols+` FROM jobs WHERE id=?`, id))
}

// ListJobs returns the most recent jobs.
func (s *Store) ListJobs(limit int) ([]Job, error) {
	return s.queryJobs(`SELECT `+jobCols+` FROM jobs ORDER BY id DESC LIMIT ?`, limit)
}

// JobsInStates returns jobs in any of the given states, oldest first.
func (s *Store) JobsInStates(states ...string) ([]Job, error) {
	if len(states) == 0 {
		return nil, nil
	}
	q := `SELECT ` + jobCols + ` FROM jobs WHERE state IN (?` + strings.Repeat(",?", len(states)-1) + `) ORDER BY id`
	args := make([]any, len(states))
	for i, v := range states {
		args[i] = v
	}
	return s.queryJobs(q, args...)
}

func (s *Store) queryJobs(q string, args ...any) ([]Job, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, rows.Err()
}

// SetJobState updates a job's state.
func (s *Store) SetJobState(id int64, state string) error {
	_, err := s.db.Exec(`UPDATE jobs SET state=? WHERE id=?`, state, id)
	return err
}

// SetJobAck stores the acknowledgement message ID.
func (s *Store) SetJobAck(id int64, msgID int) error {
	_, err := s.db.Exec(`UPDATE jobs SET tg_ack_message_id=? WHERE id=?`, msgID, id)
	return err
}

// MarkJobStarted records the running process.
func (s *Store) MarkJobStarted(id int64, pid int, procStart int64) error {
	_, err := s.db.Exec(`UPDATE jobs SET state=?, pid=?, proc_start_unix=?, started_at=? WHERE id=?`, JobRunning, pid, procStart, now(), id)
	return err
}

// FinishJob stores the final outcome.
func (s *Store) FinishJob(j Job) error {
	_, err := s.db.Exec(`UPDATE jobs SET state=?, finished_at=?, exit_code=?, terminal_reason=?, is_error=?, num_turns=?, in_tokens=?, out_tokens=?, cache_read_tokens=?, cache_create_tokens=?, cost_usd_est=?, resolved_model=?, result_preview=?, error_summary=? WHERE id=?`,
		j.State, now(), j.ExitCode, j.TerminalReason, b2i(j.IsError), j.NumTurns, j.InTokens, j.OutTokens, j.CacheReadTokens, j.CacheCreate, j.CostUSDEst, j.ResolvedModel, j.ResultPreview, j.ErrorSummary, j.ID)
	return err
}

// JobStats aggregates job counts and token sums since a time.
type JobStats struct {
	ByState         map[string]int
	InTokens        int64
	OutTokens       int64
	CacheReadTokens int64
	CacheCreate     int64
	CostUSDEst      float64
	Jobs            int
}

// Stats returns aggregates for prompt jobs created since t.
func (s *Store) Stats(since time.Time) (JobStats, error) {
	st := JobStats{ByState: map[string]int{}}
	rows, err := s.db.Query(`SELECT state, COUNT(*) FROM jobs WHERE created_at>=? GROUP BY state`, since.Unix())
	if err != nil {
		return st, err
	}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			rows.Close()
			return st, err
		}
		st.ByState[k] = n
		st.Jobs += n
	}
	rows.Close()
	err = s.db.QueryRow(`SELECT COALESCE(SUM(in_tokens),0),COALESCE(SUM(out_tokens),0),COALESCE(SUM(cache_read_tokens),0),COALESCE(SUM(cache_create_tokens),0),COALESCE(SUM(cost_usd_est),0) FROM jobs WHERE created_at>=?`, since.Unix()).
		Scan(&st.InTokens, &st.OutTokens, &st.CacheReadTokens, &st.CacheCreate, &st.CostUSDEst)
	return st, err
}

// ---- job events ----

// JobEvent is a bounded, redacted activity record.
type JobEvent struct {
	JobID   int64
	Seq     int
	TS      time.Time
	Type    string
	Payload string
}

// MaxEventsPerJob bounds stored events per job.
const MaxEventsPerJob = 500

// AddJobEvent appends an event unless the job reached the bound.
func (s *Store) AddJobEvent(jobID int64, seq int, typ, payload string) error {
	if seq > MaxEventsPerJob {
		return nil
	}
	if len(payload) > 1024 {
		payload = payload[:1024]
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO job_events(job_id,seq,ts,type,payload) VALUES(?,?,?,?,?)`, jobID, seq, now(), typ, payload)
	return err
}

// JobEvents returns the last n events of a job in order.
func (s *Store) JobEvents(jobID int64, n int) ([]JobEvent, error) {
	rows, err := s.db.Query(`SELECT job_id,seq,ts,type,payload FROM (SELECT * FROM job_events WHERE job_id=? ORDER BY seq DESC LIMIT ?) ORDER BY seq`, jobID, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []JobEvent
	for rows.Next() {
		var e JobEvent
		var t int64
		if err := rows.Scan(&e.JobID, &e.Seq, &t, &e.Type, &e.Payload); err != nil {
			return nil, err
		}
		e.TS = time.Unix(t, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---- aliases ----

// Alias maps a friendly name to a model.
type Alias struct {
	Name      string
	Model     string
	Canonical string
	CreatedAt time.Time
}

// SetAlias creates or replaces an alias.
func (s *Store) SetAlias(a Alias) error {
	_, err := s.db.Exec(`INSERT INTO aliases(name,model,canonical,created_at) VALUES(?,?,?,?) ON CONFLICT(name) DO UPDATE SET model=excluded.model, canonical=excluded.canonical, created_at=excluded.created_at`, a.Name, a.Model, a.Canonical, now())
	return err
}

// DeleteAlias removes an alias; ErrNotFound if absent.
func (s *Store) DeleteAlias(name string) error {
	res, err := s.db.Exec(`DELETE FROM aliases WHERE name=?`, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetAlias returns an alias.
func (s *Store) GetAlias(name string) (*Alias, error) {
	var a Alias
	var c int64
	err := s.db.QueryRow(`SELECT name,model,canonical,created_at FROM aliases WHERE name=?`, name).Scan(&a.Name, &a.Model, &a.Canonical, &c)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	a.CreatedAt = time.Unix(c, 0)
	return &a, err
}

// ListAliases lists all aliases.
func (s *Store) ListAliases() ([]Alias, error) {
	rows, err := s.db.Query(`SELECT name,model,canonical,created_at FROM aliases ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Alias
	for rows.Next() {
		var a Alias
		var c int64
		if err := rows.Scan(&a.Name, &a.Model, &a.Canonical, &c); err != nil {
			return nil, err
		}
		a.CreatedAt = time.Unix(c, 0)
		out = append(out, a)
	}
	return out, rows.Err()
}

// ---- processed updates ----

// MarkUpdate records an update ID; it returns false if it was already seen.
func (s *Store) MarkUpdate(updateID int64) (bool, error) {
	res, err := s.db.Exec(`INSERT OR IGNORE INTO processed_updates(update_id,received_at) VALUES(?,?)`, updateID, now())
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ---- audit ----

// Audit appends an audit row (never containing prompts or secrets).
func (s *Store) Audit(category, action, target, outcome string, jobID int64) {
	var j any
	if jobID != 0 {
		j = jobID
	}
	_, _ = s.db.Exec(`INSERT INTO audit(ts,category,action,target,outcome,job_id) VALUES(?,?,?,?,?,?)`, now(), category, action, target, outcome, j)
}

// ---- capability cache ----

// Capability returns a cached value valid for the CLI version and max age.
func (s *Store) Capability(key, cliVersion string, maxAge time.Duration) (string, bool) {
	var v, ver string
	var at int64
	if err := s.db.QueryRow(`SELECT value,cli_version,checked_at FROM capability_cache WHERE key=?`, key).Scan(&v, &ver, &at); err != nil {
		return "", false
	}
	if ver != cliVersion || time.Since(time.Unix(at, 0)) > maxAge {
		return "", false
	}
	return v, true
}

// SetCapability stores a capability value.
func (s *Store) SetCapability(key, value, cliVersion string) error {
	_, err := s.db.Exec(`INSERT INTO capability_cache(key,value,cli_version,checked_at) VALUES(?,?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, cli_version=excluded.cli_version, checked_at=excluded.checked_at`, key, value, cliVersion, now())
	return err
}

// ClearCapabilities empties the cache (after a CLI version change).
func (s *Store) ClearCapabilities() error {
	_, err := s.db.Exec(`DELETE FROM capability_cache`)
	return err
}

// ---- captures audit ----

// Capture is one audit row of a screen or camera capture (never pixels).
type Capture struct {
	Kind          string // screen (default) | camera-photo | camera-video
	DisplayKey    string // the display's key, or the camera's
	Width, Height int
	Bytes         int
	DurationMS    int64  // camera videos
	Destination   string // preview | telegram | claude | mcp
	Requester     string
	JobID         int64
}

// AuditCapture records a capture.
func (s *Store) AuditCapture(c Capture) {
	if c.Kind == "" {
		c.Kind = "screen"
	}
	_, _ = s.db.Exec(`INSERT INTO captures(ts,kind,display_key,width,height,bytes,duration_ms,destination,requester,job_id) VALUES(?,?,?,?,?,?,?,?,?,NULLIF(?,0))`,
		now(), c.Kind, c.DisplayKey, c.Width, c.Height, c.Bytes, c.DurationMS, c.Destination, c.Requester, c.JobID)
}

// RecentCaptures returns the newest capture audit rows, newest first.
func (s *Store) RecentCaptures(n int) ([]Capture, error) {
	rows, err := s.db.Query(`SELECT kind,display_key,width,height,bytes,duration_ms,destination,requester,COALESCE(job_id,0) FROM captures ORDER BY id DESC LIMIT ?`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Capture
	for rows.Next() {
		var c Capture
		if err := rows.Scan(&c.Kind, &c.DisplayKey, &c.Width, &c.Height, &c.Bytes, &c.DurationMS, &c.Destination, &c.Requester, &c.JobID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CaptureCount returns how many captures were recorded since t.
func (s *Store) CaptureCount(since time.Time) int {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM captures WHERE ts>=?`, since.Unix()).Scan(&n)
	return n
}

// ---- maintenance ----

// Retention in days.
type Retention struct {
	JobsDays, JobEventsDays, ProcessedUpdatesDays, AuditDays int
}

// Prune deletes rows older than the retention policy.
func (s *Store) Prune(r Retention) error {
	day := int64(86400)
	t := now()
	stmts := []struct {
		q    string
		days int
	}{
		{`DELETE FROM job_events WHERE ts < ?`, r.JobEventsDays},
		{`DELETE FROM processed_updates WHERE received_at < ?`, r.ProcessedUpdatesDays},
		{`DELETE FROM audit WHERE ts < ?`, r.AuditDays},
		{`DELETE FROM jobs WHERE created_at < ? AND state IN ('completed','failed','timeout','cancelled','interrupted')`, r.JobsDays},
		{`DELETE FROM captures WHERE ts < ?`, r.AuditDays},
	}
	for _, st := range stmts {
		if st.days <= 0 {
			continue
		}
		if _, err := s.db.Exec(st.q, t-int64(st.days)*day); err != nil {
			return err
		}
	}
	return nil
}

// Backup writes a consistent copy with VACUUM INTO.
func (s *Store) Backup(ctx context.Context, path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("store: backup target %s exists", path)
	}
	_, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, path)
	if err == nil {
		_ = os.Chmod(path, 0o600)
	}
	return err
}

// QuickCheck runs PRAGMA quick_check.
func (s *Store) QuickCheck() error {
	var r string
	if err := s.db.QueryRow(`PRAGMA quick_check`).Scan(&r); err != nil {
		return err
	}
	if r != "ok" {
		return fmt.Errorf("quick_check: %s", r)
	}
	return nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
