package capability

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestTLSServer returns an HTTPS test server whose handler can be replaced
// after the issuer URL is known (mirroring the oidc test helper).
func newTestTLSServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	return server
}

// testHTTPClient returns an HTTP client that trusts the test server's
// certificate and never follows redirects, mirroring the production bounded
// client (CheckRedirect returns http.ErrUseLastResponse).
func testHTTPClient(t *testing.T, server *httptest.Server) *http.Client {
	t.Helper()
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	return &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}},
	}
}

func validMetricsQuery() MetricsQuery {
	return MetricsQuery{
		ClusterID:   1,
		Namespace:   "default",
		ServiceName: "api",
		Template:    TemplateRequestRate,
		Start:       time.Unix(1672531200, 0).UTC(),
		End:         time.Unix(1672531260, 0).UTC(),
		Step:        30 * time.Second,
	}
}

func validLogQuery() LogQuery {
	return LogQuery{
		ClusterID: 1,
		Namespace: "default",
		PodName:   "api-0",
		Container: "api",
		Start:     time.Unix(1672531200, 0).UTC(),
		End:       time.Unix(1672531260, 0).UTC(),
		Direction: DirectionForward,
		Limit:     100,
		MaxBytes:  MaxLogBytes,
	}
}

func TestValidateMetricsQuery(t *testing.T) {
	base := validMetricsQuery()
	tests := []struct {
		name    string
		mutate  func(MetricsQuery) MetricsQuery
		wantErr bool
	}{
		{"valid query", func(q MetricsQuery) MetricsQuery { return q }, false},
		{"invalid template", func(q MetricsQuery) MetricsQuery { q.Template = "unknown"; return q }, true},
		{"zero step", func(q MetricsQuery) MetricsQuery { q.Step = 0; return q }, true},
		{"negative step", func(q MetricsQuery) MetricsQuery { q.Step = -time.Second; return q }, true},
		{"end before start", func(q MetricsQuery) MetricsQuery { q.End = q.Start.Add(-time.Second); return q }, true},
		{"range too large", func(q MetricsQuery) MetricsQuery { q.End = q.Start.Add(MaxLogRange + time.Hour); return q }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMetricsQuery(tt.mutate(base))
			if (err != nil) != tt.wantErr {
				t.Errorf("validateMetricsQuery() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidMetricsQuery) {
				t.Errorf("validateMetricsQuery() error = %v, want wrap ErrInvalidMetricsQuery", err)
			}
		})
	}
}

func TestValidateLogQuery(t *testing.T) {
	base := validLogQuery()
	tests := []struct {
		name    string
		mutate  func(LogQuery) LogQuery
		wantErr bool
	}{
		{"valid query", func(q LogQuery) LogQuery { return q }, false},
		{"text too long", func(q LogQuery) LogQuery { q.TextFilter = strings.Repeat("x", MaxTextFilter+1); return q }, true},
		{"limit too high", func(q LogQuery) LogQuery { q.Limit = MaxLogLimit + 1; return q }, true},
		{"range too large", func(q LogQuery) LogQuery { q.End = q.Start.Add(MaxLogRange + time.Hour); return q }, true},
		{"invalid direction", func(q LogQuery) LogQuery { q.Direction = "sideways"; return q }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLogQuery(tt.mutate(base))
			if (err != nil) != tt.wantErr {
				t.Errorf("validateLogQuery() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidLogQuery) {
				t.Errorf("validateLogQuery() error = %v, want wrap ErrInvalidLogQuery", err)
			}
		})
	}
}

func TestNopMetricsProviderReturnsUnavailable(t *testing.T) {
	provider := NopMetricsProvider{}
	result, err := provider.QueryMetrics(context.Background(), validMetricsQuery())
	if err != nil {
		t.Fatalf("QueryMetrics error = %v", err)
	}
	if result.State != StateUnavailable {
		t.Fatalf("State = %q, want %q", result.State, StateUnavailable)
	}
	if provider.Name() != "nop" {
		t.Fatalf("Name = %q, want %q", provider.Name(), "nop")
	}
}

