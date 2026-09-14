package mcpserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// fakeBackend records what the server asked for, so the tests can assert on
// the request rather than only on the reply.
type fakeBackend struct {
	gotPath  string
	gotQuery map[string]string
	calls    int
	body     []byte
	err      error
}

func (f *fakeBackend) Get(_ context.Context, path string, query map[string]string) ([]byte, error) {
	f.calls++
	f.gotPath = path
	f.gotQuery = query
	if f.err != nil {
		return nil, f.err
	}
	return f.body, nil
}

// exchange runs one session over the given newline-delimited messages and
// returns the decoded replies in order.
func exchange(t *testing.T, backend Backend, messages ...string) []map[string]any {
	t.Helper()
	in := strings.NewReader(strings.Join(messages, "\n") + "\n")
	var out bytes.Buffer
	if err := New(backend, DefaultTools(), "test-version").Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	replies := make([]map[string]any, 0, len(messages))
	scanner := bufio.NewScanner(&out)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("reply is not JSON: %q", line)
		}
		replies = append(replies, decoded)
	}
	return replies
}

func errorMessage(t *testing.T, reply map[string]any) string {
	t.Helper()
	raw, ok := reply["error"].(map[string]any)
	if !ok {
		t.Fatalf("reply carries no error: %v", reply)
	}
	message, ok := raw["message"].(string)
	if !ok {
		t.Fatalf("error carries no message: %v", raw)
	}
	return message
}

func TestInitializeAdvertisesPinnedRevisionAndToolsCapability(t *testing.T) {
	replies := exchange(t, &fakeBackend{}, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"test"}}}`)
	if len(replies) != 1 {
		t.Fatalf("got %d replies, want 1", len(replies))
	}
	result, ok := replies[0]["result"].(map[string]any)
	if !ok {
		t.Fatalf("initialize returned no result: %v", replies[0])
	}
	if got := result["protocolVersion"]; got != ProtocolVersion {
		t.Fatalf("protocolVersion = %v, want %q", got, ProtocolVersion)
	}
	capabilities, ok := result["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("capabilities missing: %v", result)
	}
	if _, ok := capabilities["tools"]; !ok {
		t.Fatalf("tools capability missing: %v", capabilities)
	}
	info, ok := result["serverInfo"].(map[string]any)
	if !ok {
		t.Fatalf("serverInfo missing: %v", result)
	}
	if info["version"] != "test-version" {
		t.Fatalf("serverInfo.version = %v, want test-version", info["version"])
	}
}

func TestToolsListPublishesClosedCatalogue(t *testing.T) {
	replies := exchange(t, &fakeBackend{}, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if len(replies) != 1 {
		t.Fatalf("got %d replies, want 1", len(replies))
	}
	result, _ := replies[0]["result"].(map[string]any)
	tools, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list returned no tools array: %v", replies[0])
	}
	if len(tools) != len(DefaultTools()) {
		t.Fatalf("advertised %d tools, catalogue has %d", len(tools), len(DefaultTools()))
	}
	first, _ := tools[0].(map[string]any)
	if first["name"] == "" || first["description"] == "" {
		t.Fatalf("tool descriptor incomplete: %v", first)
	}
	schema, ok := first["inputSchema"].(map[string]any)
	if !ok {
		t.Fatalf("inputSchema missing: %v", first)
	}
	if schema["additionalProperties"] != false {
		t.Fatalf("schema must refuse undeclared properties, got %v", schema["additionalProperties"])
	}
}

// The catalogue is the whole attack surface, so it gets asserted on directly.
func TestCatalogueIsGETOnlyAndUniquelyNamed(t *testing.T) {
	seen := map[string]struct{}{}
	for _, tool := range DefaultTools() {
		if _, duplicate := seen[tool.Name]; duplicate {
			t.Fatalf("duplicate tool name %q", tool.Name)
		}
		seen[tool.Name] = struct{}{}
		if !strings.HasPrefix(tool.Path, "/api/v1/") {
			t.Fatalf("tool %q path %q is outside the platform API", tool.Name, tool.Path)
		}
		if tool.Description == "" {
			t.Fatalf("tool %q has no description", tool.Name)
		}
		for _, name := range tool.Query {
			declared := false
			for _, a := range tool.Args {
				if a.Name == name {
					declared = true
					break
				}
			}
			if !declared {
				t.Fatalf("tool %q forwards undeclared query parameter %q", tool.Name, name)
			}
		}
	}
}

func TestToolsCallForwardsOnlyDeclaredArguments(t *testing.T) {
	backend := &fakeBackend{body: []byte(`{"items":[]}`)}
	replies := exchange(t, backend,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_diagnoses","arguments":{"limit":"5","status":"open"}}}`)
	if len(replies) != 1 {
		t.Fatalf("got %d replies, want 1", len(replies))
	}
	if _, hasError := replies[0]["error"]; hasError {
		t.Fatalf("call rejected: %v", replies[0])
	}
	if backend.gotPath != "/api/v1/diagnoses" {
		t.Fatalf("backend path = %q", backend.gotPath)
	}
	if backend.gotQuery["limit"] != "5" || backend.gotQuery["status"] != "open" {
		t.Fatalf("backend query = %v, want limit/status forwarded", backend.gotQuery)
	}
	if _, leaked := backend.gotQuery["cluster_id"]; leaked {
		t.Fatalf("unset argument was forwarded: %v", backend.gotQuery)
	}

	result, _ := replies[0]["result"].(map[string]any)
	if result["isError"] == true {
		t.Fatalf("successful call marked as error: %v", result)
	}
	content, _ := result["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("content = %v, want one block", result["content"])
	}
	block, _ := content[0].(map[string]any)
	if block["text"] != `{"items":[]}` {
		t.Fatalf("content text = %v, want the backend body verbatim", block["text"])
	}
}

