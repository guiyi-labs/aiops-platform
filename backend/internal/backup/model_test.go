package backup

import (
	"reflect"
	"testing"
)

func TestStringArrayUsesPostgresArrayContract(t *testing.T) {
	value, err := (StringArray{"m28-source", "namespace-with-dash"}).Value()
	if err != nil {
		t.Fatalf("Value() error = %v", err)
	}
	if value != `{"m28-source","namespace-with-dash"}` {
		t.Fatalf("Value() = %q", value)
	}

	var decoded StringArray
	if err := decoded.Scan(`{"m28-source","namespace-with-dash"}`); err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if !reflect.DeepEqual(decoded, StringArray{"m28-source", "namespace-with-dash"}) {
		t.Fatalf("Scan() = %#v", decoded)
	}
}
