package diagnosis

// Completes the service orchestration coverage: the knowledge-ingester option
// and its ingestion trigger, the no-match/error branches of every Diagnose*
// entry point (including the Ingress backend dedup loop) and the repository
// error paths of save/List/Assign.

import (
	"context"
	"errors"
	"testing"
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// --- fakes ---

// resolvingRepo mirrors memRepo but stamps ResolvedAt, which is what makes a
// record eligible for knowledge ingestion.
type resolvingRepo struct {
	*memRepo
	resolvedAt time.Time
}

func (r *resolvingRepo) Transition(ctx context.Context, id int64, status string, actor ActorRef, comment string) (Record, error) {
	record, err := r.memRepo.Transition(ctx, id, status, actor, comment)
	if err != nil {
		return Record{}, err
	}
	if status == "resolved" {
		resolved := r.resolvedAt
		record.ResolvedAt = &resolved
		r.records[id] = record
	}
	return record, nil
}

// recordingIngester (declared in ingest_test.go) is reused to observe what the
// service pushes into the knowledge base.

// failingRepo drives the repository error branches of the service layer.
type failingRepo struct {
	*memRepo
	saveErr       error
	listErr       error
	assignErr     error
	transitionErr error
	feedbackErr   error
}

func (f *failingRepo) Save(context.Context, *Record) error { return f.saveErr }
func (f *failingRepo) List(context.Context, ListFilter) ([]Record, error) {
	return nil, f.listErr
}
func (f *failingRepo) Assign(context.Context, int64, ActorRef, ActorRef, string) (Record, error) {
	return Record{}, f.assignErr
}
func (f *failingRepo) Transition(context.Context, int64, string, ActorRef, string) (Record, error) {
	return Record{}, f.transitionErr
}
func (f *failingRepo) AddFeedback(context.Context, int64, string, ActorRef, string) (Record, error) {
	return Record{}, f.feedbackErr
}

// selectiveSource fails individual source calls while leaving the rest usable,
// so each Diagnose* error branch can be reached in isolation.
type selectiveSource struct {
	*fakeSource
	eventsErr    error
	serviceErr   error
	endpointsErr error
	serviceCalls int
	endpointCall int
}

func (s *selectiveSource) PodEvents(ctx context.Context, clusterID int64, namespace, uid string) ([]k8sgateway.Event, error) {
	if s.eventsErr != nil {
		return nil, s.eventsErr
	}
	return s.fakeSource.PodEvents(ctx, clusterID, namespace, uid)
}

func (s *selectiveSource) ResourceEvents(ctx context.Context, clusterID int64, namespace, uid string) ([]k8sgateway.Event, error) {
	if s.eventsErr != nil {
		return nil, s.eventsErr
	}
	return s.fakeSource.ResourceEvents(ctx, clusterID, namespace, uid)
}

func (s *selectiveSource) GetService(ctx context.Context, clusterID int64, namespace, name string) (k8sgateway.ServiceResource, error) {
	s.serviceCalls++
	if s.serviceErr != nil {
		return k8sgateway.ServiceResource{}, s.serviceErr
	}
	return s.fakeSource.GetService(ctx, clusterID, namespace, name)
}

func (s *selectiveSource) ServiceEndpoints(ctx context.Context, clusterID int64, namespace, name string) (k8sgateway.Endpoints, error) {
	s.endpointCall++
	if s.endpointsErr != nil {
		return k8sgateway.Endpoints{}, s.endpointsErr
	}
	return s.fakeSource.ServiceEndpoints(ctx, clusterID, namespace, name)
}

// --- WithKnowledgeIngester ---

func TestServiceWithKnowledgeIngesterPushesResolvedRecord(t *testing.T) {
	src := &fakeSource{}
	mustDecode(t, `{"metadata":{"name":"worker-1"},"status":{"conditions":[]}}`, &src.node)
	resolvedAt := time.Date(2026, 7, 27, 9, 15, 0, 0, time.UTC)
	repo := &resolvingRepo{memRepo: newMemRepo(), resolvedAt: resolvedAt}
	ingester := &recordingIngester{}
	svc := NewService(src, repo).WithKnowledgeIngester(ingester)
	ctx := context.Background()

	record, err := svc.DiagnoseNode(ctx, 7, "worker-1")
	if err != nil {
		t.Fatalf("DiagnoseNode: %v", err)
	}
	actor := ActorRef{ID: 1, Name: "ops"}

	// A non-resolved transition must not touch the knowledge base.
	if _, err := svc.Transition(ctx, record.ID, "confirmed", actor, "triaged"); err != nil {
		t.Fatalf("Transition: %v", err)
	}
	if len(ingester.calls) != 0 {
		t.Fatalf("ingested %d entries on a confirmed record", len(ingester.calls))
	}

	if _, err := svc.Transition(ctx, record.ID, "resolved", actor, "fixed"); err != nil {
		t.Fatalf("Transition to resolved: %v", err)
	}
	if len(ingester.calls) != 1 {
		t.Fatalf("ingested %d entries, want 1", len(ingester.calls))
	}
	got := ingester.calls[0]
	if got.SourceDiagnosisID != record.ID {
		t.Fatalf("source diagnosis id = %d, want %d", got.SourceDiagnosisID, record.ID)
	}
	if got.RuleID != RuleNodeNotReady || got.Severity != "critical" {
		t.Fatalf("rule/severity = %q / %q", got.RuleID, got.Severity)
	}
	if got.ResourceKind != "Node" || got.ResourceName != "worker-1" {
		t.Fatalf("resource = %q / %q", got.ResourceKind, got.ResourceName)
	}
	if len(got.RootCauses) == 0 || len(got.Recommendations) == 0 {
		t.Fatalf("distilled content = %#v / %#v", got.RootCauses, got.Recommendations)
	}
	if !got.NotedAt.Equal(resolvedAt) {
		t.Fatalf("noted at = %v, want %v", got.NotedAt, resolvedAt)
	}
}

func TestServiceTransitionWithoutIngesterIsUnchanged(t *testing.T) {
	svc, src, _ := newSvc(t)
	mustDecode(t, `{"metadata":{"name":"worker-1"},"status":{"conditions":[]}}`, &src.node)
	ctx := context.Background()

	record, err := svc.DiagnoseNode(ctx, 7, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	// The option must be nil-safe: a service without an ingester still
	// transitions normally.
	if got := svc.WithKnowledgeIngester(nil); got != svc {
		t.Fatal("WithKnowledgeIngester must return the same service for chaining")
	}
	resolved, err := svc.Transition(ctx, record.ID, "confirmed", ActorRef{ID: 1, Name: "ops"}, "")
	if err != nil || resolved.Status != "confirmed" {
		t.Fatalf("record = %#v, err = %v", resolved, err)
	}
}

// --- DiagnoseService ---

func TestDiagnoseServiceErrorPaths(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("service gateway down")
	serviceJSON := `{"metadata":{"name":"api","namespace":"demo","uid":"service-1"},"spec":{"type":"ClusterIP","selector":{"app":"api"},"ports":[{"port":80,"targetPort":8080}]}}`

	t.Run("service fetch error", func(t *testing.T) {
		src := &fakeSource{err: boom}
		if _, err := NewService(src, newMemRepo()).DiagnoseService(ctx, 7, "demo", "api"); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want %v", err, boom)
		}
	})

	t.Run("endpoints fetch error", func(t *testing.T) {
		src := &selectiveSource{fakeSource: &fakeSource{}, endpointsErr: boom}
		mustDecode(t, serviceJSON, &src.service)
		if _, err := NewService(src, newMemRepo()).DiagnoseService(ctx, 7, "demo", "api"); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want %v", err, boom)
		}
	})
}

