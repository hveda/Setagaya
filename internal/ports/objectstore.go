package ports

import (
	"context"
	"errors"
	"io"
)

// ErrObjectNotFound is returned by ObjectStore.Download when the key is absent.
var ErrObjectNotFound = errors.New("ports: object not found")

// ObjectStore is a blob store for uploaded test artifacts (JMX scripts, CSV data,
// execution config). Keys are slash-separated paths, e.g. "scenario/42/test.jmx".
type ObjectStore interface {
	// Upload stores content at key, overwriting any existing object.
	Upload(ctx context.Context, key string, content io.Reader) error
	// Download returns the object bytes, or ErrObjectNotFound.
	Download(ctx context.Context, key string) ([]byte, error)
	// Delete removes the object at key. Deleting a missing key is not an error.
	Delete(ctx context.Context, key string) error
	// URL returns a retrieval URL for key (may be empty for stores without one).
	URL(key string) string
}
