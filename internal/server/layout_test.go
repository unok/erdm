// layout_test.go は internal/server の /api/layout ハンドラのユニットテスト。
//
// Requirements: 5.5, 6.1, 6.2, 6.3, 6.6
package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/unok/erdm/internal/layout"
)

// TestHandleGetLayout_NonExistent_ReturnsEmptyObject はレイアウトファイル
// 不存在時に 200 + 空 JSON `{}` を返すことを確認する（要件 5.5 / 6.5）。
func TestHandleGetLayout_NonExistent_ReturnsEmptyObject(t *testing.T) {
	path := writeSchemaFileWith(t, validSchemaSrc)
	_, ts := newTestServerWith(t, Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1"})

	resp, err := http.Get(ts.URL + "/api/layout")
	if err != nil {
		t.Fatalf("GET /api/layout: %v", err)
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
	got := strings.TrimSpace(string(body))
	if got != "{}" {
		t.Fatalf("body should be empty object, got: %q", got)
	}
}

// TestHandleGetLayout_ExistingFile_ReturnsContent は既存レイアウト JSON が
// 取得できることを確認する（要件 6.1）。
func TestHandleGetLayout_ExistingFile_ReturnsContent(t *testing.T) {
	path := writeSchemaFileWith(t, validSchemaSrc)
	srv, ts := newTestServerWith(t, Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1"})

	want := layout.Layout{"users": {X: 100, Y: 200}}
	if err := layout.Save(srv.layoutPath, want); err != nil {
		t.Fatalf("seed layout: %v", err)
	}

	resp, err := http.Get(ts.URL + "/api/layout")
	if err != nil {
		t.Fatalf("GET /api/layout: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var got layout.Layout
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got["users"] != (layout.Position{X: 100, Y: 200}) {
		t.Fatalf("users position = %+v, want {100, 200}", got["users"])
	}
}

// TestHandleGetLayout_Corrupted_Returns500 は破損 JSON を含むレイアウト
// ファイルに対して 500 + layout_load_error を返すことを確認する（要件 6.6）。
func TestHandleGetLayout_Corrupted_Returns500(t *testing.T) {
	path := writeSchemaFileWith(t, validSchemaSrc)
	srv, ts := newTestServerWith(t, Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1"})

	if err := os.WriteFile(srv.layoutPath, []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("write broken layout: %v", err)
	}

	resp, err := http.Get(ts.URL + "/api/layout")
	if err != nil {
		t.Fatalf("GET /api/layout: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "layout_load_error") {
		t.Fatalf("body should contain layout_load_error, got: %s", body)
	}
}

// TestHandlePutLayout_NoWrite_Returns403 は --no-write モードで PUT が
// 403 を返し書き込みを行わないことを確認する（要件 6.3）。
func TestHandlePutLayout_NoWrite_Returns403(t *testing.T) {
	path := writeSchemaFileWith(t, validSchemaSrc)
	srv, ts := newTestServerWith(t, Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1", NoWrite: true})

	body := []byte(`{"users":{"x":1,"y":2}}`)
	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/layout", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/layout: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
	respBody, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(respBody), "read_only_mode") {
		t.Fatalf("body should contain read_only_mode, got: %s", respBody)
	}
	if _, err := os.Stat(srv.layoutPath); !os.IsNotExist(err) {
		t.Fatalf("layout file should not be created in --no-write mode, stat err=%v", err)
	}
}

// TestHandlePutLayout_ValidJSON_Saves は正常な JSON を PUT した場合に
// ファイルへ保存されることを確認する（要件 6.1 / 6.2 / 10.3）。
func TestHandlePutLayout_ValidJSON_Saves(t *testing.T) {
	path := writeSchemaFileWith(t, validSchemaSrc)
	srv, ts := newTestServerWith(t, Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1"})

	body := []byte(`{"users":{"x":1.5,"y":2.5},"orders":{"x":-3,"y":4}}`)
	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/layout", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/layout: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d, want %d (body=%s)", resp.StatusCode, http.StatusOK, respBody)
	}

	got, loadErr := layout.Load(srv.layoutPath)
	if loadErr != nil {
		t.Fatalf("Load after PUT: %v", loadErr)
	}
	if got["users"] != (layout.Position{X: 1.5, Y: 2.5}) {
		t.Fatalf("users position = %+v, want {1.5, 2.5}", got["users"])
	}
	if got["orders"] != (layout.Position{X: -3, Y: 4}) {
		t.Fatalf("orders position = %+v, want {-3, 4}", got["orders"])
	}
}

// TestHandlePutLayout_InvalidJSON_Returns400 は不正な JSON で 400 +
// invalid_json を返すことを確認する。
func TestHandlePutLayout_InvalidJSON_Returns400(t *testing.T) {
	path := writeSchemaFileWith(t, validSchemaSrc)
	_, ts := newTestServerWith(t, Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1"})

	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/layout",
		bytes.NewReader([]byte("{not json")))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/layout: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "invalid_json") {
		t.Fatalf("body should contain invalid_json, got: %s", body)
	}
}

// TestHandleLayout_MethodNotAllowed_Returns405 は GET / PUT 以外のメソッドが
// 405 で拒絶されることを確認する。
func TestHandleLayout_MethodNotAllowed_Returns405(t *testing.T) {
	path := writeSchemaFileWith(t, validSchemaSrc)
	_, ts := newTestServerWith(t, Config{SchemaPath: path, Port: 0, Listen: "127.0.0.1"})

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/layout", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /api/layout: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}