func TestNopLogProviderReturnsUnavailable(t *testing.T) {
	provider := NopLogProvider{}
	result, err := provider.QueryLogs(context.Background(), validLogQuery())
	if err != nil {
		t.Fatalf("QueryLogs error = %v", err)
	}
	if result.State != StateUnavailable {
		t.Fatalf("State = %q, want %q", result.State, StateUnavailable)
	}
	if provider.Name() != "nop" {
		t.Fatalf("Name = %q, want %q", provider.Name(), "nop")
	}
}

// newTestPrometheusProvider constructs a provider bound to the test server,
// overriding the HTTP client so the test server's self-signed certificate is
// trusted while preserving the no-redirect invariant.
func newTestPrometheusProvider(t *testing.T, server *httptest.Server, config PrometheusConfig) *PrometheusMetricsProvider {
	t.Helper()
	if config.Endpoint == "" {
		config.Endpoint = server.URL
	}
	provider, err := NewPrometheusMetricsProvider(config)
	if err != nil {
		t.Fatalf("NewPrometheusMetricsProvider error = %v", err)
	}
	provider.client = testHTTPClient(t, server)
	return provider
}

func newTestLokiProvider(t *testing.T, server *httptest.Server, config LokiConfig) *LokiLogProvider {
	t.Helper()
	if config.Endpoint == "" {
		config.Endpoint = server.URL
	}
	provider, err := NewLokiLogProvider(config)
	if err != nil {
		t.Fatalf("NewLokiLogProvider error = %v", err)
	}
	provider.client = testHTTPClient(t, server)
	return provider
}

// prometheusMatrixHandler serves a successful matrix response with the given
// number of series, each carrying two points.
func prometheusMatrixHandler(seriesCount int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result := make([]map[string]any, 0, seriesCount)
		for i := 0; i < seriesCount; i++ {
			result = append(result, map[string]any{
				"metric": map[string]string{"namespace": "default", "service": "api", "instance": fmt.Sprintf("i%d", i)},
				"values": []any{
					[]any{1672531200.0, "1.5"},
					[]any{1672531230.0, "2.5"},
				},
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "matrix",
				"result":     result,
			},
		})
	})
}

func TestPrometheusHappyPath(t *testing.T) {
	server := newTestTLSServer(t, prometheusMatrixHandler(1))
	provider := newTestPrometheusProvider(t, server, PrometheusConfig{})

	result, err := provider.QueryMetrics(context.Background(), validMetricsQuery())
	if err != nil {
		t.Fatalf("QueryMetrics error = %v", err)
	}
	if result.State != StateComplete {
		t.Fatalf("State = %q, want %q", result.State, StateComplete)
	}
	if result.Template != TemplateRequestRate {
		t.Fatalf("Template = %q, want %q", result.Template, TemplateRequestRate)
	}
	if result.SchemaVersion != MetricsSchemaVersion {
		t.Fatalf("SchemaVersion = %q, want %q", result.SchemaVersion, MetricsSchemaVersion)
	}
	if len(result.Series) != 1 {
		t.Fatalf("Series len = %d, want 1", len(result.Series))
	}
	if len(result.Series[0].Points) != 2 {
		t.Fatalf("Points len = %d, want 2", len(result.Series[0].Points))
	}
	if result.Series[0].Points[0].Value != 1.5 {
		t.Fatalf("first point value = %v, want 1.5", result.Series[0].Points[0].Value)
	}
	if !result.Series[0].Points[1].Timestamp.Equal(time.Unix(1672531230, 0).UTC()) {
		t.Fatalf("second point timestamp = %v, want 2023-01-01T00:00:30Z", result.Series[0].Points[1].Timestamp)
	}
	if result.Coverage.TotalSamples != 2 || result.Coverage.ExpectedSamples != 2 || result.Coverage.MissingSamples != 0 {
		t.Fatalf("Coverage = %#v", result.Coverage)
	}
	if result.Coverage.Source != "prometheus" {
		t.Fatalf("Coverage.Source = %q, want %q", result.Coverage.Source, "prometheus")
	}
	if !result.Freshness.Equal(time.Unix(1672531230, 0).UTC()) {
		t.Fatalf("Freshness = %v, want 2023-01-01T00:00:30Z", result.Freshness)
	}
}

