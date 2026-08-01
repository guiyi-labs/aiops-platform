// Package optimization wires the read-only M61-M63 analyzers
// (FinOps right-sizing, CIS posture, deprecated-API check) behind the HTTP
// API. The analyzers themselves are pure functions over an observation
// bundle; this package only carries the shared configuration (e.g. the
// default cost rate) and lets the http server register the routes.
//
// Per ADR 0004 the analyzers never mutate cluster state: the caller supplies
// the already-collected observation bundle and the server returns findings.
package optimization

import "k8s-aiops.local/backend/internal/finops"

// Service carries the shared configuration for the optimization analyzers.
// It is intentionally thin: the analyzers are package-level pure functions and
// this service exists mainly so the http server can register the routes
// behind a single non-nil Options field, matching the per-milestone pattern.
type Service struct {
	defaultCostRate finops.CostRate
}

// NewService constructs the optimization service. rate is the monthly unit
// price used to translate idle resources into dollars when a request does not
// supply one; pass finops.DefaultCostRate() for the illustrative defaults.
func NewService(rate finops.CostRate) *Service {
	return &Service{defaultCostRate: rate}
}

// DefaultCostRate returns the configured cost rate, falling back to the
// illustrative defaults when the service is nil (e.g. in tests).
func (s *Service) DefaultCostRate() finops.CostRate {
	if s == nil {
		return finops.DefaultCostRate()
	}
	return s.defaultCostRate
}
