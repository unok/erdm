// schema_test.go は internal/server の /api/schema ハンドラのユニットテスト。
//
// Requirements: 5.4, 5.9, 7.7, 7.8, 7.9
package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validSchemaSrc はテスト用の最小 .erdm。Parse が成功するよう
// 1 テーブル + 1 カラムを含む。
const validSchemaSrc = `# Title: t
users
    +id [bigserial][NN][U]
`

// invalidSchemaSrc は意図的にパース失敗させる .erdm。@groups[] の空配列は
// 文法側で禁止されているため確実にエラーになる（要件 2.5、parser_test.go の
// TestParse_GroupsEmptyArrayRejected と同じ入力パターン）。
const invalidSchemaSrc = `# Title: t
users @groups[]
    +id [bigserial][NN][U]
`

// newTestServerWith は cfg 上書き対応版。SchemaPath は呼び出し側が設定し、
// SPA は validSPAFS() を使う。Server を返してテスト本体が直接 HTTP ハンドラを
// 起動できるようにする。
func newTestServerWith(t *testing.T, cfg Config) (*Server, *httptest.Server) {
	t.Helper()
	srv, err := New(cfg, validSPAFS())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ts := httptest.NewServer(srv.newMux())
	t.Cleanup(ts.Close)
	return srv, ts
}

// writeSchemaFileWith は dir 配下に schema.erdm を src で書き出してパスを返す。
func writeSchemaFileWith(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "schema.erdm")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	return path
}

