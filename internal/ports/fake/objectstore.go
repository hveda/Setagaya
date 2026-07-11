package fake

import (
	"bytes"
	"context"
	"io"
	"sync"

	"github.com/heridotlife/Setagaya/internal/ports"
)

// ObjectStore is an in-memory ports.ObjectStore.
type ObjectStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

// NewObjectStore returns an empty in-memory ObjectStore.
func NewObjectStore() *ObjectStore {
	return &ObjectStore{objects: make(map[string][]byte)}
}

var _ ports.ObjectStore = (*ObjectStore)(nil)

// Upload stores a copy of content at key, overwriting any existing object.
func (o *ObjectStore) Upload(_ context.Context, key string, content io.Reader) error {
	data, err := io.ReadAll(content)
	if err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.objects[key] = data
	return nil
}

// Download returns a copy of the object bytes, or ports.ErrObjectNotFound.
func (o *ObjectStore) Download(_ context.Context, key string) ([]byte, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	data, ok := o.objects[key]
	if !ok {
		return nil, ports.ErrObjectNotFound
	}
	return bytes.Clone(data), nil
}

// Delete removes key. Deleting a missing key is not an error.
func (o *ObjectStore) Delete(_ context.Context, key string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.objects, key)
	return nil
}

// URL returns an in-memory pseudo URL for key.
func (o *ObjectStore) URL(key string) string { return "memory:///" + key }
