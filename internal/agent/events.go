// Package agent drives the locally installed Claude Code CLI as a child
// process using its documented print mode (`claude -p`) with stream-json
// output. Prompts are always passed on stdin, never on the command line.
package agent

import (
	"encoding/json"
	"strings"
)

// Kind classifies a stream-json event.
type Kind int

// Event kinds.
const (
	KindUnknown Kind = iota
	KindInit
	KindTextDelta
	KindAssistant
	KindToolResult
	KindDenied
	KindRateLimit
	KindStatus
	KindRetry
	KindResult
	KindHook
)

// Usage holds token counts.
type Usage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
}

// Add returns u+o.
func (u Usage) Add(o Usage) Usage {
	return Usage{u.InputTokens + o.InputTokens, u.OutputTokens + o.OutputTokens, u.CacheReadInputTokens + o.CacheReadInputTokens, u.CacheCreationInputTokens + o.CacheCreationInputTokens}
}

// ToolUse summarizes one tool call.
type ToolUse struct {
	ID    string
	Name  string
	Input string // compact one-line summary, not the full input
}

// Denial is a permission denial reported by the CLI.
type Denial struct {
	ToolName  string          `json:"tool_name"`
	ToolUseID string          `json:"tool_use_id"`
	ToolInput json.RawMessage `json:"tool_input"`
}

// Summary returns a short description of the denied call.
func (d Denial) Summary() string { return d.ToolName + " " + summarizeInput(d.ToolName, d.ToolInput) }

// Window is one rate-limit window (undocumented CLI field).
type Window struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    int64   `json:"resetsAt"`
}

// RateLimit is the observed rate_limit_event payload.
type RateLimit struct {
	Status        string            `json:"status"`
	RateLimitType string            `json:"rateLimitType"`
	ResetsAt      int64             `json:"resetsAt"`
	Windows       map[string]Window `json:"unifiedWindows"`
}

// ModelUsage is the per-model block of a result.
type ModelUsage struct {
	InputTokens   int64   `json:"inputTokens"`
	OutputTokens  int64   `json:"outputTokens"`
	CostUSD       float64 `json:"costUSD"`
	ContextWindow int64   `json:"contextWindow"`
}

// Result is the final result event.
type Result struct {
	Subtype           string                `json:"subtype"`
	IsError           bool                  `json:"is_error"`
	TerminalReason    string                `json:"terminal_reason"`
	Result            *string               `json:"result"`
	SessionID         string                `json:"session_id"`
	NumTurns          int                   `json:"num_turns"`
	DurationMS        int64                 `json:"duration_ms"`
	TotalCostUSD      float64               `json:"total_cost_usd"`
	Usage             Usage                 `json:"usage"`
	ModelUsage        map[string]ModelUsage `json:"modelUsage"`
	PermissionDenials []Denial              `json:"permission_denials"`
	APIErrorStatus    *int                  `json:"api_error_status"`
	Errors            []string              `json:"errors"`
}

// Text returns the result text or "".
func (r *Result) Text() string {
	if r == nil || r.Result == nil {
		return ""
	}
	return *r.Result
}

// ErrorText joins the error messages of an error result, or "".
func (r *Result) ErrorText() string {
	if r == nil {
		return ""
	}
	return strings.Join(r.Errors, "; ")
}

// Event is a parsed stream-json line.
type Event struct {
	Kind    Kind
	Type    string
	Subtype string

	// init
	SessionID      string
	Model          string
	PermissionMode string
	CLIVersion     string
	APIKeySource   string

	// text delta / assistant text
	Text string

	// assistant
	MessageID string
	Usage     Usage
	Tools     []ToolUse

	// status / retry
	Status string

	RateLimit *RateLimit
	Denial    *Denial
	Result    *Result

	// Sub reports a message that belongs to a subagent.
	Sub bool
}