// TestHandleGetSchema_ParsesAndReturnsJSON は GET /api/schema が正常な
// .erdm をパースして JSON で返すことを確認する（要件 5.4）。
func TestHandleGetSchema_ParsesAndReturnsJSON(t *testing.T) {
	path := writeSchemaFileWith(t, validSchemaSrc)
	_, ts := newTestServerWith(t, Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1"})

	resp, err := http.Get(ts.URL + "/api/schema")
	if err != nil {
		t.Fatalf("GET /api/schema: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type: got %q, want application/json prefix", got)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("response is not valid JSON: %v\nbody=%s", err, body)
	}
	tables, ok := got["Tables"].([]any)
	if !ok || len(tables) == 0 {
		t.Fatalf("response should contain non-empty Tables, got: %v", got)
	}
}

// TestHandleGetSchema_FileMissing_Returns500 はスキーマファイルが
// 起動後に削除された場合に 500 + エラー JSON を返すことを確認する（要件 5.9）。
func TestHandleGetSchema_FileMissing_Returns500(t *testing.T) {
	path := writeSchemaFileWith(t, validSchemaSrc)
	_, ts := newTestServerWith(t, Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1"})
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove schema: %v", err)
	}

	resp, err := http.Get(ts.URL + "/api/schema")
	if err != nil {
		t.Fatalf("GET /api/schema: %v", err)
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

// TestHandleGetSchema_ParseError_Returns500 は破損 .erdm が 500 +
// schema_parse_error を返すことを確認する（要件 5.9）。
func TestHandleGetSchema_ParseError_Returns500(t *testing.T) {
	path := writeSchemaFileWith(t, validSchemaSrc)
	_, ts := newTestServerWith(t, Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1"})
	// 起動後にファイル内容を破損させる（New の前提チェックは存在のみ）。
	if err := os.WriteFile(path, []byte(invalidSchemaSrc), 0o644); err != nil {
		t.Fatalf("write invalid schema: %v", err)
	}

	resp, err := http.Get(ts.URL + "/api/schema")
	if err != nil {
		t.Fatalf("GET /api/schema: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "schema_parse_error") {
		t.Fatalf("body should contain schema_parse_error, got: %s", body)
	}
}

// TestHandlePutSchema_NoWrite_Returns403 は --no-write モードで PUT が
// 403 を返し書き込みを行わないことを確認する（要件 7.8）。
func TestHandlePutSchema_NoWrite_Returns403(t *testing.T) {
	path := writeSchemaFileWith(t, validSchemaSrc)
	_, ts := newTestServerWith(t, Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1", NoWrite: true})

	newBody := []byte("# Title: t2\nusers\n    +id [bigserial][NN][U]\n")
	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/schema", bytes.NewReader(newBody))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/schema: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "read_only_mode") {
		t.Fatalf("body should contain read_only_mode, got: %s", body)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema after no-write PUT: %v", err)
	}
	if string(got) != validSchemaSrc {
		t.Fatalf("schema file should not be modified in --no-write mode\nwant=%q\ngot =%q",
			validSchemaSrc, got)
	}
}

// TestHandlePutSchema_ParseError_Returns400_FileIntact は不正な .erdm を
// PUT した場合に 400 を返し、元ファイルが破壊されないことを確認する（要件 7.9）。
func TestHandlePutSchema_ParseError_Returns400_FileIntact(t *testing.T) {
	path := writeSchemaFileWith(t, validSchemaSrc)
	_, ts := newTestServerWith(t, Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1"})

	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/schema",
		bytes.NewReader([]byte(invalidSchemaSrc)))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/schema: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "parse_error") {
		t.Fatalf("body should contain parse_error, got: %s", body)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema after parse-error PUT: %v", err)
	}
	if string(got) != validSchemaSrc {
		t.Fatalf("schema file should not be modified on parse error\nwant=%q\ngot =%q",
			validSchemaSrc, got)
	}
}

// TestHandlePutSchema_ValidBody_PreservesBytes は受信ボディがバイト単位で
// 保存されること（サーバ側で再シリアライズしないこと）を確認する
// （要件 7.7、design.md §C4 / research.md §4.5.1）。
func TestHandlePutSchema_ValidBody_PreservesBytes(t *testing.T) {
	path := writeSchemaFileWith(t, validSchemaSrc)
	_, ts := newTestServerWith(t, Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1"})

	// SPA 由来の正規化されたバイト列を模擬。空白パディングは parser が
	// 受け入れる範囲で意図的に validSchemaSrc とは異なるバイト列にする。
	newBody := []byte("# Title: titled\nusers\n    +id [bigserial][NN][U]\n    name [varchar(64)][NN]\n")
	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/schema", bytes.NewReader(newBody))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/schema: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		got, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d, want %d (body=%s)", resp.StatusCode, http.StatusOK, got)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema after PUT: %v", err)
	}
	if !bytes.Equal(got, newBody) {
		t.Fatalf("saved bytes should equal received body\nwant=%q\ngot =%q", newBody, got)
	}
}

// TestHandlePutSchema_BodyTooLarge_Returns413 は maxSchemaBodyBytes を超える
// リクエストが 413 で拒絶され、元ファイルが破壊されないことを確認する
// （Copilot レビュー指摘 #2）。
func TestHandlePutSchema_BodyTooLarge_Returns413(t *testing.T) {
	path := writeSchemaFileWith(t, validSchemaSrc)
	_, ts := newTestServerWith(t, Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1"})

	oversized := bytes.Repeat([]byte("x"), maxSchemaBodyBytes+1)
	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/schema", bytes.NewReader(oversized))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/schema: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusRequestEntityTooLarge)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "request_body_too_large") {
		t.Fatalf("body should contain request_body_too_large, got: %s", body)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema after oversized PUT: %v", err)
	}
	if string(got) != validSchemaSrc {
		t.Fatalf("schema file should not be modified on 413\nwant=%q\ngot =%q", validSchemaSrc, got)
	}
}

// TestHandleSchema_MethodNotAllowed_Returns405 は GET / PUT 以外の
// メソッドが 405 で拒絶されることを確認する。
func TestHandleSchema_MethodNotAllowed_Returns405(t *testing.T) {
	path := writeSchemaFileWith(t, validSchemaSrc)
	_, ts := newTestServerWith(t, Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1"})

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/schema", bytes.NewReader([]byte("ignored")))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/schema: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}
