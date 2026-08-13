package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/notification"
)

type notificationRepositoryStub struct {
	response notification.ListResponse
	retryID  int64
	retryErr error
}

func (s *notificationRepositoryStub) SetEnabled(context.Context, bool) error { return nil }
func (s *notificationRepositoryStub) Enqueue(context.Context, notification.EnqueueInput) error {
	return nil
}
func (s *notificationRepositoryStub) Claim(context.Context, int, time.Time) ([]notification.Delivery, error) {
	return nil, nil
}
func (s *notificationRepositoryStub) MarkDelivered(context.Context, int64, time.Time) error {
	return nil
}
func (s *notificationRepositoryStub) MarkFailed(context.Context, int64, int, time.Time, string) error {
	return nil
}
func (s *notificationRepositoryStub) List(context.Context, notification.ListFilter) (notification.ListResponse, error) {
	return s.response, nil
}
func (s *notificationRepositoryStub) Retry(_ context.Context, id int64) error {
	s.retryID = id
	return s.retryErr
}

func TestNotificationListDoesNotExposePayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repository := &notificationRepositoryStub{response: notification.ListResponse{Items: []notification.Delivery{{ID: 1, DiagnosisID: 7, EventType: "diagnosis.created", Status: "pending", Payload: []byte(`{"summary":"must-not-leak"}`)}}, Total: 1}}
	service := notification.NewService(notification.ServiceConfig{}, repository, nil)
	router := gin.New()
	router.GET("/api/v1/notification-deliveries", notificationHandler{service: service}.list)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/notification-deliveries?limit=50", nil))
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), "must-not-leak") || !strings.Contains(recorder.Body.String(), `"diagnosis_id":7`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestNotificationRetryQueuesDeadDelivery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repository := &notificationRepositoryStub{}
	service := notification.NewService(notification.ServiceConfig{Enabled: true}, repository, nil)
	router := gin.New()
	router.POST("/api/v1/notification-deliveries/:delivery_id/retry", notificationHandler{service: service}.retry)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/notification-deliveries/19/retry", nil))
	if recorder.Code != http.StatusAccepted || repository.retryID != 19 {
		t.Fatalf("status=%d retryID=%d body=%s", recorder.Code, repository.retryID, recorder.Body.String())
	}
}

func TestNotificationRetryRejectsWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repository := &notificationRepositoryStub{}
	service := notification.NewService(notification.ServiceConfig{}, repository, nil)
	router := gin.New()
	router.POST("/api/v1/notification-deliveries/:delivery_id/retry", notificationHandler{service: service}.retry)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/notification-deliveries/19/retry", nil))
	if recorder.Code != http.StatusConflict || repository.retryID != 0 {
		t.Fatalf("status=%d retryID=%d", recorder.Code, repository.retryID)
	}
}

func TestNotificationListAcceptsIncidentIDFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repository := &notificationRepositoryStub{response: notification.ListResponse{Items: []notification.Delivery{}, Total: 0}}
	service := notification.NewService(notification.ServiceConfig{}, repository, nil)
	router := gin.New()
	router.GET("/api/v1/notification-deliveries", notificationHandler{service: service}.list)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/notification-deliveries?incident_id=10&limit=25", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "0") {
		t.Fatalf("expected zero total, body=%s", recorder.Body.String())
	}
}

func TestNotificationListRejectsUnsupportedEventType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := notification.NewService(notification.ServiceConfig{}, &notificationRepositoryStub{}, nil)
	router := gin.New()
	router.GET("/api/v1/notification-deliveries", notificationHandler{service: service}.list)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/notification-deliveries?event_type=unknown.event", nil))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "INVALID_QUERY") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
