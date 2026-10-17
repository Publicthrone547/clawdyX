// fakeclaude imitates the stream-json behaviour of `claude -p` (2.1.292) for
// tests. Behaviour is selected by keywords in the stdin prompt.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func emit(v any) {
	b, _ := json.Marshal(v)
	os.Stdout.Write(append(b, '\n'))
}

func arg(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func main() {
	args := os.Args[1:]
	if len(args) == 1 && args[0] == "--version" {
		fmt.Println("2.1.292 (Claude Code)")
		return
	}
	if len(args) >= 2 && args[0] == "auth" && args[1] == "status" {
		fmt.Println(`{"loggedIn":true,"authMethod":"claude.ai","apiProvider":"firstParty","email":"x@example.com","orgId":"o","subscriptionType":"max"}`)
		return
	}
	if len(args) >= 2 && args[0] == "agents" && args[1] == "--json" {
		fmt.Println(`[{"pid":1,"cwd":"/tmp","kind":"interactive","startedAt":1,"sessionId":"s","name":"n","status":"idle"}]`)
		return
	}
	if b, err := json.Marshal(args); err == nil {
		_ = os.WriteFile(".fakeclaude-args", b, 0o600)
	}
	in, _ := io.ReadAll(os.Stdin)
	prompt := string(in)
	_ = os.WriteFile(".fakeclaude-env", []byte(strings.Join(os.Environ(), "\n")), 0o600)

	sid := arg(args, "--session-id")
	if r := arg(args, "--resume"); r != "" {
		sid = r
		if strings.HasPrefix(r, "00000000") {
			fmt.Fprintf(os.Stderr, "No conversation found with session ID: %s\n", r)
			os.Exit(1)
		}
	}
	model := arg(args, "--model")
	canonical := map[string]string{"": "claude-fable-5-1", "fable": "claude-fable-5-1", "sonnet": "claude-sonnet-5-5", "opus": "claude-opus-5-5", "haiku": "claude-haiku-4-5-20251001"}[model]
	if eff := arg(args, "--effort"); eff != "" && !strings.Contains(" low medium high xhigh max ", " "+eff+" ") {
		fmt.Fprintf(os.Stderr, "Warning: Unknown --effort value '%s' — ignoring it and using the default effort. Valid values: low, medium, high, xhigh, max.\n", eff)
	}
	if strings.Contains(prompt, "OK") && arg(args, "--output-format") == "json" {
		if canonical == "" {
			fmt.Fprintf(os.Stderr, "[claude-code:unrecognized_model] {\"model\":%q}\n", model)
			fmt.Printf(`{"type":"result","subtype":"success","is_error":true,"api_error_status":404,"terminal_reason":"api_error","result":"There's an issue with the selected model","session_id":"x","modelUsage":{}}` + "\n")
			return
		}
		fmt.Printf(`{"type":"result","subtype":"success","is_error":false,"terminal_reason":"completed","result":"OK","session_id":"x","modelUsage":{%q:{"inputTokens":2,"outputTokens":1}}}`+"\n", canonical)
		return
	}
	if canonical == "" {
		canonical = model
	}

	emit(map[string]any{"type": "system", "subtype": "hook_started", "session_id": sid})
	emit(map[string]any{"type": "system", "subtype": "init", "session_id": sid, "model": canonical, "permissionMode": arg(args, "--permission-mode"), "claude_code_version": "2.1.292", "apiKeySource": "none", "cwd": "."})

	delta := func(t string) {
		emit(map[string]any{"type": "stream_event", "session_id": sid, "parent_tool_use_id": nil, "event": map[string]any{"type": "content_block_delta", "delta": map[string]any{"type": "text_delta", "text": t}}})
	}
	result := func(subtype string, isErr bool, reason, text string, denials []any) {
		var res any = text
		if text == "" && isErr {
			res = nil
		}
		emit(map[string]any{"type": "result", "subtype": subtype, "is_error": isErr, "terminal_reason": reason, "result": res, "session_id": sid,
			"num_turns": 1, "duration_ms": 10, "total_cost_usd": 0.01,
			"usage":              map[string]any{"input_tokens": 99, "output_tokens": 99},
			"modelUsage":         map[string]any{canonical: map[string]any{"inputTokens": 99, "outputTokens": 99}},
			"permission_denials": denials})
	}

	switch {
	case strings.Contains(prompt, "SLOW") || strings.Contains(prompt, "IGNORE_INT") || strings.Contains(prompt, "SPAWN"):
		sigs := make(chan os.Signal, 2)
		if strings.Contains(prompt, "IGNORE_INT") {
			signal.Ignore(syscall.SIGINT, syscall.SIGTERM)
		} else {
			signal.Notify(sigs, syscall.SIGINT)
		}
		if strings.Contains(prompt, "SPAWN") {
			c := exec.Command("/bin/sleep", "300")
			_ = c.Start()
			_ = os.WriteFile(".fakeclaude-child", []byte(fmt.Sprint(c.Process.Pid)), 0o600)
		}
		tick := time.NewTicker(50 * time.Millisecond)
		deadline := time.After(30 * time.Second)
		for {
			select {
			case <-sigs:
				result("error_during_execution", true, "aborted_streaming", "", nil)
				os.Exit(0)
			case <-tick.C:
				delta("tick ")
			case <-deadline:
				result("success", false, "completed", "slow done", nil)
				return
			}
		}
	case strings.Contains(prompt, "DENY"):
		emit(map[string]any{"type": "assistant", "session_id": sid, "message": map[string]any{"id": "m1", "model": canonical, "usage": map[string]any{"input_tokens": 3, "output_tokens": 2},
			"content": []any{map[string]any{"type": "tool_use", "id": "t1", "name": "Bash", "input": map[string]any{"command": "touch x.txt"}}}}})
		emit(map[string]any{"type": "system", "subtype": "permission_denied", "session_id": sid, "tool_name": "Bash", "tool_use_id": "t1", "tool_input": map[string]any{"command": "touch x.txt"}})
		delta("DENIED")
		result("success", false, "completed", "DENIED", []any{map[string]any{"tool_name": "Bash", "tool_use_id": "t1", "tool_input": map[string]any{"command": "touch x.txt"}}})
	case strings.Contains(prompt, "FAIL"):
		result("success", true, "api_error", "API Error: overloaded", nil)
	case strings.Contains(prompt, "GARBAGE"):
		os.Stdout.WriteString("{not json\n")
		delta("after garbage")
		result("success", false, "completed", "after garbage", nil)
	case strings.Contains(prompt, "BIG"):
		chunk := strings.Repeat("x", 1000)
		for i := 0; i < 50; i++ {
			delta(chunk)
		}
		result("success", false, "completed", strings.Repeat(chunk, 50), nil)
	default:
		delta("Hello ")
		delta("world")
		usage := map[string]any{"input_tokens": 5, "output_tokens": 7, "cache_read_input_tokens": 100, "cache_creation_input_tokens": 10}
		// Two assistant events for the same message (one per content block), like the real CLI.
		emit(map[string]any{"type": "assistant", "session_id": sid, "message": map[string]any{"id": "m1", "model": canonical, "usage": usage,
			"content": []any{map[string]any{"type": "tool_use", "id": "t9", "name": "Read", "input": map[string]any{"file_path": "/w/main.go"}}}}})
		emit(map[string]any{"type": "assistant", "session_id": sid, "message": map[string]any{"id": "m1", "model": canonical, "usage": usage,
			"content": []any{map[string]any{"type": "text", "text": "Hello world"}}}})
		emit(map[string]any{"type": "rate_limit_event", "session_id": sid, "rate_limit_info": map[string]any{"status": "allowed", "rateLimitType": "five_hour", "unifiedWindows": map[string]any{"five_hour": map[string]any{"utilization": 0.15, "resetsAt": 1792154400}}}})
		result("success", false, "completed", "Hello world", nil)
	}
}