func TestPrometheusProviderError(t *testing.T) {
	server := newTestTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	provider := newTestPrometheusProvider(t, server, PrometheusConfig{})

	result, err := provider.QueryMetrics(context.Background(), validMetricsQuery())
	if err != nil {
		t.Fatalf("QueryMetrics error = %v", err)
	}
	if result.State != StateUnavailable {
		t.Fatalf("State = %q, want %q", result.State, StateUnavailable)
	}
	if result.Error == "" {
		t.Fatalf("Error is empty, want a sanitized message")
	}
}

func TestPrometheusTimeout(t *testing.T) {
	server := newTestTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   map[string]any{"resultType": "matrix", "result": []any{}},
		})
	}))
	provider := newTestPrometheusProvider(t, server, PrometheusConfig{RequestTimeout: 50 * time.Millisecond})

	result, err := provider.QueryMetrics(context.Background(), validMetricsQuery())
	if err != nil {
		t.Fatalf("QueryMetrics error = %v", err)
	}
	if result.State != StateUnavailable {
		t.Fatalf("State = %q, want %q", result.State, StateUnavailable)
	}
	if !strings.Contains(result.Error, "timed out") {
		t.Fatalf("Error = %q, want it to mention timeout", result.Error)
	}
}

func TestPrometheusMaxSeriesEnforcement(t *testing.T) {
	server := newTestTLSServer(t, prometheusMatrixHandler(3))
	provider := newTestPrometheusProvider(t, server, PrometheusConfig{MaxSeries: 2})

	result, err := provider.QueryMetrics(context.Background(), validMetricsQuery())
	if err != nil {
		t.Fatalf("QueryMetrics error = %v", err)
	}
	if len(result.Series) != 2 {
		t.Fatalf("Series len = %d, want 2 (capped at MaxSeries)", len(result.Series))
	}
}

func TestPrometheusNoRedirect(t *testing.T) {
	var redirected int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/query_range", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/v1/redirected", http.StatusFound)
	})
	mux.HandleFunc("/api/v1/redirected", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&redirected, 1)
		w.WriteHeader(http.StatusOK)
	})
	server := newTestTLSServer(t, mux)
	provider := newTestPrometheusProvider(t, server, PrometheusConfig{})

	result, err := provider.QueryMetrics(context.Background(), validMetricsQuery())
	if err != nil {
		t.Fatalf("QueryMetrics error = %v", err)
	}
	if result.State != StateUnavailable {
		t.Fatalf("State = %q, want %q", result.State, StateUnavailable)
	}
	if atomic.LoadInt32(&redirected) != 0 {
		t.Fatalf("provider followed redirect; redirected called %d times", redirected)
	}
}

// lokiStreamsHandler serves a successful Loki response with the given number of
// entries spread across a single stream.
func lokiStreamsHandler(entryCount int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		values := make([]any, 0, entryCount)
		for i := 0; i < entryCount; i++ {
			ts := int64(1672531200+i) * 1_000_000_000
			values = append(values, []any{fmt.Sprintf("%d", ts), fmt.Sprintf("line %d", i+1)})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"result": []map[string]any{
					{
						"stream": map[string]string{"namespace": "default", "pod": "api-0", "container": "api"},
						"values": values,
					},
				},
			},
		})
	})
}