// --- DiagnoseNode / DiagnosePod ---

func TestDiagnoseNodeNoMatchAndSourceError(t *testing.T) {
	svc, src, _ := newSvc(t)
	ctx := context.Background()

	// A Ready node without pressure conditions matches no rule.
	mustDecode(t, `{"metadata":{"name":"worker-1"},"status":{"conditions":[{"type":"Ready","status":"True"}]}}`, &src.node)
	if _, err := svc.DiagnoseNode(ctx, 7, "worker-1"); !errors.Is(err, ErrNoRuleMatch) {
		t.Fatalf("healthy node err = %v, want ErrNoRuleMatch", err)
	}

	src.err = errors.New("node gateway down")
	if _, err := svc.DiagnoseNode(ctx, 7, "worker-1"); err == nil || err.Error() != "node gateway down" {
		t.Fatalf("source error = %v, want propagation", err)
	}
}

func TestDiagnosePodEventFetchError(t *testing.T) {
	boom := errors.New("events gateway down")
	src := &selectiveSource{fakeSource: &fakeSource{}, eventsErr: boom}
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo","uid":"pod-1"},"status":{"phase":"Running"}}`, &src.pod)
	svc := NewService(src, newMemRepo())

	if _, err := svc.DiagnosePod(context.Background(), 7, "demo", "api"); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

// --- DiagnosePersistentVolumeClaim ---

func TestDiagnosePersistentVolumeClaimErrorPaths(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("gateway down")

	t.Run("claim fetch error", func(t *testing.T) {
		src := &fakeSource{err: boom}
		svc := NewService(src, newMemRepo())
		if _, err := svc.DiagnosePersistentVolumeClaim(ctx, 7, "demo", "data"); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want %v", err, boom)
		}
	})

	t.Run("event fetch error", func(t *testing.T) {
		src := &selectiveSource{fakeSource: &fakeSource{}, eventsErr: boom}
		mustDecode(t, `{"metadata":{"name":"data","namespace":"demo","uid":"pvc-1"},"status":{"phase":"Pending"}}`, &src.pvc)
		svc := NewService(src, newMemRepo())
		if _, err := svc.DiagnosePersistentVolumeClaim(ctx, 7, "demo", "data"); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want %v", err, boom)
		}
	})

	t.Run("no rule match", func(t *testing.T) {
		src := &fakeSource{}
		// Bound claim, and a Pending claim without Warning events, both abstain.
		mustDecode(t, `{"metadata":{"name":"data","namespace":"demo","uid":"pvc-1"},"status":{"phase":"Bound"}}`, &src.pvc)
		svc := NewService(src, newMemRepo())
		if _, err := svc.DiagnosePersistentVolumeClaim(ctx, 7, "demo", "data"); !errors.Is(err, ErrNoRuleMatch) {
			t.Fatalf("bound claim err = %v, want ErrNoRuleMatch", err)
		}

		mustDecode(t, `{"metadata":{"name":"data","namespace":"demo","uid":"pvc-1"},"status":{"phase":"Pending"}}`, &src.pvc)
		src.events = []k8sgateway.Event{{Type: "Normal", Reason: "Provisioning", Message: "waiting"}}
		if _, err := svc.DiagnosePersistentVolumeClaim(ctx, 7, "demo", "data"); !errors.Is(err, ErrNoRuleMatch) {
			t.Fatalf("eventless pending claim err = %v, want ErrNoRuleMatch", err)
		}
	})
}