type rawEvent struct {
	Type            string          `json:"type"`
	Subtype         string          `json:"subtype"`
	SessionID       string          `json:"session_id"`
	Model           string          `json:"model"`
	PermissionMode  string          `json:"permissionMode"`
	Version         string          `json:"claude_code_version"`
	APIKeySource    string          `json:"apiKeySource"`
	Status          string          `json:"status"`
	ParentToolUseID *string         `json:"parent_tool_use_id"`
	Message         json.RawMessage `json:"message"`
	Event           json.RawMessage `json:"event"`
	RateLimitInfo   *RateLimit      `json:"rate_limit_info"`
	Attempt         int             `json:"attempt"`
	Error           string          `json:"error"`
	ToolName        string          `json:"tool_name"`
	ToolUseID       string          `json:"tool_use_id"`
	ToolInput       json.RawMessage `json:"tool_input"`
}

type rawMessage struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Usage   Usage  `json:"usage"`
	Content []struct {
		Type  string          `json:"type"`
		Text  string          `json:"text"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content"`
}

type rawStreamEvent struct {
	Type  string `json:"type"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
}

// Parse decodes one stream-json line. Unknown shapes yield KindUnknown, never
// an error, so CLI additions do not break the bot.
func Parse(line []byte) (Event, error) {
	var r rawEvent
	if err := json.Unmarshal(line, &r); err != nil {
		return Event{}, err
	}
	ev := Event{Type: r.Type, Subtype: r.Subtype, SessionID: r.SessionID, Sub: r.ParentToolUseID != nil && *r.ParentToolUseID != ""}
	switch r.Type {
	case "system":
		switch r.Subtype {
		case "init":
			ev.Kind = KindInit
			ev.Model, ev.PermissionMode, ev.CLIVersion, ev.APIKeySource = r.Model, r.PermissionMode, r.Version, r.APIKeySource
		case "status":
			ev.Kind, ev.Status = KindStatus, r.Status
		case "api_retry":
			ev.Kind, ev.Status = KindRetry, r.Error
		case "permission_denied":
			ev.Kind = KindDenied
			ev.Denial = &Denial{ToolName: r.ToolName, ToolUseID: r.ToolUseID, ToolInput: r.ToolInput}
		case "hook_started", "hook_progress", "hook_response":
			ev.Kind = KindHook
		}
	case "stream_event":
		var se rawStreamEvent
		if json.Unmarshal(r.Event, &se) == nil && se.Type == "content_block_delta" && se.Delta.Type == "text_delta" {
			ev.Kind, ev.Text = KindTextDelta, se.Delta.Text
		}
	case "assistant":
		var m rawMessage
		if err := json.Unmarshal(r.Message, &m); err == nil {
			ev.Kind = KindAssistant
			ev.MessageID, ev.Model, ev.Usage = m.ID, m.Model, m.Usage
			var sb strings.Builder
			for _, c := range m.Content {
				switch c.Type {
				case "text":
					sb.WriteString(c.Text)
				case "tool_use":
					ev.Tools = append(ev.Tools, ToolUse{ID: c.ID, Name: c.Name, Input: summarizeInput(c.Name, c.Input)})
				}
			}
			ev.Text = sb.String()
		}
	case "user":
		ev.Kind = KindToolResult
	case "rate_limit_event":
		ev.Kind, ev.RateLimit = KindRateLimit, r.RateLimitInfo
	case "result":
		var res Result
		if err := json.Unmarshal(line, &res); err != nil {
			return Event{}, err
		}
		ev.Kind, ev.Result = KindResult, &res
	}
	return ev, nil
}

// summarizeInput renders a compact, bounded description of a tool input.
func summarizeInput(tool string, raw json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	pick := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := m[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}
	var s string
	switch tool {
	case "Bash":
		s = pick("command")
	case "Read", "Write", "Edit", "NotebookEdit":
		s = pick("file_path", "notebook_path")
	case "Grep", "Glob":
		s = pick("pattern")
	case "WebFetch":
		s = pick("url")
	case "Agent", "Task":
		s = pick("description")
	default:
		s = pick("command", "file_path", "path", "pattern", "description", "query")
	}
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 120 {
		s = string(r[:119]) + "…"
	}
	return s
}
