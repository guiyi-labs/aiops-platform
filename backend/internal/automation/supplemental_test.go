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

// AllRunbooks 必须返回稳定顺序。
//
// 原实现直接遍历 catalog map，而 Go 的 map 遍历顺序随机：同一份目录连续两次
// 调用返回次序就可能不同，控制台上的 runbook 卡片刷新一次就换位置，UI 基线
// 截图也因此永远不稳定。这里连续取多次并比对顺序。
func TestAutomationRunbooksOrderIsStable(t *testing.T) {
	first := AllRunbooks()
	if len(first) == 0 {
		t.Fatal("catalog must not be empty")
	}
	want := make([]string, len(first))
	for i, r := range first {
		want[i] = r.RunbookID
	}
	for attempt := 0; attempt < 200; attempt++ {
		got := AllRunbooks()
		if len(got) != len(want) {
			t.Fatalf("attempt %d: length changed %d -> %d", attempt, len(want), len(got))
		}
		for i := range want {
			if got[i].RunbookID != want[i] {
				t.Fatalf("attempt %d: order changed at index %d: want %q, got %q", attempt, i, want[i], got[i].RunbookID)
			}
		}
	}
}

// runbookOrder 与 catalog 必须一一对应：登记了不存在的 ID，说明顺序表写错了；
// catalog 里有未登记的 ID，则会悄悄退化到兜底排序（顺序仍稳定但不可控）。
func TestAutomationRunbookOrderMatchesCatalog(t *testing.T) {
	listed := make(map[string]struct{}, len(runbookOrder))
	for _, id := range runbookOrder {
		if _, ok := catalog[id]; !ok {
			t.Fatalf("runbookOrder lists %q which is not in the catalog", id)
		}
		listed[id] = struct{}{}
	}
	for id := range catalog {
		if _, ok := listed[id]; !ok {
			t.Fatalf("catalog has %q which is missing from runbookOrder", id)
		}
	}
}
