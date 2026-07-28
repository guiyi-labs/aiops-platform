package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap/zaptest"
)

type probeStub struct {
	err error
}

func (p probeStub) Ping(context.Context) error {
	return p.err
}

func TestLiveness(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)

	New(zaptest.NewLogger(t), Options{Probe: probeStub{}, Version: "test"}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Header().Get(requestIDHeader) == "" {
		t.Fatal("X-Request-ID response header is empty")
	}
}

func TestRequestIDAndErrorResponse(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/missing", nil)
	request.Header.Set(requestIDHeader, "client-request-1")

	New(zaptest.NewLogger(t), Options{Probe: probeStub{}, Version: "test"}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	if recorder.Header().Get(requestIDHeader) != "client-request-1" {
		t.Fatalf("X-Request-ID = %q, want client-request-1", recorder.Header().Get(requestIDHeader))
	}

	var response errorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Code != "ROUTE_NOT_FOUND" || response.RequestID != "client-request-1" {
		t.Fatalf("error response = %#v", response)
	}
}

func TestReadiness(t *testing.T) {
	tests := []struct {
		name       string
		probeError error
		wantStatus int
	}{
		{name: "ready", wantStatus: http.StatusOK},
		{name: "database unavailable", probeError: errors.New("database unavailable"), wantStatus: http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)

			New(zaptest.NewLogger(t), Options{Probe: probeStub{err: tt.probeError}, Version: "test"}).ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
		})
	}
}

func TestMetricsEndpoint(t *testing.T) {
	metrics := NewMetrics()
	metrics.Observe(http.MethodGet, "/api/v1/health/live", http.StatusOK, 5)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)

	New(zaptest.NewLogger(t), Options{Probe: probeStub{}, Version: "test", Metrics: metrics}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/plain; version=0.0.4; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
	if body := recorder.Body.String(); !strings.Contains(body, `route="/api/v1/health/live"`) {
		t.Fatalf("metrics body = %s", body)
	}
}
