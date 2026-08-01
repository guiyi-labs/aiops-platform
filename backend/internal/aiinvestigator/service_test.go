package aiinvestigator

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRepository is an in-memory Repository for service tests.
type fakeRepository struct {
	mu         sync.Mutex
	nextID     int64
	saved      []Investigation
	saveErr    error
	getErr     error
	listItems  []Investigation
	listTotal  int64
	listErr    error
	staleCalls []struct {
		caseID int64
		key    string
		except int64
	}
}

func (r *fakeRepository) Save(_ context.Context, inv *Investigation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.saveErr != nil {
		return r.saveErr
	}
	r.nextID++
	inv.ID = r.nextID
	r.saved = append(r.saved, *inv)
	// Mirror MarkStale: nothing to do for in-memory beyond tracking.
	return nil
}

func (r *fakeRepository) Get(_ context.Context, id int64) (Investigation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.getErr != nil {
		return Investigation{}, r.getErr
	}
	for _, inv := range r.saved {
		if inv.ID == id {
			return inv, nil
		}
	}
	return Investigation{}, ErrInvestigationNotFound
}

func (r *fakeRepository) ListByCase(_ context.Context, caseID int64, limit int) ([]Investigation, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.listErr != nil {
		return nil, 0, r.listErr
	}
	if r.listItems != nil || r.listTotal != 0 {
		return r.listItems, r.listTotal, nil
	}
	var items []Investigation
	for _, inv := range r.saved {
		if inv.CaseID == caseID {
			items = append(items, inv)
		}
	}
	return items, int64(len(items)), nil
}

func (r *fakeRepository) ListByFilter(_ context.Context, filter InvestigationFilter) ([]Investigation, int64, error) {
	return nil, 0, nil
}

func (r *fakeRepository) MarkStale(_ context.Context, caseID int64, key string, exceptID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.staleCalls = append(r.staleCalls, struct {
		caseID int64
		key    string
		except int64
	}{caseID, key, exceptID})
	return nil
}

// fakeCaseReader returns a fixed CaseContext and eligible codes.
type fakeCaseReader struct {
	ctx      CaseContext
	codes    map[string]bool
	getErr   error
	codesErr error
}

func (r fakeCaseReader) GetCase(_ context.Context, _ int64) (CaseContext, error) {
	if r.getErr != nil {
		return CaseContext{}, r.getErr
	}
	return r.ctx, nil
}

func (r fakeCaseReader) EligibleActionCodes(_ context.Context, _ int64) (map[string]bool, error) {
	if r.codesErr != nil {
		return nil, r.codesErr
	}
	return r.codes, nil
}

// stubProvider returns a fixed result or error.
type stubProvider struct {
	result ProviderResult
	err    error
	calls  int
}

func (p *stubProvider) Generate(_ context.Context, _ Prompt) (ProviderResult, error) {
	p.calls++
	if p.err != nil {
		return ProviderResult{}, p.err
	}
	return p.result, nil
}

func validProviderResult() ProviderResult {
	caseRef := EvidenceRef{Kind: EvidenceKindCorrelationCase, ID: 42}
	signalRef := EvidenceRef{Kind: EvidenceKindSignalOccurrence, ID: 100}
	changeRef := EvidenceRef{Kind: EvidenceKindChangeCandidate, ID: 200}
	return ProviderResult{
		Provider: "stub",
		Model:    "stub-1.0",
		Summary:  "Rollout introduced a bad image tag.",
		Impact:   "Service web is returning 5xx errors.",
		Hypotheses: []Hypothesis{{
			Claim:       "The rollout introduced a bad image tag.",
			Confidence:  HypothesisHigh,
			EvidenceIDs: []EvidenceRef{caseRef, changeRef, signalRef},
		}},
		RecommendedRunbookID: "rollback_last_rollout",
		Citations: []Citation{
			{EvidenceRef: caseRef, Claim: "case exists"},
			{EvidenceRef: signalRef, Claim: "crash-loop signal observed"},
			{EvidenceRef: changeRef, Claim: "rollout preceded the signal"},
		},
	}
}

