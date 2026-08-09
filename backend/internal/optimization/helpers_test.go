package optimization

// Coverage for the pure helper functions of the collector and metrics
// sources: quantity parsing, bucketing, selector/rule mapping, image
// extraction, annotation matching, p95 aggregation and service defaults.
// No cluster access is needed.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/finops"
	"k8s-aiops.local/backend/internal/kubernetes"
)

func TestParseCPU(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"", 0},
		{"0", 0},
		{"1", 1_000_000_000},
		{"4000m", 4_000_000_000},
		{"garbage", 0},
	}
	for _, c := range cases {
		if got := parseCPU(c.in); got != c.want {
			t.Errorf("parseCPU(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseMem(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"", 0},
		{"128Mi", 128 * 1024 * 1024},
		{"1Gi", 1024 * 1024 * 1024},
		{"garbage", 0},
	}
	for _, c := range cases {
		if got := parseMem(c.in); got != c.want {
			t.Errorf("parseMem(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestBucket(t *testing.T) {
	in := time.Date(2026, 8, 9, 10, 30, 45, 123456789, time.UTC)
	want := time.Date(2026, 8, 9, 10, 30, 0, 0, time.UTC).Unix()
	if got := bucket(in); got != want {
		t.Errorf("bucket(%v) = %d, want %d", in, got, want)
	}
}

func TestToSamples(t *testing.T) {
	if got := toSamples(nil); got != nil {
		t.Errorf("toSamples(nil) = %v, want nil", got)
	}
	samples := toSamples(map[int64]float64{200: 2.5, 100: 1.5})
	if len(samples) != 2 {
		t.Fatalf("len(samples) = %d, want 2", len(samples))
	}
	if samples[0].Timestamp.Unix() != 100 || samples[0].Value != 1.5 {
		t.Errorf("samples[0] = %+v, want {100 1.5}", samples[0])
	}
	if samples[1].Timestamp.Unix() != 200 || samples[1].Value != 2.5 {
		t.Errorf("samples[1] = %+v, want {200 2.5}", samples[1])
	}
}

func TestRawToText(t *testing.T) {
	cases := []struct {
		in   json.RawMessage
		want string
	}{
		{nil, ""},
		{json.RawMessage(`"tcp"`), "tcp"},
		{json.RawMessage(`8080`), "8080"},
		{json.RawMessage(`{}`), ""},
	}
	for _, c := range cases {
		if got := rawToText(c.in); got != c.want {
			t.Errorf("rawToText(%v) = %q, want %q", string(c.in), got, c.want)
		}
	}
}

func TestIntOrString(t *testing.T) {
	cases := []struct {
		in   json.RawMessage
		want string
	}{
		{nil, ""},
		{json.RawMessage(`"http"`), "http"},
		{json.RawMessage(`8080`), "8080"},
		{json.RawMessage(`{}`), ""},
	}
	for _, c := range cases {
		if got := intOrString(c.in); got != c.want {
			t.Errorf("intOrString(%v) = %q, want %q", string(c.in), got, c.want)
		}
	}
}

func TestToSelector(t *testing.T) {
	if toSelector(nil) != nil {
		t.Error("toSelector(nil) should be nil")
	}
	plain := toSelector(&selectorRaw{MatchLabels: map[string]string{"app": "x"}})
	if plain == nil || !plain.HasExpressions == false {
		t.Errorf("toSelector(plain) = %+v", plain)
	}
	expr := toSelector(&selectorRaw{
		MatchLabels:      map[string]string{"app": "x"},
		MatchExpressions: []json.RawMessage{json.RawMessage(`{"key":"k","operator":"In"}`)},
	})
	if expr == nil || !expr.HasExpressions {
		t.Errorf("toSelector(with expressions) = %+v, want HasExpressions", expr)
	}
}

func TestToRules(t *testing.T) {
	if got := toRules(nil, true); got != nil {
		t.Errorf("toRules(nil) = %v, want nil", got)
	}
	ingress := toRules([]netRuleRaw{{
		From: []peerRaw{{
			PodSelector: &selectorRaw{MatchLabels: map[string]string{"a": "b"}},
			IPBlock: &struct {
				CIDR   string   `json:"cidr"`
				Except []string `json:"except"`
			}{CIDR: "10.0.0.0/8", Except: []string{"10.0.0.1"}},
		}},
		Ports: []struct {
			Protocol string          `json:"protocol"`
			Port     json.RawMessage `json:"port"`
			EndPort  int32           `json:"endPort"`
		}{{Protocol: "TCP", Port: json.RawMessage(`"http"`)}},
	}}, true)
	if len(ingress) != 1 || len(ingress[0].Peers) != 1 {
		t.Fatalf("ingress rules = %+v, want 1 rule 1 peer", ingress)
	}
	if ingress[0].Peers[0].IPBlockCIDR != "10.0.0.0/8" || len(ingress[0].Peers[0].IPBlockExcept) != 1 {
		t.Errorf("peer ipBlock = %+v", ingress[0].Peers[0])
	}
	if len(ingress[0].Ports) != 1 || ingress[0].Ports[0].Port != "http" || ingress[0].Ports[0].Protocol != "TCP" {
		t.Errorf("ports = %+v, want one named TCP port", ingress[0].Ports)
	}
	egress := toRules([]netRuleRaw{{
		To: []peerRaw{{PodSelector: &selectorRaw{MatchLabels: map[string]string{"a": "b"}}}},
	}}, false)
	if len(egress) != 1 || len(egress[0].Peers) != 1 {
		t.Fatalf("egress rules = %+v, want 1 rule 1 peer", egress)
	}
}

func TestMatchLabels(t *testing.T) {
	if !matchLabels(map[string]string{"app": "x"}, nil) {
		t.Error("empty selector should match everything")
	}
	if !matchLabels(map[string]string{"a": "1", "app": "x"}, map[string]string{"app": "x"}) {
		t.Error("subset selector should match")
	}
	if matchLabels(map[string]string{"a": "1"}, map[string]string{"app": "x"}) {
		t.Error("missing key should not match")
	}
}

func TestToSubjects(t *testing.T) {
	out := toSubjects([]struct {
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}{{Kind: "User", Name: "alice"}})
	if len(out) != 1 || out[0].Kind != "User" || out[0].Name != "alice" {
		t.Errorf("toSubjects = %+v", out)
	}
}

func TestNamespaceManagedByGitOps(t *testing.T) {
	if !namespaceManagedByGitOps(map[string]string{"kustomize.toolkit.fluxcd.io/name": "x"}) {
		t.Error("fluxcd annotation should mark managed")
	}
	if !namespaceManagedByGitOps(map[string]string{"argocd.argoproj.io/name": "x"}) {
		t.Error("argocd annotation should mark managed")
	}
	if namespaceManagedByGitOps(map[string]string{"kubernetes.io/metadata.name": "x"}) {
		t.Error("unrelated annotation should not mark managed")
	}
}

func TestHasAnnotationPrefix(t *testing.T) {
	if !hasAnnotationPrefix(map[string]string{"a.fluxcd.io/k": "v"}, "fluxcd.io") {
		t.Error("containing prefix should match")
	}
	if hasAnnotationPrefix(map[string]string{"other": "v"}, "fluxcd.io") {
		t.Error("absent prefix should not match")
	}
	if hasAnnotationPrefix(nil, "fluxcd.io") {
		t.Error("nil map should not match")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "b", "c"); got != "b" {
		t.Errorf("firstNonEmpty = %q, want b", got)
	}
	if got := firstNonEmpty(); got != "" {
		t.Errorf("firstNonEmpty() = %q, want empty", got)
	}
}

func TestImageUsages(t *testing.T) {
	spec := podSpecImageRaw{
		InitContainers: []containerImageRaw{{Name: "init", Image: ""}},
		Containers: []containerImageRaw{
			{Name: "app", Image: "nginx:1.25", ImagePullPolicy: "IfNotPresent"},
		},
	}
	usages := imageUsages("Deployment", "ns", "svc", spec)
	if len(usages) != 1 {
		t.Fatalf("imageUsages = %d items, want 1 (empty init image skipped)", len(usages))
	}
	u := usages[0]
	if u.Container.WorkloadKind != "Deployment" || u.Container.WorkloadName != "svc" || u.Container.Container != "app" {
		t.Errorf("usage container = %+v", u.Container)
	}
	if u.Image.Repository == "" || u.Image.Tag != "1.25" || u.Image.PullPolicy != "IfNotPresent" {
		t.Errorf("usage image = %+v, want nginx:1.25 IfNotPresent", u.Image)
	}
}

func TestPodP95(t *testing.T) {
	ctx := context.Background()
	c := &Collector{metrics: fakeMetrics{cpu: 500_000_000, mem: 256 * 1024 * 1024}}
	pods := []podLite{
		{Namespace: "ns", Name: "p1", Labels: map[string]string{"app": "web"}},
		{Namespace: "other", Name: "p2", Labels: map[string]string{"app": "web"}},
	}
	cpu, mem, ok := c.podP95(ctx, 1, "ns", pods, map[string]string{"app": "web"}, "app")
	if !ok || cpu != 500_000_000 || mem != 256*1024*1024 {
		t.Errorf("podP95 = %d,%d,%v, want 5e8,256Mi,true", cpu, mem, ok)
	}
	if _, _, ok := c.podP95(ctx, 1, "ns", pods, map[string]string{"app": "nope"}, "app"); ok {
		t.Error("selector without matches should report ok=false")
	}
	c2 := &Collector{}
	if _, _, ok := c2.podP95(ctx, 1, "ns", pods, nil, "app"); ok {
		t.Error("nil metrics should report ok=false")
	}
}

func TestListPodLite(t *testing.T) {
	ctx := context.Background()
	c := &Collector{lister: &fakeLister{data: map[string][]json.RawMessage{
		"/api/v1/pods": {
			json.RawMessage(`{"metadata":{"name":"p1","namespace":"a","labels":{"k":"v"}}}`),
			json.RawMessage(`{"metadata":{"name":"p2","namespace":"b"}}`),
			json.RawMessage(`not-json`),
		},
	}}}
	got := c.listPodLite(ctx, 1)
	if len(got) != 2 {
		t.Fatalf("listPodLite = %d items, want 2 (invalid skipped)", len(got))
	}
	if got[0].Namespace != "a" || got[0].Name != "p1" || got[0].Labels["k"] != "v" {
		t.Errorf("listPodLite[0] = %+v", got[0])
	}
	c2 := &Collector{lister: &errLister{}}
	if got := c2.listPodLite(ctx, 1); got != nil {
		t.Errorf("listPodLite with lister error = %v, want nil", got)
	}
}

func TestFinopsInput(t *testing.T) {
	ctx := context.Background()
	ctr := kubernetes.WorkloadContainer{
		Name: "app",
		Resources: kubernetes.ResourceRequirements{
			Requests: map[string]string{"cpu": "100m", "memory": "128Mi"},
			Limits:   map[string]string{"cpu": "200m", "memory": "256Mi"},
		},
	}
	in := (&Collector{}).finopsInput(ctx, 1, "Deployment", "ns", "svc", 3, ctr, nil, nil)
	if in.WorkloadName != "svc" || in.Replicas != 3 || in.Requests.CPURequest != 100_000_000 {
		t.Errorf("finopsInput(no metrics) = %+v", in)
	}
	with := (&Collector{metrics: fakeMetrics{cpu: 500_000_000, mem: 64 * 1024 * 1024}}).finopsInput(
		ctx, 1, "Deployment", "ns", "svc", 3, ctr,
		[]podLite{{Namespace: "ns", Name: "p", Labels: map[string]string{"app": "svc"}}},
		map[string]string{"app": "svc"},
	)
	if with.CPUUsageP95 != 500_000_000 || with.MemUsageP95 != 64*1024*1024 {
		t.Errorf("finopsInput(with metrics) = %+v", with)
	}
}

func TestMetricsHistorySourceDefaults(t *testing.T) {
	m := NewMetricsHistorySource(nil, 0).(metricsHistorySource)
	if m.window != 24*time.Hour {
		t.Errorf("default window = %v, want 24h", m.window)
	}
	m2 := NewMetricsHistorySource(nil, 2*time.Hour).(metricsHistorySource)
	if m2.window != 2*time.Hour {
		t.Errorf("window = %v, want 2h", m2.window)
	}
}

func TestServiceDefaults(t *testing.T) {
	rate := finops.CostRate{PerCoreMonth: 137, PerGBMonth: 9}
	svc := NewService(rate, nil)
	if svc.HasCollector() {
		t.Error("HasCollector should be false without a collector")
	}
	if got := svc.DefaultCostRate(); got != rate {
		t.Errorf("DefaultCostRate = %+v, want %+v", got, rate)
	}
	var nilSvc *Service
	if got := nilSvc.DefaultCostRate(); got != finops.DefaultCostRate() {
		t.Errorf("nil service DefaultCostRate = %+v, want defaults", got)
	}
	withCol := NewService(rate, NewCollector(&fakeLister{}, nil, nil))
	if !withCol.HasCollector() {
		t.Error("HasCollector should be true with a collector")
	}
}

type errLister struct{}

func (errLister) List(context.Context, int64, string) ([]json.RawMessage, error) {
	return nil, context.Canceled
}
