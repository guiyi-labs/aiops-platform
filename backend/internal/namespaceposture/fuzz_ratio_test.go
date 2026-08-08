package namespaceposture

import (
	"math"
	"testing"
)

// FuzzQuantityRatio exercises the quota utilization ratio parser. A reported
// ratio must always be finite (never NaN/Inf), mirroring the production guard.
func FuzzQuantityRatio(f *testing.F) {
	for _, seed := range []string{
		"0", "1", "1Ki", "999999999999999999999999", "not-a-qty", "", "-1Gi", "1e308",
	} {
		f.Add(seed, seed)
		f.Add(seed, "100Mi")
		f.Add("100Mi", seed)
	}
	f.Fuzz(func(t *testing.T, used, hard string) {
		ratio, ok := quantityRatio(used, hard)
		if !ok {
			return
		}
		if math.IsNaN(ratio) || math.IsInf(ratio, 0) {
			t.Fatalf("invalid ratio %v for used=%q hard=%q", ratio, used, hard)
		}
	})
}
