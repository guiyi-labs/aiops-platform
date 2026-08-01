package servicemesh

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/apiquery"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/metricshistory"
)

type fakeCRD struct {
	customResources func(ctx context.Context, clusterID int64, group, version, resource, namespace string, q apiquery.ListQuery) (apiquery.ListResponse[map[string]interface{}], error)
	customResource  func(ctx context.Context, clusterID int64, group, version, resource, namespace, name string) (map[string]interface{}, error)
}

func (f fakeCRD) CustomResources(ctx context.Context, clusterID int64, group, version, resource, namespace string, q apiquery.ListQuery) (apiquery.ListResponse[map[string]interface{}], error) {
	if f.customResources == nil {
		return apiquery.ListResponse[map[string]interface{}]{}, errors.New("not implemented")
	}
	return f.customResources(ctx, clusterID, group, version, resource, namespace, q)
}

func (f fakeCRD) CustomResource(ctx context.Context, clusterID int64, group, version, resource, namespace, name string) (map[string]interface{}, error) {
	if f.customResource == nil {
		return nil, errors.New("not implemented")
	}
	return f.customResource(ctx, clusterID, group, version, resource, namespace, name)
}

type fakeMetricsReader struct {
	// responses stores ordered responses, returned one-per-Query call.
	responses []metricshistory.SeriesResponse
	// callIdx tracks how many responses have been consumed.
	callIdx int
	err     error
}

func (f *fakeMetricsReader) Query(_ context.Context, q metricshistory.SeriesQuery) (metricshistory.SeriesResponse, error) {
	if f.err != nil {
		return metricshistory.SeriesResponse{}, f.err
	}
	if f.callIdx >= len(f.responses) {
		return metricshistory.SeriesResponse{
			Series: metricshistory.Series{
				ClusterID:         q.ClusterID,
				ResourceKind:      q.ResourceKind,
				ResourceNamespace: q.ResourceNamespace,
				ResourceName:      q.ResourceName,
				MetricName:        q.MetricName,
			},
		}, nil
	}
	resp := f.responses[f.callIdx]
	f.callIdx++
	return resp, nil
}

func sampleVS(name, namespace, uid string) map[string]interface{} {
	return map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":              name,
			"namespace":         namespace,
			"uid":               uid,
			"creationTimestamp": "2026-01-01T00:00:00Z",
			"labels":            map[string]interface{}{"app": "x"},
		},
		"spec": map[string]interface{}{
			"hosts":    []interface{}{"reviews.example.com"},
			"gateways": []interface{}{"mesh"},
			"http": []interface{}{
				map[string]interface{}{
					"route": []interface{}{
						map[string]interface{}{
							"destination": map[string]interface{}{
								"host":   "reviews",
								"subset": "v1",
							},
							"weight": float64(90),
						},
						map[string]interface{}{
							"destination": map[string]interface{}{
								"host":   "reviews",
								"subset": "v2",
							},
							"weight": float64(10),
						},
					},
				},
			},
		},
	}
}

func sampleDR(name, namespace, uid, host string) map[string]interface{} {
	return map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":              name,
			"namespace":         namespace,
			"uid":               uid,
			"creationTimestamp": "2026-01-01T00:00:00Z",
		},
		"spec": map[string]interface{}{
			"host": host,
			"subsets": []interface{}{
				map[string]interface{}{"name": "v1"},
				map[string]interface{}{"name": "v2"},
			},
			"trafficPolicy": map[string]interface{}{
				"tls":              map[string]interface{}{"mode": "ISTIO_MUTUAL"},
				"outlierDetection": map[string]interface{}{"consecutiveErrors": 5},
			},
		},
	}
}

func TestListVirtualServicesProjectsFields(t *testing.T) {
	crd := fakeCRD{customResources: func(_ context.Context, _ int64, g, v, r, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[map[string]interface{}], error) {
		if g != APIGroupNetworking || v != APIVersionV1Beta1 || r != ResourceVirtualService {
			t.Errorf("unexpected call: %s/%s/%s", g, v, r)
		}
		return apiquery.ListResponse[map[string]interface{}]{
			Items: []map[string]interface{}{sampleVS("reviews", "prod", "u1")},
			Total: 1,
		}, nil
	}}
	svc := NewService(crd, nil)
	got, err := svc.ListVirtualServices(context.Background(), 1, ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListVirtualServices: %v", err)
	}
	if got.Total != 1 {
		t.Fatalf("total = %d, want 1", got.Total)
	}
	v := got.Items[0]
	if v.Name != "reviews" || v.Namespace != "prod" || v.UID != "u1" {
		t.Errorf("metadata = %+v, want reviews/prod/u1", v)
	}
	if len(v.Hosts) != 1 || v.Hosts[0] != "reviews.example.com" {
		t.Errorf("hosts = %v", v.Hosts)
	}
	if len(v.Destinations) != 2 {
		t.Fatalf("destinations = %d, want 2", len(v.Destinations))
	}
	d := v.Destinations[0]
	if d.Host != "reviews" || d.Subset != "v1" || d.Weight != 90 {
		t.Errorf("dest[0] = %+v", d)
	}
}