func serviceCaseContext() CaseContext {
	return CaseContext{
		CaseID:               42,
		ClusterID:            1,
		RuleID:               "pod_failure_with_rollout",
		PrimaryResourceKind:  "Pod",
		PrimaryResourceName:  "web-abc",
		PrimaryResourceUID:   "uid-123",
		Confidence:           "candidate",
		EvidenceCompleteness: "partial",
		SignalLinks: []SignalLinkContext{
			{SignalOccurrenceID: 100, Relation: "trigger", SignalID: "pod_crashloop", Producer: "diagnosis", ObservedAt: "2026-07-31T10:00:00Z"},
		},
		ChangeCandidates: []ChangeCandidateContext{
			{ChangeEventID: 200, RuleID: "rollout_preceded", Confidence: "candidate", Rank: 1, ReasonCode: "temporal_proximity"},
		},
	}
}

func TestInvestigateSuccess(t *testing.T) {
	repo := &fakeRepository{}
	reader := fakeCaseReader{
		ctx:   serviceCaseContext(),
		codes: map[string]bool{"deployment.rollback": true},
	}
	provider := &stubProvider{result: validProviderResult()}
	fixedNow := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	svc := NewService(repo, provider, reader, WithNow(func() time.Time { return fixedNow }))

	inv, err := svc.Investigate(context.Background(), 42, ActorRef{ID: 1, Name: "alice"})
	if err != nil {
		t.Fatalf("Investigate failed: %v", err)
	}
	if inv.Status != InvestigationStatusCompleted {
		t.Errorf("Status = %q, want completed", inv.Status)
	}
	if inv.CaseID != 42 {
		t.Errorf("CaseID = %d, want 42", inv.CaseID)
	}
	if inv.InvestigatorVersion != InvestigatorVersion {
		t.Errorf("InvestigatorVersion = %q, want %q", inv.InvestigatorVersion, InvestigatorVersion)
	}
	if inv.Provider != "stub" {
		t.Errorf("Provider = %q, want stub", inv.Provider)
	}
	if inv.RecommendedRunbookID != "rollback_last_rollout" {
		t.Errorf("RunbookID = %q, want rollback_last_rollout", inv.RecommendedRunbookID)
	}
	if inv.CreatedAt != fixedNow {
		t.Errorf("CreatedAt = %v, want %v", inv.CreatedAt, fixedNow)
	}
	if len(inv.Citations) != 3 {
		t.Errorf("Citations = %d, want 3", len(inv.Citations))
	}
	if inv.InvestigationKey == "" || len(inv.InvestigationKey) != 64 {
		t.Errorf("InvestigationKey should be a 64-char SHA256 hex, got %q", inv.InvestigationKey)
	}
	if inv.ID == 0 {
		t.Errorf("ID should be set by repository Save")
	}
	if provider.calls != 1 {
		t.Errorf("provider should be called once, got %d", provider.calls)
	}
	if len(repo.saved) != 1 {
		t.Errorf("repository should have one saved investigation, got %d", len(repo.saved))
	}
}

func TestInvestigateCaseNotFound(t *testing.T) {
	repo := &fakeRepository{}
	reader := fakeCaseReader{getErr: ErrCaseNotFound}
	svc := NewService(repo, &stubProvider{}, reader)
	_, err := svc.Investigate(context.Background(), 999, ActorRef{ID: 1, Name: "alice"})
	if !errors.Is(err, ErrCaseNotFound) {
		t.Errorf("expected ErrCaseNotFound, got %v", err)
	}
	if len(repo.saved) != 0 {
		t.Errorf("no investigation should be persisted when case is not found")
	}
}

func TestInvestigateProviderFailurePersistsFailed(t *testing.T) {
	repo := &fakeRepository{}
	reader := fakeCaseReader{
		ctx:   serviceCaseContext(),
		codes: map[string]bool{},
	}
	providerErr := errors.New("model unavailable")
	provider := &stubProvider{err: providerErr}
	fixedNow := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	svc := NewService(repo, provider, reader, WithNow(func() time.Time { return fixedNow }))

	inv, err := svc.Investigate(context.Background(), 42, ActorRef{ID: 1, Name: "alice"})
	if !errors.Is(err, providerErr) {
		t.Errorf("expected provider error to propagate, got %v", err)
	}
	// A failed investigation must still be persisted.
	if inv.Status != InvestigationStatusFailed {
		t.Errorf("Status = %q, want failed", inv.Status)
	}
	if inv.FailureReason != "provider_error" {
		t.Errorf("FailureReason = %q, want provider_error", inv.FailureReason)
	}
	if inv.Provider != "unknown" {
		t.Errorf("Provider = %q, want unknown on failure", inv.Provider)
	}
	if len(repo.saved) != 1 {
		t.Errorf("failed investigation should be persisted, got %d saved", len(repo.saved))
	}
	if repo.saved[0].Status != InvestigationStatusFailed {
		t.Errorf("persisted investigation should be failed")
	}
}

