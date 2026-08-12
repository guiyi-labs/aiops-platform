package httpserver

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// updateMatrix regenerates docs/security/permission-matrix.md from the live
// routeTable when the committed document is out of date.
var updateMatrix = flag.Bool("update", false, "regenerate docs/security/permission-matrix.md")

// matrixDocPath is relative to the httpserver package directory
// (backend/internal/httpserver -> repository root).
const matrixDocPath = "../../../docs/security/permission-matrix.md"

func buildMatrix(t *testing.T) PermissionMatrix {
	t.Helper()
	buildFullEngine(t)
	return BuildPermissionMatrix()
}

// TestPermissionMatrixMatchesCommittedDocument is the diff gate for the
// permission matrix: the committed document must equal the generated matrix.
// Route/role/audit changes without updating the document fail here.
func TestPermissionMatrixMatchesCommittedDocument(t *testing.T) {
	matrix := buildMatrix(t)
	rendered := matrix.RenderMarkdown()
	committed, err := os.ReadFile(matrixDocPath)
	if err != nil {
		if *updateMatrix {
			if werr := os.WriteFile(matrixDocPath, []byte(rendered), 0o644); werr != nil {
				t.Fatalf("update matrix %s: %v", matrixDocPath, werr)
			}
			t.Logf("generated %s", matrixDocPath)
			return
		}
		t.Fatalf("read committed matrix %s: %v (run with -update to generate)", matrixDocPath, err)
	}
	if string(committed) == rendered {
		return
	}
	if *updateMatrix {
		if err := os.WriteFile(matrixDocPath, []byte(rendered), 0o644); err != nil {
			t.Fatalf("update matrix %s: %v", matrixDocPath, err)
		}
		t.Logf("updated %s", matrixDocPath)
		return
	}
	t.Errorf("permission matrix is out of date; run `go test ./internal/httpserver -run TestPermissionMatrixMatchesCommittedDocument -update`")
}

// TestPermissionMatrixScopeInvariants asserts structural guarantees: namespace
// scope implies the route also carries cluster_id (group nesting), workspace
// routes never carry cluster/namespace keys, and every route has exactly one
// scope classification.
func TestPermissionMatrixScopeInvariants(t *testing.T) {
	matrix := buildMatrix(t)
	seen := map[string]PermissionEntry{}
	for _, e := range matrix.Entries {
		key := e.Method + " " + e.Path
		if prev, dup := seen[key]; dup {
			t.Errorf("duplicate matrix row: %s (scope %s vs %s)", key, prev.Scope, e.Scope)
		}
		seen[key] = e
		switch e.Scope {
		case ScopeNamespace:
			if !strings.Contains(e.Path, ":cluster_id") {
				t.Errorf("namespace-scoped route lacks :cluster_id: %s", key)
			}
		case ScopeWorkspace:
			if strings.Contains(e.Path, ":cluster_id") || strings.Contains(e.Path, ":namespace") {
				t.Errorf("workspace route carries cluster/namespace key: %s", key)
			}
		case ScopeCluster, ScopeNone:
		default:
			t.Errorf("unknown scope %q for %s", e.Scope, key)
		}
	}
}

// TestPermissionMatrixRolesClosedSet ensures every role used in the matrix is
// one of the four platform roles; any other value is a contract violation.
func TestPermissionMatrixRolesClosedSet(t *testing.T) {
	matrix := buildMatrix(t)
	for _, e := range matrix.Entries {
		for _, role := range e.Roles {
			if _, ok := validRoles[role]; !ok {
				t.Errorf("%s %s: unknown role %q", e.Method, e.Path, role)
			}
		}
	}
}

// TestPermissionMatrixEntriesSorted ensures rendering is deterministic.
func TestPermissionMatrixEntriesSorted(t *testing.T) {
	matrix := buildMatrix(t)
	for i := 1; i < len(matrix.Entries); i++ {
		prev := matrix.Entries[i-1]
		cur := matrix.Entries[i]
		if cur.Path < prev.Path || (cur.Path == prev.Path && cur.Method < prev.Method) {
			t.Fatalf("entries not sorted by (path, method) at index %d: %s %s before %s %s",
				i, cur.Method, cur.Path, prev.Method, prev.Path)
		}
	}
}

// TestPermissionMatrixDocumentPathSanity guards against accidental path moves.
func TestPermissionMatrixDocumentPathSanity(t *testing.T) {
	abs := filepath.Clean(matrixDocPath)
	if !strings.HasSuffix(abs, "docs/security/permission-matrix.md") {
		t.Fatalf("matrix doc path drifted: %s", abs)
	}
}