// --- DiagnoseHorizontalPodAutoscaler ---

func TestDiagnoseHorizontalPodAutoscalerErrorPaths(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("hpa gateway down")

	src := &fakeSource{err: boom}
	svc := NewService(src, newMemRepo())
	if _, err := svc.DiagnoseHorizontalPodAutoscaler(ctx, 7, "demo", "api"); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}

	// At maxReplicas without the TooManyReplicas condition: no rule matches.
	src = &fakeSource{}
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo"},"spec":{"maxReplicas":4},"status":{"currentReplicas":4,"desiredReplicas":4}}`, &src.hpa)
	svc = NewService(src, newMemRepo())
	if _, err := svc.DiagnoseHorizontalPodAutoscaler(ctx, 7, "demo", "api"); !errors.Is(err, ErrNoRuleMatch) {
		t.Fatalf("unlimited hpa err = %v, want ErrNoRuleMatch", err)
	}
}

// --- DiagnoseIngress ---

func TestDiagnoseIngressDeduplicatesBackendLookups(t *testing.T) {
	// The same Service is referenced by the default backend and by a rule path;
	// the source must only be consulted once per distinct Service name.
	src := &selectiveSource{fakeSource: &fakeSource{}}
	mustDecode(t, `{"metadata":{"name":"broken","namespace":"demo","uid":"ingress-1"},"spec":{"defaultBackend":{"service":{"name":"api","port":{"number":80}}},"rules":[{"host":"broken.example.test","http":{"paths":[{"path":"/api","pathType":"Prefix","backend":{"service":{"name":"api","port":{"number":80}}}}]}}]}}`, &src.ingress)
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo","uid":"service-1"},"spec":{"type":"ClusterIP","selector":{"app":"api"},"ports":[{"port":80,"targetPort":8080}]}}`, &src.service)
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo"},"subsets":[{"notReadyAddresses":[{"ip":"10.0.0.8"}]}]}`, &src.endpoints)

	svc := NewService(src, newMemRepo())
	record, err := svc.DiagnoseIngress(context.Background(), 7, "demo", "broken")
	if err != nil || record.RuleID != RuleIngressBackendUnavailable {
		t.Fatalf("record = %#v, err = %v", record, err)
	}
	if src.serviceCalls != 1 || src.endpointCall != 1 {
		t.Fatalf("source calls = %d/%d, want 1/1", src.serviceCalls, src.endpointCall)
	}
	// Both routes resolve to the same backend, so both are reported.
	if len(record.Evidence) != 2 {
		t.Fatalf("evidence = %#v", record.Evidence)
	}
}

