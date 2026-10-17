package screen

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

type fakeDesk struct{ calls []string }

func (f *fakeDesk) Screenshot(ctx context.Context) (Shot, error) {
	f.calls = append(f.calls, "shot")
	return Shot{JPEG: []byte{0xff, 0xd8}, Width: 1000, Height: 500, ScreenW: 2000, ScreenH: 1000, Scale: 0.5}, nil
}
func (f *fakeDesk) Click(ctx context.Context, x, y float64, b string, n int) error {
	f.calls = append(f.calls, "click "+ff(x)+","+ff(y)+" "+b)
	return nil
}
func (f *fakeDesk) Move(ctx context.Context, x, y float64) error {
	f.calls = append(f.calls, "move")
	return nil
}
func (f *fakeDesk) Scroll(ctx context.Context, x, y float64, dx, dy int) error {
	f.calls = append(f.calls, "scroll")
	return nil
}
func (f *fakeDesk) TypeText(ctx context.Context, t string) error {
	f.calls = append(f.calls, "type "+t)
	return nil
}
func (f *fakeDesk) PressKey(ctx context.Context, c int, fl uint64) error {
	f.calls = append(f.calls, "key")
	return nil
}
func (f *fakeDesk) Trusted(ctx context.Context) bool { return true }

func session(t *testing.T, lines ...string) ([]map[string]any, *fakeDesk) {
	d := &fakeDesk{}
	s := &Server{D: d, Version: "t"}
	pr, pw := io.Pipe()
	go func() {
		_ = s.Serve(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"), pw)
		pw.Close()
	}()
	var out []map[string]any
	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<24)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("bad json %q", sc.Text())
		}
		out = append(out, m)
	}
	return out, d
}

func call(id int, name, args string) string {
	return `{"jsonrpc":"2.0","id":` + itoa(id) + `,"method":"tools/call","params":{"name":"` + name + `","arguments":` + args + `}}`
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }

func isErr(m map[string]any) bool {
	r, _ := m["result"].(map[string]any)
	e, _ := r["isError"].(bool)
	return e
}

func TestHandshakeAndTools(t *testing.T) {
	out, _ := session(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"c","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"nope"}`,
		`not json`,
	)
	if len(out) != 4 {
		t.Fatalf("responses %d: %v", len(out), out)
	}
	if out[0]["result"].(map[string]any)["protocolVersion"] != "2025-06-18" {
		t.Fatal("protocol version not echoed")
	}
	tools := out[1]["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 7 {
		t.Fatalf("tools %d", len(tools))
	}
	if out[2]["error"] == nil || out[3]["error"] == nil {
		t.Fatal("expected errors for unknown method and bad json")
	}
}

func TestCoordinatesMappedAndValidated(t *testing.T) {
	out, d := session(t,
		call(1, "click", `{"x":10,"y":10}`), // before any screenshot
		call(2, "screenshot", `{}`),
		call(3, "click", `{"x":100,"y":50,"button":"right"}`),
		call(4, "click", `{"x":1000,"y":10}`), // outside
		call(5, "click", `{"x":5,"y":5,"count":9}`),
		call(6, "type_text", `{"text":"hello"}`),
		call(7, "press_key", `{"key":"cmd+l"}`),
		call(8, "press_key", `{"key":"hyper+q"}`),
		call(9, "scroll", `{"x":1,"y":1,"direction":"sideways"}`),
		call(10, "wait", `{"seconds":60}`),
		call(11, "rm_rf", `{}`),
	)
	wantErr := []bool{true, false, false, true, true, false, false, true, true, true, true}
	for i, w := range wantErr {
		if isErr(out[i]) != w {
			t.Errorf("call %d isError=%v want %v: %v", i+1, isErr(out[i]), w, out[i])
		}
	}
	shot := out[1]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)
	if shot["type"] != "image" || shot["mimeType"] != "image/jpeg" {
		t.Fatalf("screenshot content %v", shot)
	}
	joined := strings.Join(d.calls, "|")
	if !strings.Contains(joined, "click 200.0,100.0 right") || !strings.Contains(joined, "type hello") {
		t.Fatalf("desktop calls %v (image pixels must be scaled to screen points)", d.calls)
	}
}

func TestParseKey(t *testing.T) {
	c, f, err := ParseKey("cmd+shift+t")
	if err != nil || c != 17 || f != 0x100000|0x20000 {
		t.Fatalf("%d %x %v", c, f, err)
	}
	if _, _, err := ParseKey("Return"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "cmd+", "super+a", "a+b+c+d+e+f"} {
		if _, _, err := ParseKey(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}
