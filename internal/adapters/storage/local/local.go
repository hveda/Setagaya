// Package local is a filesystem-backed ObjectStore adapter. Objects are stored
// as files under a root directory, keyed by their slash-separated path. It is
// the default store for local development and tests; GCP and Nexus adapters
// implement the same port for production.
package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/heridotlife/Setagaya/internal/ports"
)

// Store persists objects as files beneath root.
type Store struct {
	root    string
	baseURL string
}

// New returns a Store rooted at root. baseURL, if non-empty, is used to build
// retrieval URLs (otherwise a file:// URL is returned).
func New(root, baseURL string) *Store {
	return &Store{root: filepath.Clean(root), baseURL: strings.TrimRight(baseURL, "/")}
}

var _ ports.ObjectStore = (*Store)(nil)

// openRoot returns an *os.Root confined to the store's root directory. All file
// operations go through the returned root, so the OS itself rejects any key
// that would escape it (via "..", absolute paths, or symlinks) — no manual path
// sanitisation required. The caller must Close the root.
func (s *Store) openRoot() (*os.Root, error) {
	if err := os.MkdirAll(s.root, 0o750); err != nil {
		return nil, fmt.Errorf("local: mkdir root: %w", err)
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, fmt.Errorf("local: open root: %w", err)
	}
	return root, nil
}

// Upload writes content to the file for key, creating parent directories and
// overwriting any existing file.
func (s *Store) Upload(_ context.Context, key string, content io.Reader) error {
	root, err := s.openRoot()
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	name := filepath.FromSlash(key)
	if dir := filepath.Dir(name); dir != "." {
		if err := root.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("local: mkdir: %w", err)
		}
	}
	f, err := root.Create(name)
	if err != nil {
		return fmt.Errorf("local: create: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(f, content); err != nil {
		return fmt.Errorf("local: write: %w", err)
	}
	return f.Close()
}

// Download reads the object bytes, or returns ports.ErrObjectNotFound.
func (s *Store) Download(_ context.Context, key string) ([]byte, error) {
	root, err := s.openRoot()
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()

	data, err := root.ReadFile(filepath.FromSlash(key))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ports.ErrObjectNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("local: read: %w", err)
	}
	return data, nil
}

// Delete removes the file for key. A missing file is not an error.
func (s *Store) Delete(_ context.Context, key string) error {
	root, err := s.openRoot()
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	if err := root.Remove(filepath.FromSlash(key)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("local: delete: %w", err)
	}
	return nil
}

// URL returns a retrieval URL for key.
func (s *Store) URL(key string) string {
	if s.baseURL != "" {
		return s.baseURL + "/" + key
	}
	return "file://" + filepath.Join(s.root, filepath.FromSlash(key))
}
