package automation

// Supplementary tests for catalog sanity, gate aggregation and the JSONB
// column wrapper used across the automation tables.

import (
	"testing"
)

func TestAllRunbooksExecutable(t *testing.T) {
	runbooks := AllRunbooks()
	if len(runbooks) < 2 {
		t.Fatalf("AllRunbooks = %d, want the compiled catalog", len(runbooks))
	}
	byID := map[string]RunbookDescriptor{}
	for _, r := range runbooks {
		if r.RunbookID == "" || r.ActionCode == "" || r.Title == "" || len(r.Steps) == 0 {
			t.Errorf("runbook %+v is incomplete", r)
		}
		if _, dup := byID[r.RunbookID]; dup {
			t.Errorf("duplicate runbook id %s", r.RunbookID)
		}
		byID[r.RunbookID] = r
	}
	// Every catalog entry must also be reachable via lookup.
	for id := range byID {
		if _, ok := LookupRunbook(id); !ok {
			t.Errorf("runbook %s missing from LookupRunbook", id)
		}
	}
}

func TestFailedGates(t *testing.T) {
	gates := []PolicyGate{
		{Code: GateScope, Status: GateFailed},
		{Code: GateSLOBurn, Status: GatePassed},
		{Code: GateFreezeWindow, Status: GateFailed},
	}
	if got := FailedGates(gates); len(got) != 2 {
		t.Errorf("FailedGates = %d, want 2", len(got))
	}
	if got := FailedGates(nil); len(got) != 0 {
		t.Errorf("FailedGates(nil) = %d, want 0", len(got))
	}
}

func TestJSONBValue(t *testing.T) {
	empty := JSONB{}
	v, err := empty.Value()
	if err != nil {
		t.Fatalf("JSONB{}.Value: %v", err)
	}
	if got, _ := v.([]byte); string(got) != "[]" {
		t.Errorf("JSONB{}.Value = %q, want []", got)
	}
	nonEmpty := JSONB(`{"a":1}`)
	v, err = nonEmpty.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if got, _ := v.([]byte); string(got) != `{"a":1}` {
		t.Errorf("Value = %q", got)
	}
}

func TestJSONBScan(t *testing.T) {
	var j JSONB
	if err := j.Scan(nil); err != nil {
		t.Fatal(err)
	}
	if string(j) != "[]" {
		t.Errorf("Scan(nil) = %q, want []", string(j))
	}
	if err := j.Scan([]byte(`{"x":1}`)); err != nil {
		t.Fatal(err)
	}
	if string(j) != `{"x":1}` {
		t.Errorf("Scan([]byte) = %q", string(j))
	}
	if err := j.Scan(`{"y":2}`); err != nil {
		t.Fatal(err)
	}
	if string(j) != `{"y":2}` {
		t.Errorf("Scan(string) = %q", string(j))
	}
}

func TestJSONBMarshalRoundTrip(t *testing.T) {
	empty, err := (JSONB{}).MarshalJSON()
	if err != nil || string(empty) != "[]" {
		t.Errorf("MarshalJSON empty = %s, %v", empty, err)
	}
	raw := JSONB(`{"k":"v"}`)
	marshaled, err := raw.MarshalJSON()
	if err != nil || string(marshaled) != `{"k":"v"}` {
		t.Errorf("MarshalJSON = %s, %v", marshaled, err)
	}
	var got JSONB
	if err := got.UnmarshalJSON(marshaled); err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"k":"v"}` {
		t.Errorf("round trip = %q", string(got))
	}
}