func TestLokiHappyPath(t *testing.T) {
	server := newTestTLSServer(t, lokiStreamsHandler(2))
	provider := newTestLokiProvider(t, server, LokiConfig{})

	result, err := provider.QueryLogs(context.Background(), validLogQuery())
	if err != nil {
		t.Fatalf("QueryLogs error = %v", err)
	}
	if result.State != StateComplete {
		t.Fatalf("State = %q, want %q", result.State, StateComplete)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("Entries len = %d, want 2", len(result.Entries))
	}
	if result.TotalReturned != 2 {
		t.Fatalf("TotalReturned = %d, want 2", result.TotalReturned)
	}
	if result.Entries[0].Line != "line 1" {
		t.Fatalf("first line = %q, want %q", result.Entries[0].Line, "line 1")
	}
	if result.Entries[0].Namespace != "default" || result.Entries[0].Pod != "api-0" || result.Entries[0].Container != "api" {
		t.Fatalf("first entry labels = %#v", result.Entries[0])
	}
	if !result.Entries[1].Timestamp.Equal(time.Unix(1672531201, 0).UTC()) {
		t.Fatalf("second timestamp = %v, want 2023-01-01T00:00:01Z", result.Entries[1].Timestamp)
	}
}

func TestLokiProviderError(t *testing.T) {
	server := newTestTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	provider := newTestLokiProvider(t, server, LokiConfig{})

	result, err := provider.QueryLogs(context.Background(), validLogQuery())
	if err != nil {
		t.Fatalf("QueryLogs error = %v", err)
	}
	if result.State != StateUnavailable {
		t.Fatalf("State = %q, want %q", result.State, StateUnavailable)
	}
	if result.Error == "" {
		t.Fatalf("Error is empty, want a sanitized message")
	}
}

func TestLokiTimeout(t *testing.T) {
	server := newTestTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   map[string]any{"result": []any{}},
		})
	}))
	provider := newTestLokiProvider(t, server, LokiConfig{RequestTimeout: 50 * time.Millisecond})

	result, err := provider.QueryLogs(context.Background(), validLogQuery())
	if err != nil {
		t.Fatalf("QueryLogs error = %v", err)
	}
	if result.State != StateUnavailable {
		t.Fatalf("State = %q, want %q", result.State, StateUnavailable)
	}
	if !strings.Contains(result.Error, "timed out") {
		t.Fatalf("Error = %q, want it to mention timeout", result.Error)
	}
}

func TestLokiMaxEntriesEnforcement(t *testing.T) {
	server := newTestTLSServer(t, lokiStreamsHandler(3))
	provider := newTestLokiProvider(t, server, LokiConfig{MaxEntries: 2})

	query := validLogQuery()
	query.Limit = 0 // defer to provider MaxEntries
	result, err := provider.QueryLogs(context.Background(), query)
	if err != nil {
		t.Fatalf("QueryLogs error = %v", err)
	}
	if result.State != StateTruncated {
		t.Fatalf("State = %q, want %q", result.State, StateTruncated)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("Entries len = %d, want 2 (capped at MaxEntries)", len(result.Entries))
	}
	if result.TotalReturned != 2 {
		t.Fatalf("TotalReturned = %d, want 2", result.TotalReturned)
	}
}

func TestLokiNoRedirect(t *testing.T) {
	var redirected int32
	mux := http.NewServeMux()
	mux.HandleFunc("/loki/api/v1/query_range", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loki/api/v1/redirected", http.StatusFound)
	})
	mux.HandleFunc("/loki/api/v1/redirected", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&redirected, 1)
		w.WriteHeader(http.StatusOK)
	})
	server := newTestTLSServer(t, mux)
	provider := newTestLokiProvider(t, server, LokiConfig{})

	result, err := provider.QueryLogs(context.Background(), validLogQuery())
	if err != nil {
		t.Fatalf("QueryLogs error = %v", err)
	}
	if result.State != StateUnavailable {
		t.Fatalf("State = %q, want %q", result.State, StateUnavailable)
	}
	if atomic.LoadInt32(&redirected) != 0 {
		t.Fatalf("provider followed redirect; redirected called %d times", redirected)
	}
}