func TestToolsCallRejectsUnknownTool(t *testing.T) {
	backend := &fakeBackend{}
	replies := exchange(t, backend,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"delete_everything","arguments":{}}}`)
	if len(replies) != 1 {
		t.Fatalf("got %d replies, want 1", len(replies))
	}
	if !strings.Contains(errorMessage(t, replies[0]), "unknown tool") {
		t.Fatalf("unexpected error: %v", replies[0])
	}
	if backend.calls != 0 {
		t.Fatal("an unknown tool must never reach the backend")
	}
}

func TestToolsCallRejectsUndeclaredArgument(t *testing.T) {
	backend := &fakeBackend{}
	replies := exchange(t, backend,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"get_diagnosis","arguments":{"diagnosis_id":"7","verb":"DELETE"}}}`)
	if !strings.Contains(errorMessage(t, replies[0]), "unknown argument") {
		t.Fatalf("undeclared argument was not rejected: %v", replies[0])
	}
	if backend.calls != 0 {
		t.Fatal("a rejected call must never reach the backend")
	}
}

// Path arguments are substituted into the URL, so traversal has to be refused
// before substitution rather than sanitised afterwards.
func TestToolsCallRejectsPathTraversalAndInjection(t *testing.T) {
	hostile := []string{
		`../../api/v1/users`,
		`7/../../admin`,
		`7?admin=1`,
		`7#fragment`,
		`7 8`,
		`7%2e%2e`,
		`.hidden`,
		`a/b`,
	}
	for _, value := range hostile {
		t.Run(value, func(t *testing.T) {
			backend := &fakeBackend{}
			payload := `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"get_diagnosis","arguments":{"diagnosis_id":` + jsonString(value) + `}}}`
			replies := exchange(t, backend, payload)
			if len(replies) != 1 {
				t.Fatalf("got %d replies, want 1", len(replies))
			}
			if _, hasError := replies[0]["error"]; !hasError {
				t.Fatalf("hostile value %q was accepted: %v", value, replies[0])
			}
			if backend.calls != 0 {
				t.Fatalf("hostile value %q reached the backend", value)
			}
		})
	}
}