func TestDiagnoseIngressErrorPaths(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("gateway down")
	ingressJSON := `{"metadata":{"name":"broken","namespace":"demo","uid":"ingress-1"},"spec":{"rules":[{"host":"broken.example.test","http":{"paths":[{"path":"/api","pathType":"Prefix","backend":{"service":{"name":"api","port":{"number":80}}}}]}}]}}`
	serviceJSON := `{"metadata":{"name":"api","namespace":"demo","uid":"service-1"},"spec":{"type":"ClusterIP","selector":{"app":"api"},"ports":[{"port":80,"targetPort":8080}]}}`

	t.Run("ingress fetch error", func(t *testing.T) {
		src := &fakeSource{err: boom}
		svc := NewService(src, newMemRepo())
		if _, err := svc.DiagnoseIngress(ctx, 7, "demo", "broken"); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want %v", err, boom)
		}
	})

	t.Run("backend service fetch error", func(t *testing.T) {
		src := &selectiveSource{fakeSource: &fakeSource{}, serviceErr: boom}
		mustDecode(t, ingressJSON, &src.ingress)
		svc := NewService(src, newMemRepo())
		if _, err := svc.DiagnoseIngress(ctx, 7, "demo", "broken"); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want %v", err, boom)
		}
	})

	t.Run("backend endpoints fetch error", func(t *testing.T) {
		src := &selectiveSource{fakeSource: &fakeSource{}, endpointsErr: boom}
		mustDecode(t, ingressJSON, &src.ingress)
		mustDecode(t, serviceJSON, &src.service)
		svc := NewService(src, newMemRepo())
		if _, err := svc.DiagnoseIngress(ctx, 7, "demo", "broken"); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want %v", err, boom)
		}
	})

	t.Run("no rule match", func(t *testing.T) {
		src := &fakeSource{}
		mustDecode(t, ingressJSON, &src.ingress)
		mustDecode(t, serviceJSON, &src.service)
		mustDecode(t, `{"metadata":{"name":"api","namespace":"demo"},"subsets":[{"addresses":[{"ip":"10.0.0.8"}]}]}`, &src.endpoints)
		svc := NewService(src, newMemRepo())
		if _, err := svc.DiagnoseIngress(ctx, 7, "demo", "broken"); !errors.Is(err, ErrNoRuleMatch) {
			t.Fatalf("ready backend err = %v, want ErrNoRuleMatch", err)
		}
	})

	t.Run("ingress without routes", func(t *testing.T) {
		src := &fakeSource{}
		mustDecode(t, `{"metadata":{"name":"empty","namespace":"demo"}}`, &src.ingress)
		svc := NewService(src, newMemRepo())
		if _, err := svc.DiagnoseIngress(ctx, 7, "demo", "empty"); !errors.Is(err, ErrNoRuleMatch) {
			t.Fatalf("routeless ingress err = %v, want ErrNoRuleMatch", err)
		}
	})
}

