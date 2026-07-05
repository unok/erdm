package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSchemaBasename(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/tmp/xix.erdm", "xix"},
		{"xix.erdm", "xix"},
		{"/a/b/foo.bar.erdm", "foo.bar"},
		{"noext", "noext"},
		{"/a/b/noext", "noext"},
		{"", "schema"},
	}
	for _, tc := range cases {
		if got := schemaBasename(tc.path); got != tc.want {
			t.Errorf("schemaBasename(%q)=%q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestHandleMeta_ReturnsBasename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "my_schema.erdm")
	if err := os.WriteFile(path, []byte("// dummy"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	srv, err := New(Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1"}, validSPAFS())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/meta", nil)
	srv.newMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", rec.Code)
	}
	var got metaResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v (body=%q)", err, rec.Body.String())
	}
	if got.Basename != "my_schema" {
		t.Errorf("basename=%q, want my_schema", got.Basename)
	}
}

func TestHandleMeta_RejectsNonGet(t *testing.T) {
	srv, err := New(Config{SchemaPath: writeSchemaFile(t), Port: 0, Listen: "127.0.0.1"}, validSPAFS())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/meta", nil)
	srv.newMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status=%d, want 405", rec.Code)
	}
}
