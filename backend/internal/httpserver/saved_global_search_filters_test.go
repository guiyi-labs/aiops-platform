package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/globalsearch"
	"k8s-aiops.local/backend/internal/requestctx"
)

type savedFilterHTTPRepositoryStub struct {
	err          error
	actorID      int64
	item         globalsearch.SavedFilter
	changes      globalsearch.SavedFilterChanges
	deletedID    int64
	createdLimit int
}

func (s *savedFilterHTTPRepositoryStub) ListSavedFilters(_ context.Context, userID int64) ([]globalsearch.SavedFilter, error) {
	s.actorID = userID
	if s.err != nil {
		return nil, s.err
	}
	return []globalsearch.SavedFilter{s.item}, nil
}

func (s *savedFilterHTTPRepositoryStub) CreateSavedFilter(_ context.Context, item globalsearch.SavedFilter, limit int) (globalsearch.SavedFilter, error) {
	s.actorID, s.item, s.createdLimit = item.UserID, item, limit
	if s.err != nil {
		return globalsearch.SavedFilter{}, s.err
	}
	s.item.ID = 12
	return s.item, nil
}

func (s *savedFilterHTTPRepositoryStub) UpdateSavedFilter(_ context.Context, userID, id int64, changes globalsearch.SavedFilterChanges) (globalsearch.SavedFilter, error) {
	s.actorID, s.changes = userID, changes
	if s.err != nil {
		return globalsearch.SavedFilter{}, s.err
	}
	return globalsearch.SavedFilter{ID: id, UserID: userID, Name: valueOr(changes.Name, "Existing"), Query: "api", Kinds: []globalsearch.Kind{globalsearch.KindPod}, SchemaVersion: 1}, nil
}

func (s *savedFilterHTTPRepositoryStub) DeleteSavedFilter(_ context.Context, userID, id int64) error {
	s.actorID, s.deletedID = userID, id
	return s.err
}

func valueOr(value *string, fallback string) string {
	if value != nil {
		return *value
	}
	return fallback
}

func savedFilterTestRouter(repository globalsearch.SavedFilterRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		metadata := requestctx.Metadata{ActorID: 73, RequestID: "saved-filter-test"}
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), metadata))
		c.Next()
	})
	handler := savedGlobalSearchFilterHandler{service: globalsearch.NewSavedFilterService(repository)}
	router.GET("/filters", handler.list)
	router.POST("/filters", handler.create)
	router.PATCH("/filters/:filter_id", handler.update)
	router.DELETE("/filters/:filter_id", handler.delete)
	return router
}

func TestSavedFilterHandlersUseAuthenticatedActor(t *testing.T) {
	repository := &savedFilterHTTPRepositoryStub{item: globalsearch.SavedFilter{ID: 1, Name: "API", Query: "api", Kinds: []globalsearch.Kind{globalsearch.KindPod}, SchemaVersion: 1}}
	router := savedFilterTestRouter(repository)

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/filters", nil))
	if list.Code != http.StatusOK || repository.actorID != 73 {
		t.Fatalf("list status=%d actor=%d body=%s", list.Code, repository.actorID, list.Body.String())
	}
	var response globalsearch.SavedFilterListResponse
	if err := json.Unmarshal(list.Body.Bytes(), &response); err != nil || response.Total != 1 || response.Limit != 20 {
		t.Fatalf("list response=%#v error=%v", response, err)
	}

	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/filters", strings.NewReader(`{"name":" Production API ","query":"api","namespace":"prod","kinds":["Service","Pod"]}`)))
	if create.Code != http.StatusCreated || repository.actorID != 73 || repository.createdLimit != 20 || repository.item.Name != "Production API" {
		t.Fatalf("create status=%d repository=%#v body=%s", create.Code, repository, create.Body.String())
	}

	rename := httptest.NewRecorder()
	router.ServeHTTP(rename, httptest.NewRequest(http.MethodPatch, "/filters/12", strings.NewReader(`{"name":"Renamed"}`)))
	if rename.Code != http.StatusOK || repository.actorID != 73 || repository.changes.Name == nil || *repository.changes.Name != "Renamed" {
		t.Fatalf("rename status=%d repository=%#v body=%s", rename.Code, repository, rename.Body.String())
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(http.MethodDelete, "/filters/12", nil))
	if remove.Code != http.StatusNoContent || repository.actorID != 73 || repository.deletedID != 12 {
		t.Fatalf("delete status=%d repository=%#v body=%s", remove.Code, repository, remove.Body.String())
	}
}

func TestSavedFilterHandlersRejectUnknownAndPartialFields(t *testing.T) {
	for _, test := range []struct {
		method, path, body string
	}{
		{method: http.MethodPost, path: "/filters", body: `{"name":"API","query":"api","kinds":["Pod"],"selector":"app=api"}`},
		{method: http.MethodPatch, path: "/filters/1", body: `{"query":"api"}`},
		{method: http.MethodPatch, path: "/filters/1", body: `{}`},
	} {
		recorder := httptest.NewRecorder()
		savedFilterTestRouter(&savedFilterHTTPRepositoryStub{}).ServeHTTP(recorder, httptest.NewRequest(test.method, test.path, strings.NewReader(test.body)))
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"INVALID_SAVED_FILTER"`) {
			t.Fatalf("%s %s status=%d body=%s", test.method, test.path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestSavedFilterHandlersMapStableErrors(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{err: globalsearch.ErrSavedFilterNotFound, status: http.StatusNotFound, code: "SAVED_FILTER_NOT_FOUND"},
		{err: globalsearch.ErrSavedFilterLimit, status: http.StatusConflict, code: "SAVED_FILTER_LIMIT_REACHED"},
		{err: globalsearch.ErrSavedFilterNameExists, status: http.StatusConflict, code: "SAVED_FILTER_NAME_EXISTS"},
		{err: errors.New("database detail"), status: http.StatusInternalServerError, code: "SAVED_FILTERS_FAILED"},
	}
	for _, test := range tests {
		recorder := httptest.NewRecorder()
		savedFilterTestRouter(&savedFilterHTTPRepositoryStub{err: test.err}).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/filters", strings.NewReader(`{"name":"API","query":"api","kinds":["Pod"]}`)))
		if recorder.Code != test.status || !strings.Contains(recorder.Body.String(), `"code":"`+test.code+`"`) {
			t.Fatalf("error=%v status=%d body=%s", test.err, recorder.Code, recorder.Body.String())
		}
	}
}

func TestSavedFilterHandlerRejectsInvalidID(t *testing.T) {
	recorder := httptest.NewRecorder()
	savedFilterTestRouter(&savedFilterHTTPRepositoryStub{}).ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/filters/not-a-number", nil))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"INVALID_SAVED_FILTER_ID"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
