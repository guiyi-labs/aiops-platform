package mcpserver

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// argumentValuePattern is the character set an argument value may use.
//
// This is the load-bearing control in the package. Path placeholders are
// substituted from argument values, so anything able to terminate the path or
// inject a second request — '/', '?', '#', whitespace, '%', a dot-dot segment —
// has to be excluded. A strict allowlist is the only version of that which
// stays correct as the catalogue grows; a denylist would need updating every
// time someone thinks of a new metacharacter.
//
// The leading character must be alphanumeric, which also rules out leading
// dots, dashes and colons.
var argumentValuePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

// Arg declares one accepted argument of a tool.
type Arg struct {
	Name        string
	Description string
	Required    bool
}

// Tool is one published capability.
type Tool struct {
	Name        string
	Description string
	Args        []Arg
	// Path is the fixed backend endpoint. A brace placeholder is filled from
	// the argument of the same name; a caller never supplies a path.
	Path string
	// Query lists the argument names forwarded as query parameters. An
	// argument absent from this list never reaches the backend, so a tool
	// cannot be used to probe undocumented parameters.
	Query []string
}

// DefaultTools returns the published catalogue.
//
// The set is closed and hand-written on purpose: every entry is a capability
// someone decided to expose, and there is no configuration path that could
// widen it at runtime. Each maps to a GET endpoint the platform already
// serves — the MCP server adds reach, not capability, and adds no privilege
// of its own.
func DefaultTools() []Tool {
	return []Tool{
		{
			Name:        "list_diagnoses",
			Description: "List recent cluster diagnoses, newest first. Read-only. Returns the platform's diagnosis records, each with its rule id, severity and target resource.",
			Path:        "/api/v1/diagnoses",
			Args: []Arg{
				{Name: "cluster_id", Description: "restrict to one cluster id"},
				{Name: "status", Description: "restrict to one diagnosis status"},
				{Name: "limit", Description: "maximum number of records to return"},
			},
			Query: []string{"cluster_id", "status", "limit"},
		},
		{
			Name:        "get_diagnosis",
			Description: "Fetch one diagnosis record by id, including its conclusion and the evidence identifiers behind it. Read-only.",
			Path:        "/api/v1/diagnoses/{diagnosis_id}",
			Args: []Arg{
				{Name: "diagnosis_id", Description: "diagnosis record id", Required: true},
			},
		},
		{
			Name:        "get_diagnosis_evidence",
			Description: "Replay the evidence chain of one diagnosis: the signals that were correlated and why the conclusion follows. Read-only.",
			Path:        "/api/v1/diagnoses/{diagnosis_id}/replay",
			Args: []Arg{
				{Name: "diagnosis_id", Description: "diagnosis record id", Required: true},
			},
		},
		{
			Name:        "list_incidents",
			Description: "List incidents tracked by the platform. Read-only.",
			Path:        "/api/v1/incidents",
			Args: []Arg{
				{Name: "limit", Description: "maximum number of records to return"},
			},
			Query: []string{"limit"},
		},
		{
			Name:        "get_incident",
			Description: "Fetch one incident by id. Read-only.",
			Path:        "/api/v1/incidents/{incident_id}",
			Args: []Arg{
				{Name: "incident_id", Description: "incident id", Required: true},
			},
		},
		{
			Name:        "get_incident_evidence",
			Description: "Fetch the evidence attached to one incident. Read-only.",
			Path:        "/api/v1/incidents/{incident_id}/evidence",
			Args: []Arg{
				{Name: "incident_id", Description: "incident id", Required: true},
			},
		},
		{
			Name:        "search_knowledge",
			Description: "Search the distilled case library of previously resolved failures. Each entry carries the confirmed root cause and the remediation that worked. Read-only.",
			Path:        "/api/v1/aiops/knowledge",
			Args: []Arg{
				{Name: "rule_id", Description: "restrict to one diagnosis rule id"},
				{Name: "resource_kind", Description: "restrict to one Kubernetes resource kind"},
				{Name: "severity", Description: "restrict to exactly this severity"},
				{Name: "min_severity", Description: "keep entries at or above this severity (info|warning|high|critical)"},
				{Name: "limit", Description: "maximum number of entries to return"},
			},
			Query: []string{"rule_id", "resource_kind", "severity", "min_severity", "limit"},
		},
		{
			Name:        "knowledge_stats",
			Description: "Report how many cases the library holds, so an agent can tell an empty library from an unsuccessful search. Read-only.",
			Path:        "/api/v1/aiops/knowledge/stats",
		},
		{
			Name:        "get_cluster",
			Description: "Fetch one cluster's record by id. Read-only.",
			Path:        "/api/v1/clusters/{cluster_id}",
			Args: []Arg{
				{Name: "cluster_id", Description: "cluster id", Required: true},
			},
		},
	}
}

// ValidateArguments checks a call against the tool's declared shape and returns
// the accepted values.
//
// Unknown arguments are an error rather than being dropped quietly: a caller
// probing for undocumented parameters should be told the call was rejected, not
// handed a normal-looking response it will misread as success.
func (t Tool) ValidateArguments(raw map[string]string) (map[string]string, error) {
	declared := make(map[string]struct{}, len(t.Args))
	for _, a := range t.Args {
		declared[a.Name] = struct{}{}
	}

	accepted := make(map[string]string, len(raw))
	for name, value := range raw {
		if _, ok := declared[name]; !ok {
			return nil, fmt.Errorf("unknown argument %q", name)
		}
		if !argumentValuePattern.MatchString(value) {
			return nil, fmt.Errorf("argument %q has an unacceptable value", name)
		}
		accepted[name] = value
	}

	for _, a := range t.Args {
		if !a.Required {
			continue
		}
		if _, ok := accepted[a.Name]; !ok {
			return nil, fmt.Errorf("missing required argument %q", a.Name)
		}
	}
	return accepted, nil
}

// URLPath renders the fixed path with its placeholders substituted from the
// accepted arguments. Values reaching this point have already passed the
// allowlist, so the result stays within the endpoint this tool declared.
func (t Tool) URLPath(accepted map[string]string) (string, error) {
	path := t.Path
	for _, a := range t.Args {
		placeholder := "{" + a.Name + "}"
		if !strings.Contains(path, placeholder) {
			continue
		}
		value, ok := accepted[a.Name]
		if !ok {
			return "", fmt.Errorf("missing path argument %q", a.Name)
		}
		path = strings.ReplaceAll(path, placeholder, value)
	}
	if strings.ContainsAny(path, "{}") {
		return "", fmt.Errorf("unresolved placeholder in %q", t.Path)
	}
	return path, nil
}

// inputSchema renders the JSON Schema advertised for this tool. A map marshals
// with sorted keys, so the schema is byte-stable across calls — clients that
// cache it see no spurious change.
func (t Tool) inputSchema() json.RawMessage {
	properties := make(map[string]any, len(t.Args))
	required := make([]string, 0, len(t.Args))
	for _, a := range t.Args {
		properties[a.Name] = map[string]any{"type": "string", "description": a.Description}
		if a.Required {
			required = append(required, a.Name)
		}
	}
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		// A schema built from a literal map cannot fail to marshal; returning
		// an empty object keeps tools/list total rather than partial.
		return json.RawMessage(`{"type":"object"}`)
	}
	return encoded
}
