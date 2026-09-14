// Package mcpserver implements a read-only Model Context Protocol server for
// the AIOps platform.
//
// The server is a deliberately narrow gateway. It publishes a fixed catalogue
// of read-only diagnostic capabilities to a general-purpose AI agent (Claude
// Desktop, Cursor, or any other MCP client) and offers no way to point it
// anywhere else: a caller names a tool, never an HTTP path. Every argument is
// checked against the tool's declared shape and a strict character allowlist
// before it is substituted, and only arguments a tool declares are forwarded.
//
// This is the outward-facing half of the idea the AI investigator applies
// inward. There, the model may cite only evidence the server authorised; here,
// an external agent may call only capabilities the server published. Both
// answer the same question — how do you let a model help without letting it
// wander — and both answer it the same way, by making the permitted set small
// and explicit rather than by trusting the caller.
//
// Protocol scope: the implemented subset is exactly what a client needs to
// discover and invoke tools — initialize, notifications/initialized,
// tools/list and tools/call. Resources, prompts, sampling and subscriptions are
// not implemented and are refused with MethodNotFound. Treat that as a
// narrowing rather than a gap: an agent that cannot ask this server for
// anything outside the published read-only tools is the intended property.
//
// The server pins one protocol revision instead of echoing the caller's. A
// client that speaks a different revision is answered with the pinned one and
// either proceeds or disconnects, which is the negotiation the protocol
// defines; silently mirroring an unknown version would claim conformance this
// implementation has not been tested against.
package mcpserver

import "encoding/json"

const (
	jsonRPCVersion = "2.0"

	// ProtocolVersion is the MCP revision this server speaks.
	ProtocolVersion = "2025-06-18"

	// serverName identifies this server in the initialize result.
	serverName = "aiops-platform-readonly"

	// maxMessageBytes bounds one inbound JSON-RPC line.
	maxMessageBytes = 1 << 20
)

// JSON-RPC 2.0 error codes used by this server.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

// request is one inbound JSON-RPC message. A message with no id is a
// notification and must not be answered.
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// response is one outbound JSON-RPC message.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// toolDescriptor is the shape advertised by tools/list.
type toolDescriptor struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// callToolParams is the shape of a tools/call request.
type callToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// callToolResult is the shape of a tools/call response.
type callToolResult struct {
	Content []contentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