func TestInvestigateCitationRejectionPersistsFailed(t *testing.T) {
	repo := &fakeRepository{}
	reader := fakeCaseReader{
		ctx:   serviceCaseContext(),
		codes: map[string]bool{},
	}
	// Provider returns a result that cites unauthorized evidence.
	bad := validProviderResult()
	bad.Citations = append(bad.Citations, Citation{
		EvidenceRef: EvidenceRef{Kind: EvidenceKindSignalOccurrence, ID: 999}, // not authorized
		Claim:       "fabricated",
	})
	provider := &stubProvider{result: bad}
	svc := NewService(repo, provider, reader)

	inv, err := svc.Investigate(context.Background(), 42, ActorRef{ID: 1, Name: "alice"})
	if err == nil {
		t.Fatalf("expected citation rejection error")
	}
	if inv.Status != InvestigationStatusFailed {
		t.Errorf("Status = %q, want failed", inv.Status)
	}
	if inv.FailureReason != "citation_rejected" {
		t.Errorf("FailureReason = %q, want citation_rejected", inv.FailureReason)
	}
	// The provider's summary/hypotheses are still persisted for audit.
	if inv.Summary == "" {
		t.Errorf("failed investigation should retain the provider summary for audit")
	}
	if len(repo.saved) != 1 {
		t.Errorf("failed investigation should be persisted")
	}
}

func TestInvestigateIneligibleRunbookPersistsFailed(t *testing.T) {
	repo := &fakeRepository{}
	// No eligible action codes.
	reader := fakeCaseReader{
		ctx:   serviceCaseContext(),
		codes: map[string]bool{},
	}
	// Provider recommends a rollback runbook but deployment.rollback is not
	// eligible for this case.
	bad := validProviderResult()
	provider := &stubProvider{result: bad}
	svc := NewService(repo, provider, reader)

	inv, err := svc.Investigate(context.Background(), 42, ActorRef{ID: 1, Name: "alice"})
	if err == nil {
		t.Fatalf("expected runbook eligibility error")
	}
	if inv.Status != InvestigationStatusFailed {
		t.Errorf("Status = %q, want failed", inv.Status)
	}
	if inv.FailureReason != "citation_rejected" {
		t.Errorf("FailureReason = %q, want citation_rejected (runbook rejection is a citation rejection)", inv.FailureReason)
	}
}

func TestInvestigateNilProviderUsesNopProvider(t *testing.T) {
	// nil provider → NopProvider, service enabled. NopProvider produces a
	// deterministic, citation-valid result. The service is enabled by
	// default (matching the automation service pattern); there is no
	// "disabled" state when constructed with a repository.
	repo := &fakeRepository{}
	reader := fakeCaseReader{ctx: serviceCaseContext()}
	svc := NewService(repo, nil, reader)
	inv, err := svc.Investigate(context.Background(), 42, ActorRef{ID: 1, Name: "alice"})
	if err != nil {
		t.Fatalf("expected success with NopProvider, got error: %v", err)
	}
	if inv.Status != InvestigationStatusCompleted {
		t.Errorf("Status = %q, want completed", inv.Status)
	}
	if len(repo.saved) != 1 {
		t.Errorf("expected 1 investigation persisted, got %d", len(repo.saved))
	}
}

func TestInvestigateExplicitNopProviderProducesValidResult(t *testing.T) {
	// NopProvider can also be passed explicitly; behavior is identical to
	// nil provider → NopProvider default.
	repo := &fakeRepository{}
	reader := fakeCaseReader{
		ctx:   serviceCaseContext(),
		codes: map[string]bool{},
	}
	svc := NewService(repo, NopProvider{}, reader)
	inv, err := svc.Investigate(context.Background(), 42, ActorRef{ID: 1, Name: "alice"})
	if err != nil {
		t.Fatalf("Investigate with NopProvider failed: %v", err)
	}
	if inv.Status != InvestigationStatusCompleted {
		t.Errorf("Status = %q, want completed", inv.Status)
	}
	if inv.Provider != "nop" {
		t.Errorf("Provider = %q, want nop", inv.Provider)
	}
	if len(inv.Citations) == 0 {
		t.Errorf("nop result must have citations")
	}
}

