package requestctx

import "context"

type metadataKey struct{}

type Metadata struct {
	RequestID        string
	ActorID          int64
	ActorName        string
	ActorDisplayName string
	Roles            []string
	ClusterID        int64
	Namespace        string
	Resource         string
	Name             string
	Action           string
}

func WithMetadata(ctx context.Context, metadata Metadata) context.Context {
	return context.WithValue(ctx, metadataKey{}, metadata)
}

func MetadataFrom(ctx context.Context) (Metadata, bool) {
	metadata, ok := ctx.Value(metadataKey{}).(Metadata)
	return metadata, ok
}

func RequestIDFrom(ctx context.Context) string {
	metadata, _ := MetadataFrom(ctx)
	return metadata.RequestID
}
