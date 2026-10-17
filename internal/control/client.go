package control

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// ErrClosed is returned when the connection to the worker is gone.
var ErrClosed = errors.New("control: connection to the clawdyx worker closed")

// Client is a control-channel client. It is safe for concurrent use.
type Client struct {
	c      net.Conn
	wmu    sync.Mutex
	mu     sync.Mutex
	next   atomic.Uint64
	wait   map[string]chan Response
	events chan Event
	done   chan struct{}
	err    error
	once   sync.Once
}

// Dial connects to the worker's socket.
func Dial(ctx context.Context, path string) (*Client, error) {
	var d net.Dialer
	c, err := d.DialContext(ctx, "unix", path)
	if err != nil {
		return nil, err
	}
	cl := &Client{c: c, wait: map[string]chan Response{}, events: make(chan Event, 512), done: make(chan struct{})}
	go cl.read()
	return cl, nil
}

// Events delivers server events after Subscribe. The channel is closed
// when the connection ends.
func (c *Client) Events() <-chan Event { return c.events }

// Done is closed when the connection ends.
func (c *Client) Done() <-chan struct{} { return c.done }

// Close closes the connection.
func (c *Client) Close() error {
	c.shutdown(ErrClosed)
	return nil
}

func (c *Client) shutdown(err error) {
	c.once.Do(func() {
		c.mu.Lock()
		c.err = err
		for _, ch := range c.wait {
			close(ch)
		}
		c.wait = map[string]chan Response{}
		c.mu.Unlock()
		c.c.Close()
		close(c.done)
		close(c.events)
	})
}

func (c *Client) read() {
	sc := bufio.NewScanner(c.c)
	sc.Buffer(make([]byte, 0, 64<<10), MaxLargeFrame+1024)
	for sc.Scan() {
		var r Response
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			continue
		}
		if r.Method == "event" {
			var ev Event
			if json.Unmarshal(r.Params, &ev) == nil {
				select {
				case c.events <- ev:
				default: // consumer too slow; drop
				}
			}
			continue
		}
		if len(r.ID) == 0 {
			if r.Error != nil && r.Error.Code == CodeForbidden {
				c.shutdown(r.Error)
				return
			}
			continue
		}
		c.mu.Lock()
		ch := c.wait[string(r.ID)]
		delete(c.wait, string(r.ID))
		c.mu.Unlock()
		if ch != nil {
			ch <- r
		}
	}
	c.shutdown(ErrClosed)
}

// Call invokes method with params (any JSON-encodable value or nil) and
// decodes the result into out (may be nil).
func (c *Client) Call(ctx context.Context, method string, params, out any) error {
	id := strconv.FormatUint(c.next.Add(1), 10)
	req := Request{JSONRPC: "2.0", ID: json.RawMessage(id), Method: method}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return err
		}
		req.Params = raw
	}
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	ch := make(chan Response, 1)
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		return c.err
	}
	c.wait[id] = ch
	c.mu.Unlock()
	c.wmu.Lock()
	_ = c.c.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err = c.c.Write(append(b, '\n'))
	c.wmu.Unlock()
	if err != nil {
		c.shutdown(ErrClosed)
		return ErrClosed
	}
	select {
	case r, ok := <-ch:
		if !ok {
			return ErrClosed
		}
		if r.Error != nil {
			return r.Error
		}
		if out != nil && len(r.Result) > 0 {
			if err := json.Unmarshal(r.Result, out); err != nil {
				return fmt.Errorf("control: decode %s result: %w", method, err)
			}
		}
		return nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.wait, id)
		c.mu.Unlock()
		return ctx.Err()
	}
}

// Subscribe asks the server to push events on this connection.
func (c *Client) Subscribe(ctx context.Context) error {
	return c.Call(ctx, "events.subscribe", nil, nil)
}
