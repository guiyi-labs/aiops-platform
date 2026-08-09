package insight

import "testing"

func TestSnapshotKindsAndOperations(t *testing.T) {
	kinds := Kinds()
	if len(kinds) == 0 {
		t.Fatal("Kinds() returned empty catalog")
	}
	for i := 1; i < len(kinds); i++ {
		if kinds[i-1] >= kinds[i] {
			t.Fatalf("Kinds() not sorted: %v", kinds)
		}
	}
	ops := Operations()
	if len(ops) == 0 {
		t.Fatal("Operations() returned empty catalog")
	}
	for i := 1; i < len(ops); i++ {
		if ops[i-1] >= ops[i] {
			t.Fatalf("Operations() not sorted: %v", ops)
		}
	}
}
