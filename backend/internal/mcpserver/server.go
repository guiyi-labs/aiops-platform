package mcpserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// Backend is the narrow slice of the platform API the server needs.
//
// It has exactly one method, and that method is a GET. This is not an artifact
// of the current feature set — it is the read-only property made structural.
// There is no function here through which a tool could write, so no future tool
// can introduce a write without changing this interface, which is a change a
// reviewer will see in the diff.
type Backend interface {
	Get(ctx context.Context, path string, query map[string]string) ([]byte, error)
}

// Server answers MCP requests over a pair of streams.
type Server struct {
	backend Backend
	tools   []Tool
	byName  map[string]Tool
	version string
}

// New builds a server over the given backend and tool catalogue. version is
// reported in initialize and is expected to be the build version of the binary.
func New(backend Backend, tools []Tool, version string) *Server {
	byName := make(map[string]Tool, len(tools))
	for _, tool := range tools {
		byName[tool.Name] = tool
	}
	if version == "" {
		version = "dev"
	}
	return &Server{backend: backend, tools: tools, byName: byName, version: version}
}

// Serve reads newline-delimited JSON-RPC messages from in and writes responses
// to out until the input reaches EOF.
//
// It returns only on a transport error. A malformed message is answered with an
// error and the loop continues: one bad request from a client is not a reason
// to drop the session.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), maxMessageBytes)
	encoder := json.NewEncoder(out)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		reply := s.handle(ctx, line)
		if reply == nil {
			continue // a notification is answered with silence, by protocol
		}
		if err := encoder.Encode(reply); err != nil {
			return fmt.Errorf("write response: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read request: %w", err)
	}
	return nil
}

// handle dispatches one message, returning nil for notifications.
func (s *Server) handle(ctx context.Context, line []byte) *response {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return &response{
			JSONRPC: jsonRPCVersion,
			Error:   &rpcError{Code: codeParseError, Message: "message is not valid JSON"},
		}
	}
	if req.JSONRPC != jsonRPCVersion || req.Method == "" {
		return s.failure(req.ID, codeInvalidRequest, "not a JSON-RPC 2.0 request")
	}
	if len(req.ID) == 0 {
		// notifications/initialized and anything else id-less: no reply by
		// protocol, and an unknown notification is also dropped rather than
		// errored, because there is no id to report an error against.
		return nil
	}

	switch req.Method {
	case "initialize":
		return s.success(req.ID, s.initializeResult())
	case "tools/list":
		return s.success(req.ID, map[string]any{"tools": s.descriptors()})
	case "tools/call":
		return s.callTool(ctx, req)
	default:
		return s.failure(req.ID, codeMethodNotFound,
			fmt.Sprintf("method %q is not supported by this server", req.Method))
	}
}

// initializeResult advertises the server's capabilities.
func (s *Server) initializeResult() map[string]any {
	return map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities": map[string]any{
			// listChanged is false because the catalogue is compiled in: it
			// cannot change while a client is connected, so promising change
			// notifications would be a promise the client might wait on.
			"tools": map[string]any{"listChanged": false},
		},
		"serverInfo": map[string]any{"name": serverName, "version": s.version},
	}
}

// descriptors renders the catalogue for tools/list.
func (s *Server) descriptors() []toolDescriptor {
	out := make([]toolDescriptor, 0, len(s.tools))
	for _, tool := range s.tools {
		out = append(out, toolDescriptor{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.inputSchema(),
		})
	}
	return out
}

// callTool executes one tools/call.
//
// A failure to reach the backend is reported inside the tool result with
// isError set rather than as a JSON-RPC error: the call was well-formed and
// understood, it simply did not succeed. A malformed call, by contrast, is a
// protocol error. The distinction matters to a client deciding whether to retry
// or to fix its request.
func (s *Server) callTool(ctx context.Context, req request) *response {
	var params callToolParams
	if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
		return s.failure(req.ID, codeInvalidParams, "tools/call requires a tool name")
	}
	tool, exists := s.byName[params.Name]
	if !exists {
		return s.failure(req.ID, codeInvalidParams,
			fmt.Sprintf("unknown tool %q: this server publishes a closed catalogue", params.Name))
	}

	raw, err := decodeArguments(params.Arguments)
	if err != nil {
		return s.failure(req.ID, codeInvalidParams, err.Error())
	}
	accepted, err := tool.ValidateArguments(raw)
	if err != nil {
		return s.failure(req.ID, codeInvalidParams, err.Error())
	}
	path, err := tool.URLPath(accepted)
	if err != nil {
		return s.failure(req.ID, codeInternalError, err.Error())
	}
	query := make(map[string]string, len(tool.Query))
	for _, name := range tool.Query {
		if value, ok := accepted[name]; ok {
			query[name] = value
		}
	}

	body, err := s.backend.Get(ctx, path, query)
	if err != nil {
		return s.success(req.ID, callToolResult{
			IsError: true,
			Content: []contentBlock{{Type: "text", Text: err.Error()}},
		})
	}
	return s.success(req.ID, callToolResult{
		Content: []contentBlock{{Type: "text", Text: string(body)}},
	})
}

// decodeArguments flattens the arguments object into a string map. The
// advertised schema accepts strings only, so a non-string value is a malformed
// call rather than something to coerce.
func decodeArguments(raw json.RawMessage) (map[string]string, error) {
	out := map[string]string{}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return out, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(trimmed, &decoded); err != nil {
		return nil, fmt.Errorf("arguments must be an object")
	}
	for name, value := range decoded {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("argument %q must be a string", name)
		}
		out[name] = text
	}
	return out, nil
}

func (s *Server) success(id json.RawMessage, result any) *response {
	return &response{JSONRPC: jsonRPCVersion, ID: id, Result: result}
}

func (s *Server) failure(id json.RawMessage, code int, message string) *response {
	return &response{JSONRPC: jsonRPCVersion, ID: id, Error: &rpcError{Code: code, Message: message}}
}
