// Generated targeted tests for pure automation helpers (repository JSON
// serialization, verifier resource-state parsing, identity/token generation
// and error sanitization). These run without a store or live cluster.

package automation

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"testing"

	"k8s-aiops.local/backend/internal/cluster"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

func TestPolicyGatesJSONRoundTrip(t *testing.T) {
	if got := unmarshalPolicyGates(JSONB("")); got != nil {
		t.Errorf("unmarshalPolicyGates(empty) = %v, want nil", got)
	}
	if got := unmarshalPolicyGates(JSONB("not-json")); got != nil {
		t.Errorf("unmarshalPolicyGates(bad) = %v, want nil", got)
	}
	if got := unmarshalPolicyGates(JSONB("[]")); got == nil {
		t.Error("unmarshalPolicyGates([]) = nil, want empty slice")
	}
	got := unmarshalPolicyGates(JSONB("[{\"code\":\"scope\",\"status\":\"failed\"}]"))
	if len(got) != 1 || got[0].Code != GateScope || got[0].Status != GateFailed {
		t.Errorf("unmarshalPolicyGates = %+v", got)
	}
}

func TestEvidenceSnapshotJSONRoundTrip(t *testing.T) {
	if got := unmarshalEvidenceSnapshot(JSONB("")); len(got.ResourceState) != 0 {
		t.Errorf("unmarshalEvidenceSnapshot(empty) = %+v", got)
	}
	if got := unmarshalEvidenceSnapshot(JSONB("not-json")); len(got.ResourceState) != 0 {
		t.Errorf("unmarshalEvidenceSnapshot(bad) = %+v", got)
	}
	got := unmarshalEvidenceSnapshot(JSONB("{\"resource_state\":{\"kind\":\"Deployment\"},\"slo_state\":{\"slo_id\":7,\"state\":\"ok\"}}"))
	if got.ResourceState["kind"] != "Deployment" {
		t.Errorf("ResourceState = %v", got.ResourceState)
	}
	if got.SLOState == nil || got.SLOState.SLOID != 7 || got.SLOState.State != "ok" {
		t.Errorf("SLOState = %+v", got.SLOState)
	}
}

func TestBytesEqual(t *testing.T) {
	if !bytesEqual([]byte("abc"), []byte("abc")) {
		t.Error("bytesEqual equal = false")
	}
	if bytesEqual([]byte("abc"), []byte("abd")) {
		t.Error("bytesEqual diff = true")
	}
	if bytesEqual([]byte("ab"), []byte("abc")) {
		t.Error("bytesEqual len diff = true")
	}
	if !bytesEqual(nil, nil) {
		t.Error("bytesEqual nil,nil = false")
	}
}

func TestResourceStateHelpers(t *testing.T) {
	if replicasInt(nil) != 0 {
		t.Error("replicasInt(nil) != 0")
	}
	replicas := int32(3)
	if replicasInt(&replicas) != 3 {
		t.Error("replicasInt(ptr) != 3")
	}
	var nilState map[string]any
	if resourceInt(nilState, "x") != 0 {
		t.Error("resourceInt(nil) != 0")
	}
	if resourceBool(nilState, "x") != false {
		t.Error("resourceBool(nil) != false")
	}
	if resourceStr(nilState, "x") != "" {
		t.Error("resourceStr(nil) != empty")
	}
	state := map[string]any{
		"i":   int(7),
		"i32": int32(8),
		"i64": int64(9),
		"f":   float64(10.0),
		"num": json.Number("11"),
		"str": "12",
		"b":   true,
		"no":  false,
	}
	if resourceInt(state, "i") != 7 || resourceInt(state, "i32") != 8 || resourceInt(state, "i64") != 9 || resourceInt(state, "f") != 10 || resourceInt(state, "num") != 11 {
		t.Error("resourceInt numeric mismatch")
	}
	if resourceInt(state, "str") != 0 || resourceInt(state, "missing") != 0 {
		t.Error("resourceInt fallback != 0")
	}
	if !resourceBool(state, "b") || resourceBool(state, "no") || resourceBool(state, "missing") || resourceBool(state, "str") {
		t.Error("resourceBool mismatch")
	}
	if resourceStr(state, "str") != "12" || resourceStr(state, "missing") != "" || resourceStr(state, "b") != "" {
		t.Error("resourceStr mismatch")
	}
}

func TestNewIdentityShape(t *testing.T) {
	id, token, hash, err := newIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := regexp.MatchString("^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$", id); !ok {
		t.Errorf("id = %q, want UUID v4 shape", id)
	}
	if token == "" || len(hash) != 32 {
		t.Errorf("token = %q hash len = %d", token, len(hash))
	}
}

func TestNewCorrelationRequestID(t *testing.T) {
	a := newCorrelationRequestID()
	b := newCorrelationRequestID()
	if a == "" || b == "" || a == b {
		t.Errorf("ids a=%q b=%q", a, b)
	}
	if ok, _ := regexp.MatchString("^[0-9a-f]{32}$", a); !ok {
		t.Errorf("correlation request id %q not 32 hex", a)
	}
}

func TestSafeExecutionError(t *testing.T) {
	cases := map[string]struct {
		err  error
		want string
	}{
		"api status": {
			err:  fmt.Errorf("wrap: %w", cluster.APIStatusError{StatusCode: 500}),
			want: "Kubernetes API rejected automation with HTTP 500",
		},
		"not found": {
			err:  fmt.Errorf("wrap: %w", k8sgateway.ErrResourceNotFound),
			want: "Kubernetes automation target was not found",
		},
		"target changed": {
			err:  ErrTargetChanged,
			want: "Kubernetes automation target changed after preview",
		},
		"generic": {
			err:  errors.New("boom"),
			want: "Kubernetes automation request failed",
		},
		"nil": {
			err:  nil,
			want: "Kubernetes automation request failed",
		},
	}
	for name, tc := range cases {
		if got := safeExecutionError(tc.err); got != tc.want {
			t.Errorf("%s: safeExecutionError = %q, want %q", name, got, tc.want)
		}
	}
}