func TestInvestigateInvestigationKeyDeterministic(t *testing.T) {
	reader := fakeCaseReader{
		ctx:   serviceCaseContext(),
		codes: map[string]bool{"deployment.rollback": true},
	}
	provider := &stubProvider{result: validProviderResult()}
	fixedNow := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	svc1 := NewService(&fakeRepository{}, provider, reader, WithNow(func() time.Time { return fixedNow }))
	svc2 := NewService(&fakeRepository{}, provider, reader, WithNow(func() time.Time { return fixedNow }))
	inv1, err := svc1.Investigate(context.Background(), 42, ActorRef{ID: 1, Name: "alice"})
	if err != nil {
		t.Fatalf("svc1 Investigate failed: %v", err)
	}
	inv2, err := svc2.Investigate(context.Background(), 42, ActorRef{ID: 2, Name: "bob"})
	if err != nil {
		t.Fatalf("svc2 Investigate failed: %v", err)
	}
	// Identical case context + investigator version must produce identical
	// investigation keys, regardless of actor.
	if inv1.InvestigationKey != inv2.InvestigationKey {
		t.Errorf("InvestigationKey must be deterministic: %q vs %q", inv1.InvestigationKey, inv2.InvestigationKey)
	}
}

func TestGetInvestigation(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		repo := &fakeRepository{}
		repo.nextID = 7
		repo.saved = []Investigation{{ID: 7, CaseID: 42, Summary: "x"}}
		svc := NewService(repo, nil, nil)
		inv, err := svc.GetInvestigation(context.Background(), 7)
		if err != nil {
			t.Fatalf("GetInvestigation failed: %v", err)
		}
		if inv.ID != 7 {
			t.Errorf("ID = %d, want 7", inv.ID)
		}
	})
	t.Run("not found", func(t *testing.T) {
		svc := NewService(&fakeRepository{}, nil, nil)
		_, err := svc.GetInvestigation(context.Background(), 99)
		if !errors.Is(err, ErrInvestigationNotFound) {
			t.Errorf("expected ErrInvestigationNotFound, got %v", err)
		}
	})
}

func TestListByCase(t *testing.T) {
	t.Run("returns items and total", func(t *testing.T) {
		items := []Investigation{{ID: 1}, {ID: 2}}
		repo := &fakeRepository{listItems: items, listTotal: 5}
		svc := NewService(repo, nil, nil)
		resp, err := svc.ListByCase(context.Background(), 42, 10)
		if err != nil {
			t.Fatalf("ListByCase failed: %v", err)
		}
		if len(resp.Items) != 2 {
			t.Errorf("Items = %d, want 2", len(resp.Items))
		}
		if resp.Total != 5 {
			t.Errorf("Total = %d, want 5", resp.Total)
		}
		if !resp.Truncated {
			t.Errorf("Truncated should be true when len(items) < total")
		}
	})
	t.Run("not truncated when complete", func(t *testing.T) {
		items := []Investigation{{ID: 1}}
		repo := &fakeRepository{listItems: items, listTotal: 1}
		svc := NewService(repo, nil, nil)
		resp, _ := svc.ListByCase(context.Background(), 42, 10)
		if resp.Truncated {
			t.Errorf("Truncated should be false when len(items) == total")
		}
	})
}

func TestListRunbooks(t *testing.T) {
	svc := NewService(&fakeRepository{}, nil, nil)
	runbooks := svc.ListRunbooks()
	if len(runbooks) == 0 {
		t.Fatalf("ListRunbooks should return the catalog")
	}
	// Must match AllRunbooks.
	if len(runbooks) != len(AllRunbooks()) {
		t.Errorf("ListRunbooks count = %d, want %d", len(runbooks), len(AllRunbooks()))
	}
}

