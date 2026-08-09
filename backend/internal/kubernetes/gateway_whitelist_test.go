package kubernetes

import "testing"

func TestGatewayAPIIsBrowsable(t *testing.T) {
	s := &Service{}
	cases := []struct {
		group, version, resource string
		namespaced               bool
		ok                       bool
	}{
		{"gateway.networking.k8s.io", "v1", "gateways", true, true},
		{"gateway.networking.k8s.io", "v1", "httproutes", true, true},
		{"gateway.networking.k8s.io", "v1", "gatewayclasses", false, true},
		{"gateway.networking.k8s.io", "v1", "tcpRoutes", false, false},
		{"istio.io", "v1", "gateways", false, false},
	}
	for _, tc := range cases {
		gotNamespaced, gotOK := s.IsCustomResourceBrowsable(tc.group, tc.version, tc.resource)
		if gotOK != tc.ok || gotNamespaced != tc.namespaced {
			t.Errorf("%s/%s/%s: got (namespaced=%v, ok=%v), want (namespaced=%v, ok=%v)", tc.group, tc.version, tc.resource, gotNamespaced, gotOK, tc.namespaced, tc.ok)
		}
	}
}
