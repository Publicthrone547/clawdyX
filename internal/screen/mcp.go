package screen

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Server is a minimal MCP (JSON-RPC 2.0 over newline-delimited stdio) server.
type Server struct {
	D       Desktop
	Version string
	// Authorize, when set, asks the worker whether a capture is allowed now
	// (screen features switched off, permission) and records it.
	Authorize func(ctx context.Context, display string) error
	// Displays lists the connected displays for list_displays (captures nothing).
	Displays func(ctx context.Context) ([]Display, error)

	mu   sync.Mutex
	last *Shot // last screenshot, used to map image pixels to screen points
}

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcErr         `json:"error,omitempty"`
}

type content struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

type toolResult struct {
	Content []content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

func obj(props map[string]any, required ...string) map[string]any {
	m := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		m["required"] = required
	}
	return m
}

var num = map[string]any{"type": "number"}

// Tools is the advertised tool list.
func Tools() []map[string]any {
	return []map[string]any{
		{"name": "list_displays", "description": "List the computer's connected displays (number, name, resolution, which one is main). Captures nothing. Call it first when the user may mean a display other than the main one, or asks about all screens.",
			"inputSchema": obj(map[string]any{})},
		{"name": "screenshot", "description": "Take a screenshot of one of the computer's displays (display 1 is the main display; omit display for it). The computer may have several displays: use list_displays to see them, and take one screenshot per display to look at all of them. Always take a fresh screenshot before clicking and after each action to check the result. Coordinates for the other tools are pixels in the most recent screenshot, on the display it shows.",
			"inputSchema": obj(map[string]any{"display": map[string]any{"type": "integer", "minimum": 1, "maximum": 16, "description": "1 = main display (default), 2… = other displays"}})},
		{"name": "click", "description": "Click at a point given in pixels of the most recent screenshot.",
			"inputSchema": obj(map[string]any{"x": num, "y": num,
				"button": map[string]any{"type": "string", "enum": []string{"left", "right"}},
				"count":  map[string]any{"type": "integer", "minimum": 1, "maximum": 3, "description": "2 = double click"}}, "x", "y")},
		{"name": "move_mouse", "description": "Move the pointer to a point in screenshot pixels (for hover menus).",
			"inputSchema": obj(map[string]any{"x": num, "y": num}, "x", "y")},
		{"name": "scroll", "description": "Scroll at a point in screenshot pixels. direction up/down/left/right, amount 1-20 lines.",
			"inputSchema": obj(map[string]any{"x": num, "y": num,
				"direction": map[string]any{"type": "string", "enum": []string{"up", "down", "left", "right"}},
				"amount":    map[string]any{"type": "integer", "minimum": 1, "maximum": 20}}, "x", "y", "direction")},
		{"name": "type_text", "description": "Type text into the focused field. Never type passwords, card numbers or other secrets.",
			"inputSchema": obj(map[string]any{"text": map[string]any{"type": "string", "maxLength": 2000}}, "text")},
		{"name": "press_key", "description": "Press a key or shortcut, " + KeyHelp + ".",
			"inputSchema": obj(map[string]any{"key": map[string]any{"type": "string"}}, "key")},
		{"name": "wait", "description": "Wait for the screen to update (0.1-10 seconds).",
			"inputSchema": obj(map[string]any{"seconds": num}, "seconds")},
	}
}

// Serve handles requests until r is closed.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 8<<20)
	enc := json.NewEncoder(w)
	for sc.Scan() {
		var req rpcReq
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			_ = enc.Encode(rpcResp{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcErr{-32700, "parse error"}})
			continue
		}
		if len(req.ID) == 0 { // notification
			continue
		}
		resp := rpcResp{JSONRPC: "2.0", ID: req.ID}
		switch req.Method {
		case "initialize":
			var p struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			_ = json.Unmarshal(req.Params, &p)
			if p.ProtocolVersion == "" {
				p.ProtocolVersion = "2025-06-18"
			}
			resp.Result = map[string]any{"protocolVersion": p.ProtocolVersion,
				"capabilities": map[string]any{"tools": map[string]any{"listChanged": false}},
				"serverInfo":   map[string]any{"name": "clawdyx-screen", "version": s.Version}}
		case "ping":
			resp.Result = map[string]any{}
		case "tools/list":
			resp.Result = map[string]any{"tools": Tools()}
		case "tools/call":
			var p struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			if err := json.Unmarshal(req.Params, &p); err != nil {
				resp.Error = &rpcErr{-32602, "invalid params"}
				break
			}
			resp.Result = s.call(ctx, p.Name, p.Arguments)
		default:
			resp.Error = &rpcErr{-32601, "method not found"}
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return sc.Err()
}

func fail(format string, a ...any) toolResult {
	return toolResult{IsError: true, Content: []content{{Type: "text", Text: fmt.Sprintf(format, a...)}}}
}

func okText(t string) toolResult { return toolResult{Content: []content{{Type: "text", Text: t}}} }