func TestToolsCallRejectsMissingRequiredArgument(t *testing.T) {
	backend := &fakeBackend{}
	replies := exchange(t, backend,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"get_diagnosis","arguments":{}}}`)
	if !strings.Contains(errorMessage(t, replies[0]), "missing required argument") {
		t.Fatalf("unexpected error: %v", replies[0])
	}
	if backend.calls != 0 {
		t.Fatal("a call missing a required argument must never reach the backend")
	}
}

func TestToolsCallRejectsNonStringArgument(t *testing.T) {
	backend := &fakeBackend{}
	replies := exchange(t, backend,
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"get_diagnosis","arguments":{"diagnosis_id":7}}}`)
	if !strings.Contains(errorMessage(t, replies[0]), "must be a string") {
		t.Fatalf("numeric argument was not rejected: %v", replies[0])
	}
	if backend.calls != 0 {
		t.Fatal("a malformed call must never reach the backend")
	}
}

// A backend failure is the tool's problem, not the protocol's: the call was
// understood, so the client should see isError rather than a JSON-RPC error it
// might treat as a bug in itself.
func TestBackendFailureIsReportedInsideToolResult(t *testing.T) {
	backend := &fakeBackend{err: errors.New("platform API returned HTTP 401")}
	replies := exchange(t, backend,
		`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"list_diagnoses","arguments":{}}}`)
	if len(replies) != 1 {
		t.Fatalf("got %d replies, want 1", len(replies))
	}
	if _, hasError := replies[0]["error"]; hasError {
		t.Fatalf("backend failure must not be a protocol error: %v", replies[0])
	}
	result, _ := replies[0]["result"].(map[string]any)
	if result["isError"] != true {
		t.Fatalf("isError not set: %v", result)
	}
	content, _ := result["content"].([]any)
	block, _ := content[0].(map[string]any)
	if !strings.Contains(block["text"].(string), "401") {
		t.Fatalf("backend error text lost: %v", block)
	}
}

// Notifications carry no id and the protocol forbids replying to them; a
// reply here would desynchronise a client that is counting responses.
func TestNotificationIsSilent(t *testing.T) {
	replies := exchange(t, &fakeBackend{}, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if len(replies) != 0 {
		t.Fatalf("notification produced %d replies: %v", len(replies), replies)
	}
}

func TestUnknownMethodIsRefused(t *testing.T) {
	replies := exchange(t, &fakeBackend{}, `{"jsonrpc":"2.0","id":10,"method":"resources/list"}`)
	if len(replies) != 1 {
		t.Fatalf("got %d replies, want 1", len(replies))
	}
	if !strings.Contains(errorMessage(t, replies[0]), "not supported") {
		t.Fatalf("unexpected error: %v", replies[0])
	}
	raw, _ := replies[0]["error"].(map[string]any)
	if raw["code"] != float64(codeMethodNotFound) {
		t.Fatalf("error code = %v, want %d", raw["code"], codeMethodNotFound)
	}
}

func TestMalformedMessagesAreReportedWithoutEndingTheSession(t *testing.T) {
	replies := exchange(t, &fakeBackend{body: []byte(`{}`)},
		`this is not json`,
		`{"jsonrpc":"1.0","id":11,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":12,"method":"tools/list"}`,
	)
	if len(replies) != 3 {
		t.Fatalf("got %d replies, want 3 (the session must survive bad input)", len(replies))
	}
	if !strings.Contains(errorMessage(t, replies[0]), "not valid JSON") {
		t.Fatalf("first reply: %v", replies[0])
	}
	if !strings.Contains(errorMessage(t, replies[1]), "JSON-RPC") {
		t.Fatalf("second reply: %v", replies[1])
	}
	if _, hasError := replies[2]["error"]; hasError {
		t.Fatalf("a valid request after bad input must still succeed: %v", replies[2])
	}
}

func TestBlankLinesAreIgnored(t *testing.T) {
	replies := exchange(t, &fakeBackend{}, ``, `   `, `{"jsonrpc":"2.0","id":13,"method":"tools/list"}`)
	if len(replies) != 1 {
		t.Fatalf("got %d replies, want 1", len(replies))
	}
}

func jsonString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}
