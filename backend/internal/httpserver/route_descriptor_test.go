package httpserver

import (
	"net/http"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap/zaptest"

	"k8s-aiops.local/backend/internal/aiexplain"
	"k8s-aiops.local/backend/internal/audit"
	"k8s-aiops.local/backend/internal/auth"
	"k8s-aiops.local/backend/internal/backup"
	"k8s-aiops.local/backend/internal/cluster"
	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/fleet"
	"k8s-aiops.local/backend/internal/globalsearch"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/maintenance"
	"k8s-aiops.local/backend/internal/namespaceposture"
	"k8s-aiops.local/backend/internal/notification"
	"k8s-aiops.local/backend/internal/promotion"
	"k8s-aiops.local/backend/internal/remediation"
	"k8s-aiops.local/backend/internal/restore"
)

// validRoles is the closed set of platform roles allowed inside
// RouteDescriptor.RequiredRoles. Any other value is a contract violation.
var validRoles = map[string]struct{}{
	auth.SystemAdmin:     {},
	auth.OperationsAdmin: {},
	auth.SecurityAuditor: {},
	auth.Viewer:          {},
}

var auditActionPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// buildDescriptorEngine mirrors TestRegisteredRoutesMatchOpenAPI's harness so
// the routeTable is populated exactly as in production.
func buildDescriptorEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine, ok := New(zaptest.NewLogger(t), Options{
		Probe:            probeStub{},
		Auth:             &auth.Service{},
		Clusters:         &cluster.Service{},
		Kubernetes:       &k8sgateway.Service{},
		Diagnosis:        &diagnosis.Service{},
		Audit:            &audit.Service{},
		AIExplanation:    &aiexplain.Service{},
		Notifications:    notification.NewService(notification.ServiceConfig{}, nil, nil),
		Remediation:      remediation.NewService(nil, nil, nil),
		Promotion:        promotion.NewService(nil, nil),
		Fleet:            fleet.NewService(fleet.Config{}, nil, nil),
		GlobalSearch:     globalsearch.NewService(globalsearch.Config{}, nil, nil),
		SavedFilters:     globalsearch.NewSavedFilterService(nil),
		MetricsHistory:   mustMetricsHistoryService(t),
		Alert:            mustAlertService(t),
		Backup:           backup.NewService(nil, nil),
		Maintenance:      maintenance.NewService(nil, nil),
		NamespacePosture: namespaceposture.NewService(nil),
		Restore:          restore.NewService(nil, nil),
		Version:          "route-descriptor-test",
	}).(*gin.Engine)
	if !ok {
		t.Fatal("http server is not a gin engine")
	}
	return engine
}

// TestRouteTableCoversAllGinRoutes verifies every Gin-registered route has a
// matching descriptor in routeTable. This catches any route registered via
// raw group.Handle that bypassed routeRegistrar. System routes outside
// /api/v1 (e.g. /metrics) are excluded since they are not part of the
// versioned API contract.
func TestRouteTableCoversAllGinRoutes(t *testing.T) {
	engine := buildDescriptorEngine(t)

	excluded := map[string]struct{}{
		"GET /metrics": {},
	}
	registered := make(map[string]struct{})
	for _, r := range engine.Routes() {
		key := r.Method + " " + r.Path
		if _, skip := excluded[key]; skip {
			continue
		}
		registered[key] = struct{}{}
	}

	descriptor := make(map[string]struct{})
	for _, r := range routeTable {
		descriptor[r.Method+" "+r.FullPath] = struct{}{}
	}

	missing := make([]string, 0)
	for key := range registered {
		if _, ok := descriptor[key]; !ok {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("routes registered in Gin but missing from routeTable: %s", strings.Join(missing, ", "))
	}
}

// TestDescriptorMetadataWellFormed runs table-driven assertions over every
// routeTable entry: audited routes must have action+resource, role-restricted
// routes must use valid platform roles, and audit actions must follow the
// dotted lowercase_with_underscores convention.
func TestDescriptorMetadataWellFormed(t *testing.T) {
	buildDescriptorEngine(t)

	for _, r := range routeTable {
		if r.AuditAction == "" && r.AuditResource != "" {
			t.Errorf("%s %s: AuditResource set but AuditAction empty", r.Method, r.FullPath)
		}
		if r.AuditAction != "" && r.AuditResource == "" {
			t.Errorf("%s %s: AuditAction set but AuditResource empty", r.Method, r.FullPath)
		}
		if r.AuditAction != "" && !auditActionPattern.MatchString(r.AuditAction) {
			t.Errorf("%s %s: AuditAction %q does not match dotted lowercase convention", r.Method, r.FullPath, r.AuditAction)
		}
		for _, role := range r.RequiredRoles {
			if _, ok := validRoles[role]; !ok {
				t.Errorf("%s %s: unknown RequiredRole %q", r.Method, r.FullPath, role)
			}
		}
	}
}

// TestDescriptorHTTPMethodsValid guards against typos like "get" or "Get" in
// RouteDescriptor.Method, which would silently create dead routes.
func TestDescriptorHTTPMethodsValid(t *testing.T) {
	buildDescriptorEngine(t)
	valid := map[string]struct{}{
		http.MethodGet: {}, http.MethodPost: {}, http.MethodPut: {},
		http.MethodPatch: {}, http.MethodDelete: {}, http.MethodHead: {}, http.MethodOptions: {},
	}
	for _, r := range routeTable {
		if _, ok := valid[r.Method]; !ok {
			t.Errorf("%s %s: invalid HTTP method %q", r.Method, r.FullPath, r.Method)
		}
	}
}

// TestDescriptorFullPathStartsWithAPIV1 ensures no descriptor escapes the
// /api/v1 prefix, which would silently bypass the versioned contract.
func TestDescriptorFullPathStartsWithAPIV1(t *testing.T) {
	buildDescriptorEngine(t)
	for _, r := range routeTable {
		if !strings.HasPrefix(r.FullPath, "/api/v1") {
			t.Errorf("%s %s: FullPath outside /api/v1", r.Method, r.FullPath)
		}
	}
}

// TestDescriptorNoDuplicateRoutes catches accidental re-registration of the
// same method+path, which would shadow the first descriptor and its audit
// metadata.
func TestDescriptorNoDuplicateRoutes(t *testing.T) {
	buildDescriptorEngine(t)
	seen := make(map[string]struct{})
	for _, r := range routeTable {
		key := r.Method + " " + r.FullPath
		if _, ok := seen[key]; ok {
			t.Errorf("duplicate descriptor registration: %s", key)
		}
		seen[key] = struct{}{}
	}
}
