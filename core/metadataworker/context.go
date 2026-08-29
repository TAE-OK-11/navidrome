package metadataworker

import "context"

type libraryIDKey struct{}

// WithLibraryID stores the active library ID for metadata worker requests.
func WithLibraryID(ctx context.Context, libraryID int) context.Context {
	return context.WithValue(ctx, libraryIDKey{}, libraryID)
}

// LibraryIDFromContext returns the library ID attached to ctx, or 0.
func LibraryIDFromContext(ctx context.Context) int {
	if ctx == nil {
		return 0
	}
	if value, ok := ctx.Value(libraryIDKey{}).(int); ok {
		return value
	}
	return 0
}