func TestComputeInvestigationKey(t *testing.T) {
	ctx := serviceCaseContext()
	key := computeInvestigationKey(42, ctx)
	if len(key) != 64 {
		t.Errorf("key should be 64 hex chars, got %d", len(key))
	}
	// Stable across calls.
	key2 := computeInvestigationKey(42, ctx)
	if key != key2 {
		t.Errorf("investigation key must be stable")
	}
	// Different case id produces different key.
	key3 := computeInvestigationKey(43, ctx)
	if key == key3 {
		t.Errorf("different case id should produce different key")
	}
}

func TestNewServiceDefaults(t *testing.T) {
	t.Run("nil provider becomes NopProvider", func(t *testing.T) {
		svc := NewService(&fakeRepository{}, nil, nil)
		if svc.provider == nil {
			t.Errorf("provider should default to NopProvider, not nil")
		}
		if _, ok := svc.provider.(NopProvider); !ok {
			t.Errorf("provider should be NopProvider")
		}
	})
	t.Run("nil reader becomes NopCaseReader", func(t *testing.T) {
		svc := NewService(&fakeRepository{}, nil, nil)
		if svc.reader == nil {
			t.Errorf("reader should default to NopCaseReader, not nil")
		}
		if _, ok := svc.reader.(NopCaseReader); !ok {
			t.Errorf("reader should be NopCaseReader")
		}
	})
	t.Run("enabled by default", func(t *testing.T) {
		svc := NewService(&fakeRepository{}, nil, nil)
		if !svc.enabled {
			t.Errorf("service should be enabled by default")
		}
	})
}

func TestNopCaseReader(t *testing.T) {
	reader := NopCaseReader{}
	_, err := reader.GetCase(context.Background(), 1)
	if !errors.Is(err, ErrCaseNotFound) {
		t.Errorf("NopCaseReader.GetCase should return ErrCaseNotFound, got %v", err)
	}
	codes, err := reader.EligibleActionCodes(context.Background(), 1)
	if err != nil {
		t.Errorf("EligibleActionCodes should not error: %v", err)
	}
	if codes != nil {
		t.Errorf("NopCaseReader should return nil codes")
	}
}

func TestNopRepository(t *testing.T) {
	repo := NopRepository{}
	if err := repo.Save(context.Background(), &Investigation{}); err != nil {
		t.Errorf("NopRepository.Save should not error: %v", err)
	}
	_, err := repo.Get(context.Background(), 1)
	if !errors.Is(err, ErrInvestigationNotFound) {
		t.Errorf("NopRepository.Get should return ErrInvestigationNotFound, got %v", err)
	}
	items, total, err := repo.ListByCase(context.Background(), 1, 10)
	if err != nil {
		t.Errorf("NopRepository.ListByCase should not error: %v", err)
	}
	if total != 0 || items != nil {
		t.Errorf("NopRepository.ListByCase should return empty")
	}
}

func TestInvestigateEligibleActionCodesError(t *testing.T) {
	repo := &fakeRepository{}
	reader := fakeCaseReader{
		ctx:      serviceCaseContext(),
		codesErr: errors.New("action catalog unavailable"),
	}
	svc := NewService(repo, &stubProvider{}, reader)
	_, err := svc.Investigate(context.Background(), 42, ActorRef{ID: 1, Name: "alice"})
	if err == nil {
		t.Fatalf("expected error from EligibleActionCodes")
	}
	if !strings.Contains(err.Error(), "action catalog unavailable") {
		t.Errorf("error should propagate codes error, got: %v", err)
	}
	if len(repo.saved) != 0 {
		t.Errorf("no investigation should be persisted when codes fail")
	}
}

func TestInvestigateNilEligibleCodesTreatedAsEmpty(t *testing.T) {
	// When EligibleActionCodes returns nil (no error), the service treats it
	// as an empty map. Advisory runbooks remain eligible.
	repo := &fakeRepository{}
	reader := fakeCaseReader{
		ctx:   serviceCaseContext(),
		codes: nil,
	}
	// Provider recommends an advisory runbook (always eligible).
	result := validProviderResult()
	result.RecommendedRunbookID = "inspect_pvc_capacity"
	provider := &stubProvider{result: result}
	svc := NewService(repo, provider, reader)
	inv, err := svc.Investigate(context.Background(), 42, ActorRef{ID: 1, Name: "alice"})
	if err != nil {
		t.Fatalf("Investigate failed: %v", err)
	}
	if inv.Status != InvestigationStatusCompleted {
		t.Errorf("Status = %q, want completed (advisory runbook eligible)", inv.Status)
	}
}
