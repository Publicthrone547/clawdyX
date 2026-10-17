package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func sockPath(t *testing.T) string {
	t.Helper()
	// t.TempDir() on macOS can exceed the 104-byte sun_path limit.
	d, err := os.MkdirTemp("/tmp", "cx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(d) })
	return filepath.Join(d, "s", "control.sock")
}

func start(t *testing.T, setup func(*Server)) (*Server, string) {
	t.Helper()
	s := NewServer(nil)
	if setup != nil {
		setup(s)
	}
	p := sockPath(t)
	if err := s.Listen(p); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go s.Serve(ctx)
	t.Cleanup(func() { cancel(); s.Close() })
	return s, p
}

func dial(t *testing.T, p string) *Client {
	t.Helper()
	c, err := Dial(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestSocketPermissions(t *testing.T) {
	_, p := start(t, nil)
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode %v want 0600", st.Mode().Perm())
	}
	dst, _ := os.Stat(filepath.Dir(p))
	if dst.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode %v want 0700", dst.Mode().Perm())
	}
}

func TestCallAndErrors(t *testing.T) {
	_, p := start(t, func(s *Server) {
		s.Handle("echo", 0, func(_ context.Context, raw json.RawMessage) (any, error) {
			var v struct{ X int }
			if err := json.Unmarshal(raw, &v); err != nil {
				return nil, InvalidParams("bad params")
			}
			return map[string]int{"x": v.X * 2}, nil
		})
		s.Handle("fail", 0, func(context.Context, json.RawMessage) (any, error) { return nil, Errorf("nope %d", 7) })
		s.Handle("slow", 50*time.Millisecond, func(ctx context.Context, _ json.RawMessage) (any, error) {
			time.Sleep(300 * time.Millisecond)
			return nil, nil
		})
		s.Handle("panic", 0, func(context.Context, json.RawMessage) (any, error) { panic("boom") })
	})
	c := dial(t, p)
	ctx := context.Background()
	var out struct{ X int }
	if err := c.Call(ctx, "echo", map[string]int{"X": 21}, &out); err != nil || out.X != 42 {
		t.Fatalf("echo: %v %+v", err, out)
	}
	var e *Error
	if err := c.Call(ctx, "echo", "str", nil); !errors.As(err, &e) || e.Code != CodeInvalidParams {
		t.Fatalf("want invalid params, got %v", err)
	}
	if err := c.Call(ctx, "fail", nil, nil); !errors.As(err, &e) || e.Message != "nope 7" {
		t.Fatalf("want app error, got %v", err)
	}
	if err := c.Call(ctx, "missing", nil, nil); !errors.As(err, &e) || e.Code != CodeNoMethod {
		t.Fatalf("want no method, got %v", err)
	}
	if err := c.Call(ctx, "slow", nil, nil); !errors.As(err, &e) || e.Code != CodeTimeout {
		t.Fatalf("want timeout, got %v", err)
	}
	if err := c.Call(ctx, "panic", nil, nil); !errors.As(err, &e) || e.Message != "internal error" {
		t.Fatalf("want internal error, got %v", err)
	}
	// The connection survives all of the above.
	if err := c.Call(ctx, "ping", nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestForeignUIDRejected(t *testing.T) {
	_, p := start(t, func(s *Server) {
		s.PeerUID = func(*net.UnixConn) (int, error) { return s.UID + 1, nil }
	})
	c := dial(t, p)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Call(ctx, "ping", nil, nil); err == nil {
		t.Fatal("call from another uid must fail")
	}
	select {
	case <-c.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("connection not closed")
	}
}

func TestPeerUIDIsOwnUser(t *testing.T) {
	_, p := start(t, nil)
	c := dial(t, p)
	if err := c.Call(context.Background(), "ping", nil, nil); err != nil {
		t.Fatalf("own uid must be accepted: %v", err)
	}
}

func TestEventsOrderedAndConcurrentClients(t *testing.T) {
	s, p := start(t, nil)
	clients := make([]*Client, 3)
	for i := range clients {
		clients[i] = dial(t, p)
		if err := clients[i].Subscribe(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 50; i++ {
		s.Publish("tick", map[string]int{"i": i})
	}
	for ci, c := range clients {
		var last uint64
		for i := 0; i < 50; i++ {
			select {
			case ev := <-c.Events():
				if ev.Seq <= last || ev.Type != "tick" {
					t.Fatalf("client %d: out of order %d after %d", ci, ev.Seq, last)
				}
				last = ev.Seq
			case <-time.After(2 * time.Second):
				t.Fatalf("client %d: missing event %d", ci, i)
			}
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := clients[i%3].Call(context.Background(), "ping", nil, nil); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
}

func TestStaleSocketReplacedLiveRefused(t *testing.T) {
	_, p := start(t, nil)
	s2 := NewServer(nil)
	if err := s2.Listen(p); err == nil {
		t.Fatal("must refuse to take over a live socket")
	}
	// A regular file at the path is never removed.
	f := filepath.Join(filepath.Dir(p), "file.sock")
	os.WriteFile(f, []byte("x"), 0o600)
	if err := s2.Listen(f); err == nil {
		t.Fatal("must refuse a non-socket file")
	}
}

func TestOversizeAndGarbageFrames(t *testing.T) {
	_, p := start(t, nil)
	raw, err := net.Dial("unix", p)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	fmt.Fprintln(raw, "{not json")
	fmt.Fprintln(raw, `{"jsonrpc":"1.0","id":1,"method":"ping"}`)
	buf := make([]byte, 4096)
	raw.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _ := raw.Read(buf)
	if n == 0 {
		t.Fatal("expected error responses")
	}
	// A frame above MaxFrame ends that connection but not the server.
	big := make([]byte, MaxFrame+10)
	for i := range big {
		big[i] = 'a'
	}
	raw.Write(big)
	c := dial(t, p)
	if err := c.Call(context.Background(), "ping", nil, nil); err != nil {
		t.Fatalf("server must survive: %v", err)
	}
}

func FuzzFrames(f *testing.F) {
	f.Add([]byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	f.Add([]byte(`{"jsonrpc":"2.0","id":"x","method":"echo","params":[1,2]}`))
	f.Add([]byte(`{"jsonrpc":"2.0","method":"events.subscribe"}`))
	f.Add([]byte("\x00\xff{"))
	s := NewServer(nil)
	s.Handle("echo", time.Second, func(_ context.Context, raw json.RawMessage) (any, error) { return raw, nil })
	d, _ := os.MkdirTemp("/tmp", "cxf")
	defer os.RemoveAll(d)
	p := filepath.Join(d, "f.sock")
	if err := s.Listen(p); err != nil {
		f.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Serve(ctx)
	f.Fuzz(func(t *testing.T, frame []byte) {
		c, err := net.Dial("unix", p)
		if err != nil {
			t.Fatal(err)
		}
		c.Write(append(frame, '\n'))
		c.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		buf := make([]byte, 1024)
		c.Read(buf)
		c.Close()
		cl, err := Dial(context.Background(), p)
		if err != nil {
			t.Fatal(err)
		}
		defer cl.Close()
		cctx, ccancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer ccancel()
		if err := cl.Call(cctx, "ping", nil, nil); err != nil {
			t.Fatalf("server stopped answering after %q: %v", frame, err)
		}
	})
}