// --- DiagnoseDeployment ---

func TestDiagnoseDeploymentErrorPaths(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("deployment gateway down")

	src := &fakeSource{err: boom}
	svc := NewService(src, newMemRepo())
	if _, err := svc.DiagnoseDeployment(ctx, 7, "demo", "api"); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}

	// A fully available Deployment matches no rule.
	src = &fakeSource{}
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo"},"spec":{"replicas":3},"status":{"replicas":3,"readyReplicas":3,"availableReplicas":3,"updatedReplicas":3,"unavailableReplicas":0}}`, &src.deployment)
	svc = NewService(src, newMemRepo())
	if _, err := svc.DiagnoseDeployment(ctx, 7, "demo", "api"); !errors.Is(err, ErrNoRuleMatch) {
		t.Fatalf("healthy deployment err = %v, want ErrNoRuleMatch", err)
	}
}

// --- repository error propagation ---

func TestServicePropagatesRepositoryErrors(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("repository exploded")
	svc, src, _ := newSvc(t)
	mustDecode(t, `{"metadata":{"name":"worker-1"},"status":{"conditions":[]}}`, &src.node)
	failing := &failingRepo{
		memRepo: newMemRepo(), saveErr: boom, listErr: boom, assignErr: boom,
		transitionErr: boom, feedbackErr: boom,
	}

	t.Run("save", func(t *testing.T) {
		broken := NewService(src, failing)
		if _, err := broken.DiagnoseNode(ctx, 7, "worker-1"); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want %v", err, boom)
		}
	})

	t.Run("list", func(t *testing.T) {
		if _, err := svc.List(ctx, ListFilter{Limit: 5}); err != nil {
			t.Fatalf("healthy list err = %v", err)
		}
		if _, err := NewService(src, failing).List(ctx, ListFilter{Limit: 5}); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want %v", err, boom)
		}
	})

	t.Run("assign", func(t *testing.T) {
		if _, err := NewService(src, failing).Assign(ctx, 1, ActorRef{ID: 2, Name: "sre"}, ActorRef{ID: 1, Name: "ops"}, ""); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want %v", err, boom)
		}
	})

	t.Run("transition", func(t *testing.T) {
		if _, err := NewService(src, failing).Transition(ctx, 1, "confirmed", ActorRef{ID: 1, Name: "ops"}, ""); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want %v", err, boom)
		}
	})

	t.Run("add feedback", func(t *testing.T) {
		broken := NewService(src, failing)
		// An invalid verdict is rejected before the repository is consulted.
		if _, err := broken.AddFeedback(ctx, 1, "bogus", ActorRef{ID: 1, Name: "ops"}, ""); !errors.Is(err, ErrInvalidFeedback) {
			t.Fatalf("err = %v, want ErrInvalidFeedback", err)
		}
		if _, err := broken.AddFeedback(ctx, 1, "accurate", ActorRef{ID: 1, Name: "ops"}, ""); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want %v", err, boom)
		}
	})

	t.Run("get", func(t *testing.T) {
		if _, err := svc.Get(ctx, 9999); !errors.Is(err, ErrRecordNotFound) {
			t.Fatalf("err = %v, want ErrRecordNotFound", err)
		}
	})
}
