package requestctx

import (
	"context"
	"reflect"
	"testing"
)

func TestMetadataRoundTrip(t *testing.T) {
	want := Metadata{RequestID: "request-1", ClusterID: 7, Namespace: "default"}
	ctx := WithMetadata(context.Background(), want)

	got, ok := MetadataFrom(ctx)
	if !ok {
		t.Fatal("MetadataFrom() ok = false, want true")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MetadataFrom() = %#v, want %#v", got, want)
	}
	if RequestIDFrom(ctx) != want.RequestID {
		t.Fatalf("RequestIDFrom() = %q, want %q", RequestIDFrom(ctx), want.RequestID)
	}
}
