package finops

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
)

// Service orchestrates the right-sizing advisor over a set of container
// inputs and (optionally) persists the result. It is read-only: it never
// writes back to the cluster.
type Service struct {
	rate CostRate
	repo Repository
}

// NewService builds a Service. A zero CostRate falls back to DefaultCostRate.
func NewService(rate CostRate, repo Repository) *Service {
	if rate.PerCoreMonth == 0 && rate.PerGBMonth == 0 {
		rate = DefaultCostRate()
	}
	return &Service{rate: rate, repo: repo}
}

// Evaluate runs Recommend over the supplied container inputs and persists the
// summary when a repository is configured. observedAt stamps the evaluation.
func (s *Service) Evaluate(ctx context.Context, clusterID int64, inputs []ContainerInput, observedAt time.Time) (WasteSummary, error) {
	summary := Recommend(clusterID, inputs, s.rate)
	summary.EvaluatedAt = observedAt.UTC()
	if s.repo != nil {
		if err := s.repo.Store(ctx, summary); err != nil {
			return summary, err
		}
	}
	return summary, nil
}

// QuantityFromResourceMap parses a Kubernetes container resources map
// (e.g. {"cpu":"100m","memory":"256Mi"}) into a Quantity. Missing keys are
// treated as Unset. This is the bridge from kubernetes.WorkloadContainer /
// PodContainer resource strings to the advisor's numeric model, so callers
// can build ContainerInput without re-implementing quantity parsing.
func QuantityFromResourceMap(requests, limits map[string]string) Quantity {
	return Quantity{
		CPURequest: parseOrUnset(requests, "cpu"),
		MemRequest: parseOrUnset(requests, "memory"),
		CPULimit:   parseOrUnset(limits, "cpu"),
		MemLimit:   parseOrUnset(limits, "memory"),
	}
}

func parseOrUnset(m map[string]string, key string) int64 {
	raw, ok := m[key]
	if !ok || raw == "" {
		return Unset
	}
	q, err := resource.ParseQuantity(raw)
	if err != nil {
		return Unset
	}
	switch key {
	case "cpu":
		return q.ScaledValue(resource.Nano)
	case "memory":
		return q.Value()
	}
	return Unset
}
