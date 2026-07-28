package httpserver

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap/zaptest"
	"gopkg.in/yaml.v3"

	"k8s-aiops.local/backend/internal/aiexplain"
	"k8s-aiops.local/backend/internal/audit"
	"k8s-aiops.local/backend/internal/auth"
	"k8s-aiops.local/backend/internal/cluster"
	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/fleet"
	"k8s-aiops.local/backend/internal/globalsearch"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/notification"
	"k8s-aiops.local/backend/internal/remediation"
)

type openAPIDocument struct {
	Paths map[string]map[string]yaml.Node `yaml:"paths"`
}

var ginPathParameter = regexp.MustCompile(`:([A-Za-z0-9_]+)`)

func TestRegisteredRoutesMatchOpenAPI(t *testing.T) {
	documentPath := filepath.Join(repositoryRoot(t), "docs", "api", "openapi.yaml")
	contents, err := os.ReadFile(documentPath)
	if err != nil {
		t.Fatalf("read OpenAPI document: %v", err)
	}
	var document openAPIDocument
	if err := yaml.Unmarshal(contents, &document); err != nil {
		t.Fatalf("parse OpenAPI document: %v", err)
	}

	gin.SetMode(gin.TestMode)
	engine, ok := New(zaptest.NewLogger(t), Options{
		Probe:         probeStub{},
		Auth:          &auth.Service{},
		Clusters:      &cluster.Service{},
		Kubernetes:    &k8sgateway.Service{},
		Diagnosis:     &diagnosis.Service{},
		Audit:         &audit.Service{},
		AIExplanation: &aiexplain.Service{},
		Notifications: notification.NewService(notification.ServiceConfig{}, nil, nil),
		Remediation:   remediation.NewService(nil, nil, nil),
		Fleet:         fleet.NewService(fleet.Config{}, nil, nil),
		GlobalSearch:  globalsearch.NewService(globalsearch.Config{}, nil, nil),
		SavedFilters:  globalsearch.NewSavedFilterService(nil),
		Version:       "route-contract-test",
	}).(*gin.Engine)
	if !ok {
		t.Fatal("http server is not a gin engine")
	}

	registered := make(map[string]struct{})
	for _, route := range engine.Routes() {
		registered[route.Method+" "+normalizeOpenAPIPath(route.Path)] = struct{}{}
	}
	documented := make(map[string]struct{})
	for path, operations := range document.Paths {
		for method := range operations {
			if !isHTTPMethod(method) {
				continue
			}
			documented[strings.ToUpper(method)+" "+path] = struct{}{}
		}
	}

	if missing := routeSetDifference(documented, registered); len(missing) > 0 {
		t.Fatalf("OpenAPI documents routes not registered by Gin: %s", strings.Join(missing, ", "))
	}
	if undocumented := routeSetDifference(registered, documented); len(undocumented) > 0 {
		t.Fatalf("Gin registers routes missing from OpenAPI: %s", strings.Join(undocumented, ", "))
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate route contract test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}

func normalizeOpenAPIPath(path string) string {
	return ginPathParameter.ReplaceAllString(path, `{$1}`)
}

func isHTTPMethod(method string) bool {
	switch method {
	case "get", "post", "put", "patch", "delete", "options", "head", "trace":
		return true
	default:
		return false
	}
}

func routeSetDifference(left, right map[string]struct{}) []string {
	values := make([]string, 0)
	for value := range left {
		if _, ok := right[value]; !ok {
			values = append(values, value)
		}
	}
	sort.Strings(values)
	return values
}