func TestListDestinationRulesProjectsSubsets(t *testing.T) {
	crd := fakeCRD{customResources: func(_ context.Context, _ int64, _g, _v, _r, _ns string, _ apiquery.ListQuery) (apiquery.ListResponse[map[string]interface{}], error) {
		return apiquery.ListResponse[map[string]interface{}]{
			Items: []map[string]interface{}{sampleDR("reviews-dr", "prod", "u2", "reviews")},
			Total: 1,
		}, nil
	}}
	svc := NewService(crd, nil)
	got, err := svc.ListDestinationRules(context.Background(), 1, ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListDestinationRules: %v", err)
	}
	d := got.Items[0]
	if d.Host != "reviews" {
		t.Errorf("host = %q", d.Host)
	}
	if d.SubsetCount != 2 {
		t.Errorf("subset count = %d", d.SubsetCount)
	}
	if len(d.SubsetNames) != 2 || d.SubsetNames[0] != "v1" || d.SubsetNames[1] != "v2" {
		t.Errorf("subset names = %v", d.SubsetNames)
	}
	if d.TrafficPolicySummary == "" {
		t.Error("expected non-empty traffic policy summary")
	}
}

func TestTrafficMetricsValidateWindow(t *testing.T) {
	r := &fakeMetricsReader{}
	svc := NewService(nil, r)
	// reversed window
	end := time.Now().UTC()
	start := end.Add(time.Hour)
	_, err := svc.TrafficMetrics(context.Background(), TrafficQuery{WindowStart: start, WindowEnd: end})
	if !errors.Is(err, ErrInvalidWindow) {
		t.Errorf("want ErrInvalidWindow, got %v", err)
	}
	// too-large window
	start = end.Add(25 * time.Hour)
	_, err = svc.TrafficMetrics(context.Background(), TrafficQuery{WindowStart: start, WindowEnd: end})
	if !errors.Is(err, ErrInvalidWindow) {
		t.Errorf("want ErrInvalidWindow for >24h, got %v", err)
	}
}

func TestTrafficMetricsReturnsErrWithoutReader(t *testing.T) {
	svc := NewService(nil, nil)
	_, err := svc.TrafficMetrics(context.Background(), TrafficQuery{ClusterID: 1})
	if !errors.Is(err, ErrMeshDataUnavailable) {
		t.Errorf("want ErrMeshDataUnavailable, got %v", err)
	}
}

