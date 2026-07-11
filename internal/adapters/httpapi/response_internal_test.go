package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestWriteJSON_NilBody(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	writeJSON(rec, 204, nil)
	if rec.Code != 204 {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rec.Body.String())
	}
}

func TestWriteJSON_EncodeError(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	// A channel cannot be marshaled to JSON: exercises the encode-error branch.
	writeJSON(rec, 200, make(chan int))
	if rec.Code != 200 {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
}
