package alertroute

import "testing"

func TestModelTableNames(t *testing.T) {
	cases := map[interface{ TableName() string }]string{
		Receiver{}: "alert_route_receivers",
		Route{}:    "alert_routes",
		Silence{}:  "alert_silences",
		Inhibit{}:  "alert_inhibits",
		Delivery{}: "alert_route_deliveries",
	}
	for m, want := range cases {
		if got := m.TableName(); got != want {
			t.Errorf("%T.TableName = %q, want %q", m, got, want)
		}
	}
}
