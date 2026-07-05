// export_test.go は internal/server の /api/export ハンドラのユニットテスト。
//
// Requirements: 5.6, 5.7, 5.8, 8.1, 8.2, 8.3, 8.4, 9.4
package server

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// TestHandleExportDDL_DefaultDialectIsPG は dialect クエリ未指定時に
// PostgreSQL の DDL が返ることを確認する（design.md §C10 表）。
func TestHandleExportDDL_DefaultDialectIsPG(t *testing.T) {
	path := writeSchemaFileWith(t, validSchemaSrc)
	_, ts := newTestServerWith(t, Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1"})

	resp, err := http.Get(ts.URL + "/api/export/ddl")
	if err != nil {
		t.Fatalf("GET /api/export/ddl: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d, want %d (body=%s)", resp.StatusCode, http.StatusOK, body)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("Content-Type: got %q, want text/plain prefix", got)
	}
	body, _ := io.ReadAll(resp.Body)
	// PostgreSQL 系の特徴的な構文 DROP TABLE IF EXISTS ... CASCADE が含まれる。
	if !strings.Contains(string(body), "DROP TABLE IF EXISTS") {
		t.Fatalf("body should contain DROP TABLE IF EXISTS, got: %s", body)
	}
	if !strings.Contains(string(body), "CASCADE") {
		t.Fatalf("default dialect should be pg (CASCADE expected), got: %s", body)
	}
}

// TestHandleExportDDL_SQLiteDialect は dialect=sqlite3 で SQLite3 DDL が返ることを確認する。
func TestHandleExportDDL_SQLiteDialect(t *testing.T) {
	path := writeSchemaFileWith(t, validSchemaSrc)
	_, ts := newTestServerWith(t, Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1"})

	resp, err := http.Get(ts.URL + "/api/export/ddl?dialect=sqlite3")
	if err != nil {
		t.Fatalf("GET /api/export/ddl?dialect=sqlite3: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d, want %d (body=%s)", resp.StatusCode, http.StatusOK, body)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "DROP TABLE IF EXISTS") {
		t.Fatalf("body should contain DROP TABLE IF EXISTS, got: %s", body)
	}
	// SQLite では CASCADE は出力されない。
	if strings.Contains(string(body), "CASCADE") {
		t.Fatalf("sqlite3 dialect should not contain CASCADE, got: %s", body)
	}
}

// TestHandleExportDDL_InvalidDialect_Returns400 は不正な dialect で 400 を返すことを確認する。
func TestHandleExportDDL_InvalidDialect_Returns400(t *testing.T) {
	path := writeSchemaFileWith(t, validSchemaSrc)
	_, ts := newTestServerWith(t, Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1"})

	resp, err := http.Get(ts.URL + "/api/export/ddl?dialect=mysql")
	if err != nil {
		t.Fatalf("GET /api/export/ddl?dialect=mysql: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "invalid_dialect") {
		t.Fatalf("body should contain invalid_dialect, got: %s", body)
	}
}

// TestHandleExportDDL_MethodNotAllowed_Returns405 は GET 以外を 405 で拒絶することを確認する。
func TestHandleExportDDL_MethodNotAllowed_Returns405(t *testing.T) {
	path := writeSchemaFileWith(t, validSchemaSrc)
	_, ts := newTestServerWith(t, Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1"})

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/export/ddl", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/export/ddl: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

// TestHandleExportSVG_NoDot_Returns503 は HasDot=false で 503 を返すことを確認する（要件 9.4）。
func TestHandleExportSVG_NoDot_Returns503(t *testing.T) {
	path := writeSchemaFileWith(t, validSchemaSrc)
	_, ts := newTestServerWith(t, Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1", HasDot: false})

	resp, err := http.Get(ts.URL + "/api/export/svg")
	if err != nil {
		t.Fatalf("GET /api/export/svg: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "graphviz_not_available") {
		t.Fatalf("body should contain graphviz_not_available, got: %s", body)
	}
}

// TestHandleExportPNG_NoDot_Returns503 は HasDot=false で 503 を返すことを確認する（要件 9.4）。
func TestHandleExportPNG_NoDot_Returns503(t *testing.T) {
	path := writeSchemaFileWith(t, validSchemaSrc)
	_, ts := newTestServerWith(t, Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1", HasDot: false})

	resp, err := http.Get(ts.URL + "/api/export/png")
	if err != nil {
		t.Fatalf("GET /api/export/png: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "graphviz_not_available") {
		t.Fatalf("body should contain graphviz_not_available, got: %s", body)
	}
}

// TestHandleExport_UnknownPath_Returns404 は /api/export/foo が 404 を返すことを確認する。
func TestHandleExport_UnknownPath_Returns404(t *testing.T) {
	path := writeSchemaFileWith(t, validSchemaSrc)
	_, ts := newTestServerWith(t, Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1"})

	resp, err := http.Get(ts.URL + "/api/export/unknown")
	if err != nil {
		t.Fatalf("GET /api/export/unknown: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "not_found") {
		t.Fatalf("body should contain not_found, got: %s", body)
	}
}

// TestHandleExportDDL_FileMissing_Returns500 は対象ファイル削除時に 500 を返すことを確認する。
func TestHandleExportDDL_FileMissing_Returns500(t *testing.T) {
	path := writeSchemaFileWith(t, validSchemaSrc)
	_, ts := newTestServerWith(t, Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1"})

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove schema: %v", err)
	}

	resp, err := http.Get(ts.URL + "/api/export/ddl")
	if err != nil {
		t.Fatalf("GET /api/export/ddl: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "schema_read_error") {
		t.Fatalf("body should contain schema_read_error, got: %s", body)
	}
}
