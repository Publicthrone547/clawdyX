// Package control is the worker's local control channel: JSON-RPC 2.0, one
// JSON object per line, over a Unix domain socket that only the worker's own
// macOS user may use. The desktop app and `clawdyx ctl` are its clients.
//
// Security: the socket lives in the private state directory (0700) with mode
// 0600, and every connection's peer uid (LOCAL_PEERCRED) must equal the
// worker's uid. Methods are typed and validated; none accepts shell input.
package control

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Frame limits.
const (
	MaxFrame       = 1 << 20  // requests and ordinary responses
	MaxLargeFrame  = 12 << 20 // responses that carry images (base64)
	ProtocolSchema = 1
)

// JSON-RPC error codes.
const (
	CodeParse          = -32700
	CodeInvalidRequest = -32600
	CodeNoMethod       = -32601
	CodeInvalidParams  = -32602
	CodeApp            = -32000
	CodeTimeout        = -32001
	CodeForbidden      = -32003
	CodeUnavailable    = -32004
)

// Request is a JSON-RPC request or notification.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC response; Method/Params are set for server events.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Error is a JSON-RPC error object.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return fmt.Sprintf("%s (code %d)", e.Message, e.Code) }

// Errorf builds an application error that is shown to the user as is.
func Errorf(format string, a ...any) error {
	return &Error{Code: CodeApp, Message: fmt.Sprintf(format, a...)}
}

// InvalidParams builds an invalid-params error.
func InvalidParams(format string, a ...any) error {
	return &Error{Code: CodeInvalidParams, Message: fmt.Sprintf(format, a...)}
}

// Event is a server-to-client notification ("event" method).
type Event struct {
	Type string          `json:"type"`
	Seq  uint64          `json:"seq"`
	TS   int64           `json:"ts"` // unix milliseconds
	Data json.RawMessage `json:"data,omitempty"`
}

func asError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{Code: CodeApp, Message: err.Error()}
}
