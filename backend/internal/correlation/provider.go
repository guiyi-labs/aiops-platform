package correlation

import (
	"context"
	"time"

	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/signal"
	"k8s-aiops.local/backend/internal/topology"
)

// Reader interfaces narrow the concrete gorm repositories to the queries the
// correlation provider needs. main wires *signal.GormRepository,
// *topology.GormRepository and *diagnosis.GormRepository directly; tests use
// fakes. Each read is bounded by the caller's scope (cluster, namespace) and
// by a per-source limit so a pass never scans unbounded history.
type signalReader interface {
	List(context.Context, signal.ListFilter) ([]signal.Occurrence, int64, error)
}

type topologyReader interface {
	ListEdges(context.Context, topology.EdgeFilter) ([]topology.Edge, int64, error)
	ListChangeEvents(context.Context, topology.ChangeTimelineFilter) ([]topology.ChangeEvent, int64, error)
}

type diagnosisReader interface {
	List(context.Context, diagnosis.ListFilter) ([]diagnosis.Record, error)
}

// Per-source read limits for one correlation pass. The engine is fed only the
// most recent rows per source; older rows are outside the correlation window.
const (
	providerSignalsLimit   = 200
	providerChangesLimit   = 100
	providerEdgesLimit     = 200
	providerDiagnosesLimit = 100
)

// RepositoryInputProvider implements InputProvider by reading the production
// signal/topology/diagnosis repositories and mapping their rows into the typed
// engine inputs.
type RepositoryInputProvider struct {
	signals signalReader
	graph   topologyReader
	diag    diagnosisReader
	now     func() time.Time
}

// NewRepositoryInputProvider constructs a provider from the three readers.
func NewRepositoryInputProvider(signals signalReader, graph topologyReader, diag diagnosisReader) *RepositoryInputProvider {
	return &RepositoryInputProvider{signals: signals, graph: graph, diag: diag, now: time.Now}
}

// ActiveSignals returns non-resolved signal occurrences observed within the
// lookback window. The engine never correlates resolved signals.
func (p *RepositoryInputProvider) ActiveSignals(ctx context.Context, clusterID int64, namespace string, lookback time.Duration) ([]SignalOccurrenceInput, error) {
	since := p.now().UTC().Add(-lookback)
	items, _, err := p.signals.List(ctx, signal.ListFilter{
		ClusterID:   &clusterID,
		Namespace:   namespace,
		State:       signal.StateActive,
		WindowStart: &since,
		Limit:       providerSignalsLimit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]SignalOccurrenceInput, 0, len(items))
	for _, occ := range items {
		out = append(out, toSignalInput(occ))
	}
	return out, nil
}