func TestErrorSanitization(t *testing.T) {
	t.Run("prometheus endpoint not leaked", func(t *testing.T) {
		server := newTestTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "upstream failure", http.StatusInternalServerError)
		}))
		provider := newTestPrometheusProvider(t, server, PrometheusConfig{})

		result, err := provider.QueryMetrics(context.Background(), validMetricsQuery())
		if err != nil {
			t.Fatalf("QueryMetrics error = %v", err)
		}
		if strings.Contains(result.Error, server.URL) {
			t.Fatalf("Error leaks endpoint URL: %q", result.Error)
		}
		host := strings.TrimPrefix(server.URL, "https://")
		if strings.Contains(result.Error, host) {
			t.Fatalf("Error leaks host:port: %q", result.Error)
		}
		if strings.Contains(result.Error, "prometheus") {
			t.Fatalf("Error leaks provider name: %q", result.Error)
		}
	})
	t.Run("loki endpoint not leaked", func(t *testing.T) {
		server := newTestTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "upstream failure", http.StatusBadGateway)
		}))
		provider := newTestLokiProvider(t, server, LokiConfig{})

		result, err := provider.QueryLogs(context.Background(), validLogQuery())
		if err != nil {
			t.Fatalf("QueryLogs error = %v", err)
		}
		if strings.Contains(result.Error, server.URL) {
			t.Fatalf("Error leaks endpoint URL: %q", result.Error)
		}
		host := strings.TrimPrefix(server.URL, "https://")
		if strings.Contains(result.Error, host) {
			t.Fatalf("Error leaks host:port: %q", result.Error)
		}
		if strings.Contains(result.Error, "loki") {
			t.Fatalf("Error leaks provider name: %q", result.Error)
		}
	})
}

func TestRenderLogQLEscapesInjection(t *testing.T) {
	query := LogQuery{
		Namespace:  `default`,
		PodName:    `api-0`,
		Container:  `api`,
		TextFilter: `evil" injected`,
	}
	rendered := renderLogQL(query)
	// The injected double quote must be escaped so it cannot terminate the
	// string literal and inject LogQL beyond the single text filter.
	if strings.Contains(rendered, `evil" injected`) {
		t.Fatalf("text filter quote not escaped: %q", rendered)
	}
	if !strings.Contains(rendered, `evil\" injected`) {
		t.Fatalf("text filter not properly escaped: %q", rendered)
	}
	if strings.Count(rendered, "|=") != 1 {
		t.Fatalf("expected exactly one line filter, got %q", rendered)
	}

	// Label values with quotes must also be escaped so they cannot break the
	// stream selector.
	selectorQuery := LogQuery{Namespace: `ns"`, PodName: `pod"`, Container: `ctr"`, Direction: DirectionForward}
	selectorRendered := renderLogQL(selectorQuery)
	if strings.Contains(selectorRendered, `ns"`) || strings.Contains(selectorRendered, `pod"`) || strings.Contains(selectorRendered, `ctr"`) {
		t.Fatalf("label quote not escaped: %q", selectorRendered)
	}
}

func TestNewPrometheusMetricsProviderRejectsNonHTTPS(t *testing.T) {
	if _, err := NewPrometheusMetricsProvider(PrometheusConfig{Endpoint: "http://prometheus.local"}); err == nil {
		t.Fatalf("expected error for non-HTTPS endpoint")
	}
	if _, err := NewPrometheusMetricsProvider(PrometheusConfig{Endpoint: "https://user:pass@prometheus.local"}); err == nil {
		t.Fatalf("expected error for endpoint with userinfo")
	}
}

func TestNewLokiLogProviderRejectsNonHTTPS(t *testing.T) {
	if _, err := NewLokiLogProvider(LokiConfig{Endpoint: "http://loki.local"}); err == nil {
		t.Fatalf("expected error for non-HTTPS endpoint")
	}
	if _, err := NewLokiLogProvider(LokiConfig{Endpoint: "https://user:pass@loki.local"}); err == nil {
		t.Fatalf("expected error for endpoint with userinfo")
	}
}
