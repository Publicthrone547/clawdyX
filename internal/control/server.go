package control

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Handler serves one method. params is the raw JSON params (may be empty).
type Handler func(ctx context.Context, params json.RawMessage) (any, error)

type method struct {
	h       Handler
	timeout time.Duration
	large   bool
}

// PeerUIDFunc returns the uid of the process on the other end of conn.
type PeerUIDFunc func(conn *net.UnixConn) (int, error)

// Server serves the control protocol.
type Server struct {
	Log *slog.Logger
	// PeerUID is replaceable in tests; the default reads LOCAL_PEERCRED.
	PeerUID PeerUIDFunc
	// UID is the only uid allowed to connect (default os.Getuid()).
	UID int

	mu      sync.RWMutex
	methods map[string]method
	subs    map[*conn]struct{}
	seq     atomic.Uint64

	ln      net.Listener
	path    string
	closed  atomic.Bool
	wg      sync.WaitGroup
	conns   sync.Map
	Dropped atomic.Uint64 // events dropped for slow subscribers
}

// NewServer returns a server with the built-in methods (ping, events.subscribe).
func NewServer(log *slog.Logger) *Server {
	if log == nil {
		log = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	s := &Server{Log: log, PeerUID: peerUID, UID: os.Getuid(), methods: map[string]method{}, subs: map[*conn]struct{}{}}
	s.Handle("ping", 5*time.Second, func(context.Context, json.RawMessage) (any, error) {
		return map[string]any{"pong": true, "schema": ProtocolSchema}, nil
	})
	return s
}

// Handle registers a method with a timeout (0 = 30 s).
func (s *Server) Handle(name string, timeout time.Duration, h Handler) {
	s.handle(name, timeout, false, h)
}

// HandleLarge registers a method whose response may exceed MaxFrame (images).
func (s *Server) HandleLarge(name string, timeout time.Duration, h Handler) {
	s.handle(name, timeout, true, h)
}

func (s *Server) handle(name string, timeout time.Duration, large bool, h Handler) {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	s.mu.Lock()
	s.methods[name] = method{h: h, timeout: timeout, large: large}
	s.mu.Unlock()
}

// Methods lists registered method names (for `clawdyx ctl help`).
func (s *Server) Methods() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.methods))
	for k := range s.methods {
		out = append(out, k)
	}
	return out
}

// Listen creates the socket at path (removing a stale one) with mode 0600.
// The parent directory must already exist; it is forced to 0700.
func (s *Server) Listen(path string) error {
	if len(path) > 103 {
		return fmt.Errorf("control socket path is too long (%d > 103 bytes): %s", len(path), path)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	_ = os.Chmod(dir, 0o700)
	if st, err := os.Lstat(path); err == nil {
		if !isSocket(st) {
			return fmt.Errorf("%s exists and is not a socket; refusing to replace it", path)
		}
		if c, err := net.DialTimeout("unix", path, 300*time.Millisecond); err == nil {
			c.Close()
			return fmt.Errorf("another process is serving %s", path)
		}
		_ = os.Remove(path)
	}
	// The 0700 parent directory already keeps other users out between
	// Listen and Chmod; no process-wide umask change is needed.
	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	if err := chmodSocket(path); err != nil {
		ln.Close()
		return err
	}
	ln.(*net.UnixListener).SetUnlinkOnClose(true)
	s.ln, s.path = ln, path
	return nil
}

// Serve accepts connections until ctx ends or Close is called.
func (s *Server) Serve(ctx context.Context) error {
	if s.ln == nil {
		return errors.New("control: Listen first")
	}
	go func() {
		<-ctx.Done()
		s.Close()
	}()
	for {
		c, err := s.ln.Accept()
		if err != nil {
			if s.closed.Load() {
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return err
		}
		uc, ok := c.(*net.UnixConn)
		if !ok {
			c.Close()
			continue
		}
		uid, err := s.PeerUID(uc)
		if err != nil || uid != s.UID {
			s.Log.Warn("control: rejected connection from another user", "peer_uid", uid)
			_ = writeLine(uc, Response{JSONRPC: "2.0", Error: &Error{Code: CodeForbidden, Message: "forbidden"}})
			uc.Close()
			continue
		}
		cn := &conn{s: s, c: uc, out: make(chan []byte, 256), done: make(chan struct{}), sem: make(chan struct{}, maxInflight)}
		s.conns.Store(cn, true)
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			cn.run(ctx)
			s.conns.Delete(cn)
		}()
	}
}

// Close stops accepting, closes all connections and removes the socket.
func (s *Server) Close() {
	if !s.closed.CompareAndSwap(false, true) {
		return
	}
	if s.ln != nil {
		s.ln.Close()
	}
	s.conns.Range(func(k, _ any) bool {
		k.(*conn).close()
		return true
	})
	s.wg.Wait()
}

// Publish sends an event to every subscribed connection. Slow subscribers
// lose events (counted in Dropped) rather than blocking the worker.
func (s *Server) Publish(typ string, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		return
	}
	ev := Event{Type: typ, Seq: s.seq.Add(1), TS: time.Now().UnixMilli(), Data: raw}
	params, _ := json.Marshal(ev)
	line, _ := json.Marshal(Response{JSONRPC: "2.0", Method: "event", Params: params})
	line = append(line, '\n')
	s.mu.RLock()
	defer s.mu.RUnlock()
	for c := range s.subs {
		select {
		case c.out <- line:
		default:
			s.Dropped.Add(1)
		}
	}
}