func TestTrafficMetricsAggregatesPoints(t *testing.T) {
	now := time.Now().UTC()
	nowM1 := now.Add(-time.Minute)
	// Metric order in TrafficMetrics loop:
	// istio_requests_total, istio_requests_errors, p50, p95, p99
	responses := []metricshistory.SeriesResponse{
		{
			Series: metricshistory.Series{ResourceNamespace: "prod", ResourceName: "reviews", MetricName: "istio_requests_total"},
			Points: []metricshistory.Point{
				{Value: 500, SourceTimestamp: nowM1, WindowMilliseconds: 60000, CollectedAt: nowM1},
				{Value: 1500, SourceTimestamp: now, WindowMilliseconds: 60000, CollectedAt: now},
			},
		},
		{
			Series: metricshistory.Series{ResourceNamespace: "prod", ResourceName: "reviews", MetricName: "istio_requests_errors"},
			Points: []metricshistory.Point{
				{Value: 10, SourceTimestamp: nowM1, WindowMilliseconds: 60000, CollectedAt: nowM1},
				{Value: 60, SourceTimestamp: now, WindowMilliseconds: 60000, CollectedAt: now},
			},
		},
		{
			Series: metricshistory.Series{ResourceNamespace: "prod", ResourceName: "reviews", MetricName: "istio_latency_ms_p50"},
			Points: []metricshistory.Point{
				{Value: 20, SourceTimestamp: nowM1, WindowMilliseconds: 60000, CollectedAt: nowM1},
				{Value: 40, SourceTimestamp: now, WindowMilliseconds: 60000, CollectedAt: now},
			},
		},
		{
			Series: metricshistory.Series{ResourceNamespace: "prod", ResourceName: "reviews", MetricName: "istio_latency_ms_p95"},
			Points: []metricshistory.Point{
				{Value: 100, SourceTimestamp: nowM1, WindowMilliseconds: 60000, CollectedAt: nowM1},
				{Value: 200, SourceTimestamp: now, WindowMilliseconds: 60000, CollectedAt: now},
			},
		},
		{
			Series: metricshistory.Series{ResourceNamespace: "prod", ResourceName: "reviews", MetricName: "istio_latency_ms_p99"},
			Points: []metricshistory.Point{
				{Value: 300, SourceTimestamp: nowM1, WindowMilliseconds: 60000, CollectedAt: nowM1},
				{Value: 500, SourceTimestamp: now, WindowMilliseconds: 60000, CollectedAt: now},
			},
		},
	}
	r := &fakeMetricsReader{responses: responses}
	svc := NewService(nil, r)
	got, err := svc.TrafficMetrics(context.Background(), TrafficQuery{ClusterID: 1, Namespace: "prod", WindowStart: now.Add(-time.Hour), WindowEnd: now})
	if err != nil {
		t.Fatalf("TrafficMetrics: %v", err)
	}
	if len(got.Services) != 1 {
		t.Fatalf("services = %d, want 1", len(got.Services))
	}
	top := got.Services[0]
	if top.ServiceName != "reviews" {
		t.Errorf("top service = %q, want reviews", top.ServiceName)
	}
	if top.TotalRequests != 1000 || top.TotalErrors != 50 {
		t.Errorf("requests/errors = %d/%d, want 1000/50", top.TotalRequests, top.TotalErrors)
	}
	if top.P50LatencyMs != 40 || top.P95LatencyMs != 200 || top.P99LatencyMs != 500 {
		t.Errorf("latencies = %.1f, %.1f, %.1f, want 40, 200, 500", top.P50LatencyMs, top.P95LatencyMs, top.P99LatencyMs)
	}
	if top.ErrorRatePct < 4.9 || top.ErrorRatePct > 5.1 {
		t.Errorf("error rate = %.2f%%, want 5%%", top.ErrorRatePct)
	}
}

func TestSensitiveAnnotationsRedacted(t *testing.T) {
	m := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      "x",
			"namespace": "y",
			"uid":       "z",
			"annotations": map[string]interface{}{
				"foo":                 "ok",
				"mysecret-annotation": "super-secret",
				"tls.cert":            "CERTIFICATE DATA",
			},
		},
		"spec": map[string]interface{}{"hosts": []interface{}{}},
	}
	v, err := projectVirtualService(m)
	if err != nil {
		t.Fatal(err)
	}
	if v.Annotations["foo"] != "ok" {
		t.Errorf("foo annotation should be ok, got %q", v.Annotations["foo"])
	}
	if v.Annotations["mysecret-annotation"] != "[redacted]" {
		t.Errorf("secret annotation should be redacted, got %q", v.Annotations["mysecret-annotation"])
	}
	if v.Annotations["tls.cert"] != "[redacted]" {
		t.Errorf("tls.cert should be redacted, got %q", v.Annotations["tls.cert"])
	}
}

func TestIstioNotInstalledMapsError(t *testing.T) {
	crd := fakeCRD{customResources: func(_ context.Context, _ int64, _g, _v, _r, _ns string, _ apiquery.ListQuery) (apiquery.ListResponse[map[string]interface{}], error) {
		return apiquery.ListResponse[map[string]interface{}]{}, k8sgateway.ErrResourceNotFound
	}}
	svc := NewService(crd, nil)
	_, err := svc.ListVirtualServices(context.Background(), 1, ListFilter{})
	if !errors.Is(err, ErrIstioNotInstalled) {
		t.Errorf("want ErrIstioNotInstalled, got %v", err)
	}
	_, err = svc.ListDestinationRules(context.Background(), 1, ListFilter{})
	if !errors.Is(err, ErrIstioNotInstalled) {
		t.Errorf("want ErrIstioNotInstalled for DR, got %v", err)
	}
}
