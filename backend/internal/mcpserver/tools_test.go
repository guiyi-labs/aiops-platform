package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestURLPathSubstitutesDeclaredPlaceholders(t *testing.T) {
	tool := Tool{
		Name: "get_thing",
		Path: "/api/v1/things/{thing_id}/parts/{part_id}",
		Args: []Arg{{Name: "thing_id", Required: true}, {Name: "part_id", Required: true}},
	}
	path, err := tool.URLPath(map[string]string{"thing_id": "7", "part_id": "9"})
	if err != nil {
		t.Fatalf("URLPath: %v", err)
	}
	if path != "/api/v1/things/7/parts/9" {
		t.Fatalf("path = %q", path)
	}
}

// A catalogue entry that references a placeholder it never declared, or a call
// that omits a declared path argument, must be reported — the alternative is a
// request with a literal brace in the URL reaching the platform.
func TestURLPathReportsUnresolvablePaths(t *testing.T) {
	undeclared := Tool{Name: "broken", Path: "/api/v1/{unknown}"}
	if _, err := undeclared.URLPath(map[string]string{}); err == nil {
		t.Fatal("an undeclared placeholder must be reported")
	}

	absent := Tool{Name: "partial", Path: "/api/v1/things/{thing_id}", Args: []Arg{{Name: "thing_id"}}}
	if _, err := absent.URLPath(map[string]string{}); err == nil {
		t.Fatal("a missing path argument must be reported")
	}
}

// Every published tool must accept the call its own schema invites; a tool that
// cannot be called is worse than one that is absent, because the catalogue
// advertises it.
func TestEveryToolAcceptsItsOwnRequiredArguments(t *testing.T) {
	for _, tool := range DefaultTools() {
		args := map[string]string{}
		for _, a := range tool.Args {
			if a.Required {
				args[a.Name] = "1"
			}
		}
		if _, err := tool.ValidateArguments(args); err != nil {
			t.Fatalf("tool %q rejected its own required arguments: %v", tool.Name, err)
		}
		if _, err := tool.URLPath(args); err != nil {
			t.Fatalf("tool %q cannot render its own path: %v", tool.Name, err)
		}
	}
}

func TestInputSchemaMarksRequiredArguments(t *testing.T) {
	tool := Tool{
		Name: "get_thing",
		Args: []Arg{{Name: "thing_id", Required: true}, {Name: "detail", Description: "optional flag"}},
	}
	var schema struct {
		Type       string                    `json:"type"`
		Properties map[string]map[string]any `json:"properties"`
		Required   []string                  `json:"required"`
	}
	if err := json.Unmarshal(tool.inputSchema(), &schema); err != nil {
		t.Fatalf("schema is not JSON: %v", err)
	}
	if schema.Type != "object" {
		t.Fatalf("schema type = %q", schema.Type)
	}
	if len(schema.Properties) != 2 {
		t.Fatalf("properties = %v, want both arguments", schema.Properties)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "thing_id" {
		t.Fatalf("required = %v, want [thing_id]", schema.Required)
	}
	if schema.Properties["detail"]["description"] != "optional flag" {
		t.Fatalf("description lost: %v", schema.Properties["detail"])
	}

	// A tool with no required arguments omits the key instead of advertising an
	// empty list, which some clients render as an unsatisfiable tool.
	bare := Tool{Name: "bare", Args: []Arg{{Name: "a"}}}
	if strings.Contains(string(bare.inputSchema()), "required") {
		t.Fatalf("schema should omit required when empty: %s", bare.inputSchema())
	}
}

// The schema is published to clients that cache it, so it has to be byte-stable.
func TestInputSchemaIsStableAcrossCalls(t *testing.T) {
	tool := DefaultTools()[0]
	first := string(tool.inputSchema())
	second := string(tool.inputSchema())
	if first != second {
		t.Fatal("inputSchema is not deterministic across calls")
	}
}
