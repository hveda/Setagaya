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

	"github.com/heridotlife/Setagaya/v3/internal/ports"
)

// Store persists objects as files beneath Root.
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

// resolve maps a storage key to an absolute filesystem path, rejecting any key
// that would escape the root directory (path traversal).
func (s *Store) resolve(key string) (string, error) {
	key = filepath.ToSlash(key)
	for _, seg := range strings.Split(key, "/") {
		if seg == ".." {
			return "", fmt.Errorf("local: invalid key %q", key)
		}
	}
	full := filepath.Join(s.root, filepath.FromSlash(key))
	// Defense in depth: the resolved path must stay within root.
	if full != s.root && !strings.HasPrefix(full, s.root+string(os.PathSeparator)) {
		return "", fmt.Errorf("local: invalid key %q", key)
	}
	return full, nil
}

// Upload writes content to the file for key, creating parent directories and
// overwriting any existing file.
func (s *Store) Upload(_ context.Context, key string, content io.Reader) error {
	full, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return fmt.Errorf("local: mkdir: %w", err)
	}
	f, err := os.Create(full) // #nosec G304 -- path is validated by resolve()
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
	full, err := s.resolve(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(full) // #nosec G304 -- path is validated by resolve()
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
	full, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("local: delete: %w", err)
	}
	return nil
}

// URL returns a retrieval URL for key.
func (s *Store) URL(key string) string {
	if s.baseURL != "" {
		return s.baseURL + "/" + key
	}
	full, err := s.resolve(key)
	if err != nil {
		return ""
	}
	return "file://" + full
}
