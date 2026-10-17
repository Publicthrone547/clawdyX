package setup

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestDetectSendersAcksAndFilters(t *testing.T) {
	var mu sync.Mutex
	var offsets []float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var p map[string]any
		_ = json.Unmarshal(body, &p)
		mu.Lock()
		if o, ok := p["offset"].(float64); ok {
			offsets = append(offsets, o)
		}
		mu.Unlock()
		_, _ = w.Write([]byte(`{"ok":true,"result":[
			{"update_id":10,"message":{"from":{"id":111,"first_name":"Me"},"chat":{"id":111,"type":"private"}}},
			{"update_id":11,"message":{"from":{"id":111,"first_name":"Me"},"chat":{"id":111,"type":"private"}}},
			{"update_id":12,"message":{"from":{"id":222,"first_name":"Group"},"chat":{"id":-5,"type":"group"}}},
			{"update_id":13,"message":{"from":{"id":333,"is_bot":true,"first_name":"Bot"},"chat":{"id":333,"type":"private"}}}]}`))
	}))
	defer srv.Close()
	old := APIBase
	APIBase = srv.URL
	defer func() { APIBase = old }()
	s, n, err := DetectSenders(context.Background(), "123456789:"+strings.Repeat("A", 35))
	if err != nil || n != 4 || len(s) != 1 || s[0].ID != 111 {
		t.Fatalf("%v %d %+v", err, n, s)
	}
	if len(offsets) != 1 || offsets[0] != 14 {
		t.Fatalf("pending updates must be acknowledged with offset 14, got %v", offsets)
	}
}

func TestWorkspaceName(t *testing.T) {
	for in, want := range map[string]string{"/Users/me/projects/telegram_bot": "telegram_bot", "/Users/me/My Project!": "my-project", "/": "workspace"} {
		if got := WorkspaceName(in); got != want {
			t.Errorf("%q: %q want %q", in, got, want)
		}
	}
}

func TestWorkspaceOK(t *testing.T) {
	if WorkspaceOK(t.TempDir()) != nil {
		t.Fatal("temp dir rejected")
	}
	for _, bad := range []string{"", "relative", "/nonexistent/x", "/"} {
		if WorkspaceOK(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