type conn struct {
	s    *Server
	c    *net.UnixConn
	out  chan []byte
	done chan struct{}
	once sync.Once
	sem  chan struct{} // bounds concurrent handlers; reading waits for a slot
}

func (c *conn) close() {
	c.once.Do(func() {
		close(c.done)
		c.c.Close()
		c.s.mu.Lock()
		delete(c.s.subs, c)
		c.s.mu.Unlock()
	})
}

// maxInflight bounds concurrent requests per connection. Further requests
// wait (backpressure) instead of failing.
const maxInflight = 16

func (c *conn) run(ctx context.Context) {
	defer c.close()
	go c.writer()
	sc := bufio.NewScanner(c.c)
	sc.Buffer(make([]byte, 0, 64<<10), MaxFrame)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			c.send(Response{JSONRPC: "2.0", Error: &Error{Code: CodeParse, Message: "parse error"}}, false)
			continue
		}
		if req.JSONRPC != "2.0" || req.Method == "" {
			c.send(Response{JSONRPC: "2.0", ID: req.ID, Error: &Error{Code: CodeInvalidRequest, Message: "invalid request"}}, false)
			continue
		}
		if req.Method == "events.subscribe" {
			c.s.mu.Lock()
			c.s.subs[c] = struct{}{}
			c.s.mu.Unlock()
			c.reply(req, map[string]any{"subscribed": true}, nil, false)
			continue
		}
		c.s.mu.RLock()
		m, ok := c.s.methods[req.Method]
		c.s.mu.RUnlock()
		if !ok {
			c.reply(req, nil, &Error{Code: CodeNoMethod, Message: "unknown method " + req.Method}, false)
			continue
		}
		select {
		case c.sem <- struct{}{}:
		case <-c.done:
			return
		}
		params := append(json.RawMessage(nil), req.Params...)
		go func(req Request) {
			defer func() { <-c.sem }()
			cctx, cancel := context.WithTimeout(ctx, m.timeout)
			defer cancel()
			type res struct {
				v   any
				err error
			}
			ch := make(chan res, 1)
			go func() {
				defer func() {
					if r := recover(); r != nil {
						c.s.Log.Error("control: handler panic", "method", req.Method, "panic", fmt.Sprint(r))
						ch <- res{nil, &Error{Code: CodeApp, Message: "internal error"}}
					}
				}()
				v, err := m.h(cctx, params)
				ch <- res{v, err}
			}()
			select {
			case r := <-ch:
				var e *Error
				if r.err != nil {
					e = asError(r.err)
				}
				c.reply(req, r.v, e, m.large)
			case <-cctx.Done():
				c.reply(req, nil, &Error{Code: CodeTimeout, Message: fmt.Sprintf("%s timed out after %s", req.Method, m.timeout)}, false)
			}
		}(req)
	}
}

func (c *conn) reply(req Request, v any, e *Error, large bool) {
	if len(req.ID) == 0 { // notification: no response
		return
	}
	r := Response{JSONRPC: "2.0", ID: req.ID, Error: e}
	if e == nil {
		raw, err := json.Marshal(v)
		if err != nil {
			r.Error = &Error{Code: CodeApp, Message: "unencodable result"}
		} else {
			r.Result = raw
		}
	}
	c.send(r, large)
}

func (c *conn) send(r Response, large bool) {
	b, err := json.Marshal(r)
	if err != nil {
		return
	}
	limit := MaxFrame
	if large {
		limit = MaxLargeFrame
	}
	if len(b) > limit {
		b, _ = json.Marshal(Response{JSONRPC: "2.0", ID: r.ID, Error: &Error{Code: CodeApp, Message: "response too large"}})
	}
	b = append(b, '\n')
	select {
	case c.out <- b:
	case <-c.done:
	}
}

func (c *conn) writer() {
	for {
		select {
		case b := <-c.out:
			_ = c.c.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := c.c.Write(b); err != nil {
				c.close()
				return
			}
		case <-c.done:
			return
		}
	}
}

func writeLine(c net.Conn, r Response) error {
	b, _ := json.Marshal(r)
	_ = c.SetWriteDeadline(time.Now().Add(time.Second))
	_, err := c.Write(append(b, '\n'))
	return err
}
