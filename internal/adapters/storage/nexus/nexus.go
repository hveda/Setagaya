// Package nexus is an ObjectStore adapter backed by a Sonatype Nexus "raw"
// repository, proving the storage seam holds for a second real backend beyond
// the local filesystem. Objects map to raw-repo paths
// ({baseURL}/repository/{repo}/{key}) driven over plain HTTP: PUT to upload,
// GET to download, DELETE to remove. The base URL is injected, so the adapter
// is tested against an httptest server standing in for Nexus.
package nexus

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/heridotlife/honryu/internal/ports"
)

// maxObjectBytes caps how much a single object download may buffer.
const maxObjectBytes = 512 << 20

// Store is a Nexus raw-repository ObjectStore.
type Store struct {
	client   *http.Client
	baseURL  string // e.g. https://nexus.example.com
	repo     string // raw repository name
	username string
	password string
}

var _ ports.ObjectStore = (*Store)(nil)

// Option customizes a Store.
type Option func(*Store)

// WithClient overrides the HTTP client (default: 30s timeout).
func WithClient(c *http.Client) Option {
	return func(s *Store) {
		if c != nil {
			s.client = c
		}
	}
}

// WithBasicAuth sets credentials sent on every request.
func WithBasicAuth(username, password string) Option {
	return func(s *Store) {
		s.username = username
		s.password = password
	}
}

// New returns a Store for the raw repository repo under baseURL.
func New(baseURL, repo string, opts ...Option) *Store {
	s := &Store{
		client:  &http.Client{Timeout: 30 * time.Second},
		baseURL: strings.TrimRight(baseURL, "/"),
		repo:    repo,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// objectURL builds the raw-repo URL for key.
func (s *Store) objectURL(key string) string {
	return s.baseURL + "/repository/" + s.repo + "/" + strings.TrimLeft(key, "/")
}

// newRequest builds a request with credentials attached.
func (s *Store) newRequest(ctx context.Context, method, key string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, s.objectURL(key), body)
	if err != nil {
		return nil, err
	}
	if s.username != "" || s.password != "" {
		req.SetBasicAuth(s.username, s.password)
	}
	return req, nil
}

// Upload PUTs content to the object's raw-repo path, overwriting any existing
// object.
func (s *Store) Upload(ctx context.Context, key string, content io.Reader) error {
	data, err := io.ReadAll(content)
	if err != nil {
		return fmt.Errorf("nexus: read content: %w", err)
	}
	req, err := s.newRequest(ctx, http.MethodPut, key, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	default:
		return fmt.Errorf("nexus: upload %s failed: %s", key, resp.Status)
	}
}

// Download GETs the object bytes, mapping 404 to ports.ErrObjectNotFound.
func (s *Store) Download(ctx context.Context, key string) ([]byte, error) {
	req, err := s.newRequest(ctx, http.MethodGet, key, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxObjectBytes))
		if readErr != nil {
			return nil, fmt.Errorf("nexus: read %s: %w", key, readErr)
		}
		return data, nil
	case http.StatusNotFound:
		return nil, ports.ErrObjectNotFound
	default:
		return nil, fmt.Errorf("nexus: download %s failed: %s", key, resp.Status)
	}
}

// Delete removes the object. A missing object (404) is not an error.
func (s *Store) Delete(ctx context.Context, key string) error {
	req, err := s.newRequest(ctx, http.MethodDelete, key, nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusNotFound:
		return nil
	default:
		return fmt.Errorf("nexus: delete %s failed: %s", key, resp.Status)
	}
}

// URL returns the object's raw-repo retrieval URL.
func (s *Store) URL(key string) string {
	return s.objectURL(key)
}
