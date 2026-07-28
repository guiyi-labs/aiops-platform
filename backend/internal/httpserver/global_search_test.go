package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/cluster"
	"k8s-aiops.local/backend/internal/globalsearch"
)

type searchClusterStub struct{ err error }

func (s searchClusterStub) List(context.Context) ([]cluster.Cluster, error) { return nil, s.err }

func TestGlobalSearchHandlerParsesBoundedQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := globalsearch.NewService(globalsearch.Config{}, searchClusterStub{}, nil)
	router := gin.New()
	router.GET("/search", globalSearchHandler{service: service}.search)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/search?q=api&namespace=prod&kinds=pods,services&cluster_limit=2&limit=12", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response globalsearch.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Query != "api" || response.Namespace != "prod" || len(response.Kinds) != 2 || response.Kinds[0] != globalsearch.KindPod || response.Kinds[1] != globalsearch.KindService {
		t.Fatalf("response = %#v", response)
	}
}

func TestGlobalSearchHandlerRejectsInvalidInputAndHidesDirectoryFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name    string
		path    string
		service *globalsearch.Service
		status  int
		code    string
	}{
		{name: "short term", path: "/search?q=a", service: globalsearch.NewService(globalsearch.Config{}, searchClusterStub{}, nil), status: http.StatusBadRequest, code: "INVALID_QUERY"},
		{name: "unknown kind", path: "/search?q=api&kinds=secrets", service: globalsearch.NewService(globalsearch.Config{}, searchClusterStub{}, nil), status: http.StatusBadRequest, code: "INVALID_QUERY"},
		{name: "excess limit", path: "/search?q=api&limit=101", service: globalsearch.NewService(globalsearch.Config{}, searchClusterStub{}, nil), status: http.StatusBadRequest, code: "INVALID_QUERY"},
		{name: "directory failure", path: "/search?q=api", service: globalsearch.NewService(globalsearch.Config{}, searchClusterStub{err: errors.New("database detail")}, nil), status: http.StatusInternalServerError, code: "GLOBAL_SEARCH_FAILED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/search", globalSearchHandler{service: tt.service}.search)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if recorder.Code != tt.status {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var response errorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Code != tt.code || response.Message == "database detail" {
				t.Fatalf("response = %#v", response)
			}
		})
	}
}
