package cluster

import "fmt"

// APIStatusError preserves the legacy error type returned by the Kubernetes
// gateway. ClientProvider converts client-go *apierrors.StatusError into this
// type so that existing callers (mapGatewayError, mapCreateGatewayError,
// ResourceExists) continue to work without modification. A future milestone
// may replace this with direct apierrors.IsNotFound / IsConflict checks.
type APIStatusError struct{ StatusCode int }

func (e APIStatusError) Error() string {
	return fmt.Sprintf("Kubernetes API returned status %d", e.StatusCode)
}
