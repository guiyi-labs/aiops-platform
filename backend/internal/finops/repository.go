package finops

import (
	"context"
	"sync"

	k8sfinding "k8s-aiops.local/backend/internal/finding"
)

// Repository persists the latest WasteSummary per cluster. The in-memory
// implementation is sufficient for read-only advisory use; a SQL-backed
// implementation can be added later behind the same interface.
type Repository interface {
	Store(ctx context.Context, s WasteSummary) error
	Latest(ctx context.Context, clusterID int64) (WasteSummary, bool)
}

type memoryRepository struct {
	mu    sync.RWMutex
	byKey map[int64]WasteSummary
}

// NewMemoryRepository returns a concurrency-safe in-memory Repository.
func NewMemoryRepository() Repository {
	return &memoryRepository{byKey: make(map[int64]WasteSummary)}
}

func (m *memoryRepository) Store(_ context.Context, s WasteSummary) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byKey[s.ClusterID] = s
	return nil
}

func (m *memoryRepository) Latest(_ context.Context, clusterID int64) (WasteSummary, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.byKey[clusterID]
	return s, ok
}

// ensure interface compliance is checked by the compiler via NewService usage.
var _ = k8sfinding.SeverityCritical
