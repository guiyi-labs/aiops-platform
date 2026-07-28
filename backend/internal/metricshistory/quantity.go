package metricshistory

import (
	"errors"
	"math"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
)

var errInvalidQuantity = errors.New("invalid Kubernetes metric quantity")

var (
	maximumCPUQuantity    = resource.NewScaledQuantity(math.MaxInt64, resource.Nano)
	maximumMemoryQuantity = resource.NewQuantity(math.MaxInt64, resource.DecimalSI)
)

func cpuNanocores(raw string) (int64, error) {
	quantity, err := resource.ParseQuantity(strings.TrimSpace(raw))
	if err != nil || quantity.Sign() < 0 || quantity.Cmp(*maximumCPUQuantity) > 0 {
		return 0, errInvalidQuantity
	}
	value := quantity.ScaledValue(resource.Nano)
	if value < 0 {
		return 0, errInvalidQuantity
	}
	return value, nil
}

func memoryBytes(raw string) (int64, error) {
	quantity, err := resource.ParseQuantity(strings.TrimSpace(raw))
	if err != nil || quantity.Sign() < 0 || quantity.Cmp(*maximumMemoryQuantity) > 0 {
		return 0, errInvalidQuantity
	}
	value := quantity.Value()
	if value < 0 {
		return 0, errInvalidQuantity
	}
	return value, nil
}