// toScreen converts screenshot pixels to screen points, validating bounds.
func (s *Server) toScreen(x, y float64) (float64, float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last == nil {
		return 0, 0, fmt.Errorf("take a screenshot first; coordinates are pixels of the latest screenshot")
	}
	if math.IsNaN(x) || math.IsNaN(y) || x < 0 || y < 0 || x >= float64(s.last.Width) || y >= float64(s.last.Height) {
		return 0, 0, fmt.Errorf("point (%.0f, %.0f) is outside the %dx%d screenshot", x, y, s.last.Width, s.last.Height)
	}
	return s.last.OriginX + x/s.last.Scale, s.last.OriginY + y/s.last.Scale, nil
}

func (s *Server) call(ctx context.Context, name string, raw json.RawMessage) toolResult {
	var a struct {
		X, Y      *float64
		Button    string
		Count     int
		Direction string
		Amount    int
		Text      *string
		Key       string
		Seconds   float64
		Display   int
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &a); err != nil {
			return fail("invalid arguments: %v", err)
		}
	}
	point := func() (float64, float64, error) {
		if a.X == nil || a.Y == nil {
			return 0, 0, fmt.Errorf("x and y are required")
		}
		return s.toScreen(*a.X, *a.Y)
	}
	switch name {
	case "list_displays":
		if s.Displays == nil {
			return fail("the display list is not available")
		}
		ds, err := s.Displays(ctx)
		if err != nil {
			return fail("could not list displays: %v", err)
		}
		if len(ds) == 0 {
			return fail("no display is connected")
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%d display(s). Pass the number as display to screenshot.\n", len(ds))
		for _, d := range ds {
			b.WriteString(d.Label() + "\n")
		}
		return okText(strings.TrimSpace(b.String()))
	case "screenshot":
		sel := ""
		if a.Display > 0 {
			sel = strconv.Itoa(a.Display)
		}
		if s.Authorize != nil {
			if err := s.Authorize(ctx, sel); err != nil {
				return fail("screenshot not allowed: %v", err)
			}
		}
		shot, err := s.D.Screenshot(ctx, sel)
		if err != nil {
			return fail("screenshot failed: %v", err)
		}
		s.mu.Lock()
		s.last = &shot
		s.mu.Unlock()
		return toolResult{Content: []content{
			{Type: "image", Data: base64.StdEncoding.EncodeToString(shot.JPEG), MimeType: "image/jpeg"},
			{Type: "text", Text: fmt.Sprintf("Screenshot of display %s, %dx%d pixels (display %.0fx%.0f points). Use these pixel coordinates with click, scroll and move_mouse; they act on this display.", displayName(shot.Display), shot.Width, shot.Height, shot.ScreenW, shot.ScreenH)},
		}}
	case "click":
		x, y, err := point()
		if err != nil {
			return fail("%v", err)
		}
		btn := a.Button
		if btn == "" {
			btn = "left"
		}
		if btn != "left" && btn != "right" {
			return fail("button must be left or right")
		}
		n := a.Count
		if n == 0 {
			n = 1
		}
		if n < 1 || n > 3 {
			return fail("count must be 1-3")
		}
		if err := s.D.Click(ctx, x, y, btn, n); err != nil {
			return fail("click failed: %v", err)
		}
		return okText(fmt.Sprintf("Clicked %s ×%d at (%.0f, %.0f). Take a screenshot to see the result.", btn, n, *a.X, *a.Y))
	case "move_mouse":
		x, y, err := point()
		if err != nil {
			return fail("%v", err)
		}
		if err := s.D.Move(ctx, x, y); err != nil {
			return fail("move failed: %v", err)
		}
		return okText("Moved.")
	case "scroll":
		x, y, err := point()
		if err != nil {
			return fail("%v", err)
		}
		amt := a.Amount
		if amt == 0 {
			amt = 5
		}
		if amt < 1 || amt > 20 {
			return fail("amount must be 1-20")
		}
		var dx, dy int
		switch a.Direction {
		case "up":
			dy = amt
		case "down":
			dy = -amt
		case "left":
			dx = amt
		case "right":
			dx = -amt
		default:
			return fail("direction must be up, down, left or right")
		}
		if err := s.D.Scroll(ctx, x, y, dx, dy); err != nil {
			return fail("scroll failed: %v", err)
		}
		return okText("Scrolled.")
	case "type_text":
		if a.Text == nil || *a.Text == "" {
			return fail("text is required")
		}
		if len([]rune(*a.Text)) > 2000 {
			return fail("text is limited to 2000 characters")
		}
		if err := s.D.TypeText(ctx, *a.Text); err != nil {
			return fail("typing failed: %v", err)
		}
		return okText("Typed.")
	case "press_key":
		if _, _, err := SplitCombo(a.Key); err != nil {
			return fail("%v", err)
		}
		if err := s.D.PressKey(ctx, a.Key); err != nil {
			return fail("key press failed: %v", err)
		}
		return okText("Pressed " + a.Key + ".")
	case "wait":
		if a.Seconds < 0.1 || a.Seconds > 10 {
			return fail("seconds must be 0.1-10")
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Duration(a.Seconds * float64(time.Second))):
		}
		return okText("Waited.")
	}
	return fail("unknown tool %q", name)
}

func displayName(d Display) string {
	if d.Index == 0 {
		return "1 (main)"
	}
	n := strconv.Itoa(d.Index) + " (" + d.Name
	if d.Main {
		n += ", main"
	}
	return n + ")"
}