// RecentChanges returns change events started within the lookback window.
func (p *RepositoryInputProvider) RecentChanges(ctx context.Context, clusterID int64, namespace string, lookback time.Duration) ([]ChangeEventInput, error) {
	since := p.now().UTC().Add(-lookback)
	items, _, err := p.graph.ListChangeEvents(ctx, topology.ChangeTimelineFilter{
		ClusterID: clusterID,
		Namespace: namespace,
		StartTime: &since,
		Limit:     providerChangesLimit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ChangeEventInput, 0, len(items))
	for _, ev := range items {
		out = append(out, toChangeInput(ev))
	}
	return out, nil
}

// TopologyEdges returns the edges valid at the current time for the scope.
func (p *RepositoryInputProvider) TopologyEdges(ctx context.Context, clusterID int64, namespace string) ([]TopologyEdgeInput, error) {
	now := p.now().UTC()
	items, _, err := p.graph.ListEdges(ctx, topology.EdgeFilter{
		ClusterID: clusterID,
		Namespace: namespace,
		ValidAt:   &now,
		Limit:     providerEdgesLimit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]TopologyEdgeInput, 0, len(items))
	for _, edge := range items {
		out = append(out, toEdgeInput(edge))
	}
	return out, nil
}

// RecentDiagnoses returns diagnosis records observed within the lookback
// window.
func (p *RepositoryInputProvider) RecentDiagnoses(ctx context.Context, clusterID int64, namespace string, lookback time.Duration) ([]DiagnosisRef, error) {
	since := p.now().UTC().Add(-lookback)
	items, err := p.diag.List(ctx, diagnosis.ListFilter{
		ClusterID: clusterID,
		Since:     &since,
		Limit:     providerDiagnosesLimit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]DiagnosisRef, 0, len(items))
	for _, rec := range items {
		out = append(out, toDiagnosisRef(rec))
	}
	return out, nil
}

// --- mapping helpers ---

func toSignalInput(occ signal.Occurrence) SignalOccurrenceInput {
	return SignalOccurrenceInput{
		ID:          occ.ID,
		SignalID:    occ.SignalID,
		Producer:    string(occ.Producer),
		ClusterID:   occ.ClusterID,
		Namespace:   occ.Namespace,
		Resource:    citeResource(occ.Resource.Kind, occ.Resource.Namespace, occ.Resource.Name, occ.Resource.UID),
		Severity:    string(occ.Severity),
		State:       string(occ.State),
		Coverage:    string(occ.Coverage),
		Freshness:   occ.Freshness,
		WindowStart: occ.WindowStart,
		WindowEnd:   occ.WindowEnd,
		ObservedAt:  occ.ObservedAt,
		Evidence:    toSignalEvidence(occ.Evidence),
	}
}

func toChangeInput(ev topology.ChangeEvent) ChangeEventInput {
	return ChangeEventInput{
		ID:         ev.ID,
		ClusterID:  ev.ClusterID,
		Namespace:  ev.Namespace,
		Kind:       ev.Kind,
		PlanID:     ev.PlanID,
		Target:     citeResource(ev.Target.Kind, ev.Target.Namespace, ev.Target.Name, ev.Target.UID),
		Action:     ev.Action,
		Result:     ev.Result,
		Actor:      ev.Actor,
		StartedAt:  ev.StartedAt,
		FinishedAt: ev.FinishedAt,
		Evidence:   toTopologyEvidence(ev.Evidence),
		Confidence: ev.Confidence,
		Source:     ev.Source,
	}
}

func toEdgeInput(edge topology.Edge) TopologyEdgeInput {
	return TopologyEdgeInput{
		ID:        edge.ID,
		ClusterID: edge.ClusterID,
		Kind:      string(edge.Kind),
		Source:    citeResource(edge.Source.Kind, edge.Source.Namespace, edge.Source.Name, edge.Source.UID),
		Target:    citeResource(edge.Target.Kind, edge.Target.Namespace, edge.Target.Name, edge.Target.UID),
		ValidFrom: edge.ValidFrom,
		ValidTo:   edge.ValidTo,
	}
}

func toDiagnosisRef(rec diagnosis.Record) DiagnosisRef {
	return DiagnosisRef{
		ID:         rec.ID,
		ClusterID:  rec.ClusterID,
		RuleID:     rec.RuleID,
		Resource:   citeResource(rec.Resource.Kind, rec.Resource.Namespace, rec.Resource.Name, rec.Resource.UID),
		Severity:   rec.Severity,
		Status:     rec.Status,
		ObservedAt: rec.ObservedAt,
	}
}

func citeResource(kind, namespace, name, uid string) ResourceCitation {
	return ResourceCitation{
		Kind:       kind,
		Namespace:  namespace,
		Name:       name,
		UID:        uid,
		Incomplete: uid == "",
	}
}

func toSignalEvidence(items []signal.EvidenceRef) []EvidenceRef {
	if len(items) == 0 {
		return nil
	}
	out := make([]EvidenceRef, 0, len(items))
	for _, e := range items {
		out = append(out, EvidenceRef{Kind: e.Kind, ID: e.ID, ContentHash: e.ContentHash})
	}
	return out
}

func toTopologyEvidence(items []topology.EvidenceRef) []EvidenceRef {
	if len(items) == 0 {
		return nil
	}
	out := make([]EvidenceRef, 0, len(items))
	for _, e := range items {
		out = append(out, EvidenceRef{Kind: e.Kind, ID: e.ID, ContentHash: e.ContentHash})
	}
	return out
}
