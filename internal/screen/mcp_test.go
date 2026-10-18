package screen

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

type fakeDesk struct{ calls []string }

func (f *fakeDesk) Screenshot(ctx context.Context, display string) (Shot, error) {
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
func (f *fakeDesk) PressKey(ctx context.Context, combo string) error {
	f.calls = append(f.calls, "key "+combo)
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
	if len(tools) != 8 {
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
		call(7, "press_key", `{"key":"shift+tab"}`),
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

type offsetDesk struct{ fakeDesk }

func (f *offsetDesk) Screenshot(ctx context.Context, display string) (Shot, error) {
	f.calls = append(f.calls, "shot "+display)
	return Shot{JPEG: []byte{0xff, 0xd8}, Width: 960, Height: 540, ScreenW: 1920, ScreenH: 1080, Scale: 0.5, OriginX: -1920, OriginY: -1080, Display: Display{Index: 2, Name: "Side"}}, nil
}

func TestScreenshotOnSecondDisplayMapsGlobalPoints(t *testing.T) {
	d := &offsetDesk{}
	var asked []string
	s := &Server{D: d, Version: "t", Authorize: func(_ context.Context, disp string) error { asked = append(asked, disp); return nil }}
	ctx := context.Background()
	if r := s.call(ctx, "screenshot", []byte(`{"display":2}`)); r.IsError {
		t.Fatalf("%+v", r)
	}
	if r := s.call(ctx, "click", []byte(`{"x":100,"y":50}`)); r.IsError {
		t.Fatalf("%+v", r)
	}
	if d.calls[0] != "shot 2" || d.calls[1] != "click -1720.0,-980.0 left" {
		t.Fatalf("calls %v", d.calls)
	}
	if len(asked) != 1 || asked[0] != "2" {
		t.Fatalf("authorize %v", asked)
	}
	s.Authorize = func(context.Context, string) error { return ErrScreenOff }
	if r := s.call(ctx, "screenshot", nil); !r.IsError {
		t.Fatal("screenshot must fail when the worker says no")
	}
}

func TestListDisplays(t *testing.T) {
	s := &Server{D: &fakeDesk{}}
	if r := s.call(context.Background(), "list_displays", nil); !r.IsError {
		t.Fatal("list_displays without a display source must fail")
	}
	s.Displays = func(context.Context) ([]Display, error) {
		return []Display{{Index: 1, Name: "Built-in", PixelW: 3024, PixelH: 1964, Main: true}, {Index: 2, Name: "BenQ", PixelW: 1920, PixelH: 1080}}, nil
	}
	r := s.call(context.Background(), "list_displays", nil)
	if r.IsError || len(r.Content) != 1 {
		t.Fatalf("list_displays: %+v", r)
	}
	txt := r.Content[0].Text
	for _, want := range []string{"2 display(s)", "1 · Built-in · 3024×1964 · main", "2 · BenQ · 1920×1080"} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in %q", want, txt)
		}
	}
}

func TestSplitCombo(t *testing.T) {
	mods, key, err := SplitCombo("Shift+Tab")
	if err != nil || len(mods) != 1 || mods[0] != "shift" || key != "tab" {
		t.Fatalf("%v %q %v", mods, key, err)
	}
	for _, bad := range []string{"", "shift+", "hyper+q", "a+b+c+d+e+f", "shift+notakey"} {
		if _, _, err := SplitCombo(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

// waylandDesk: screenshots work, input refuses with its own text.
type waylandDesk struct{ fakeDesk }

var errNoInput = errors.New("mouse and keyboard tools are not available on Wayland yet")

func (w *waylandDesk) Click(context.Context, float64, float64, string, int) error { return errNoInput }
func (w *waylandDesk) Trusted(context.Context) bool                               { return false }
func (w *waylandDesk) InputError() error                                          { return errNoInput }

func TestInputUnavailableIsReported(t *testing.T) {
	d := &waylandDesk{}
	s := &Server{D: d, Session: "wayland", Displays: func(context.Context) ([]Display, error) {
		return []Display{{Index: 1, Name: "Wayland screen", PixelW: 1920, PixelH: 1080, Main: true}}, nil
	}}
	ctx := context.Background()
	txt := s.call(ctx, "list_displays", nil).Content[0].Text
	for _, want := range []string{"Session: wayland.", "Mouse and keyboard tools: not available (" + errNoInput.Error() + ")"} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in %q", want, txt)
		}
	}
	if r := s.call(ctx, "screenshot", nil); r.IsError {
		t.Fatalf("screenshot: %+v", r)
	}
	r := s.call(ctx, "click", json.RawMessage(`{"x":10,"y":10}`))
	if !r.IsError || !strings.Contains(r.Content[0].Text, errNoInput.Error()) {
		t.Fatalf("click: %+v", r)
	}
	ok := &Server{D: &fakeDesk{}, Displays: s.Displays}
	if txt := ok.call(ctx, "list_displays", nil).Content[0].Text; !strings.Contains(txt, "Mouse and keyboard tools: available.") || strings.Contains(txt, "Session:") {
		t.Errorf("trusted desktop: %q", txt)
	}
}
