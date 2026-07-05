// server_test.go は internal/server の構築・SPA 配信・graceful shutdown
// のユニットテスト。
//
// Requirements: 5.1, 5.2, 5.3, 5.12, 9.3, 10.4
package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"
	"time"
)

// validSPAFS は SPA エントリと assets の両方を含むテスト用ダミー FS を返す。
func validSPAFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":     &fstest.MapFile{Data: []byte("<!doctype html><title>test-spa</title>")},
		"assets/app.js":  &fstest.MapFile{Data: []byte("console.log('app');")},
		"assets/app.css": &fstest.MapFile{Data: []byte("body{margin:0;}")},
	}
}

// writeSchemaFile は一時ディレクトリにダミー .erdm を書き出してパスを返す。
// 本バッチではパース内容に依存しないため空ファイルで十分。
func writeSchemaFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.erdm")
	if err := os.WriteFile(path, []byte("// dummy"), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	return path
}

func TestNew_ValidConfig_Success(t *testing.T) {
	cfg := Config{SchemaPath: writeSchemaFile(t), Port: 0, Listen: "127.0.0.1"}
	srv, err := New(cfg, validSPAFS())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if srv == nil {
		t.Fatalf("New returned nil server without error")
	}
}

func TestNew_MissingSchemaFile_ReturnsError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no_such.erdm")
	cfg := Config{SchemaPath: missing, Port: 0, Listen: "127.0.0.1"}
	_, err := New(cfg, validSPAFS())
	if err == nil {
		t.Fatalf("New should fail for missing schema file")
	}
	if !strings.Contains(err.Error(), "schema file") {
		t.Fatalf("error should mention schema file, got: %v", err)
	}
}

func TestNew_SchemaIsDirectory_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{SchemaPath: dir, Port: 0, Listen: "127.0.0.1"}
	_, err := New(cfg, validSPAFS())
	if err == nil {
		t.Fatalf("New should fail when SchemaPath is a directory")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Fatalf("error should mention directory, got: %v", err)
	}
}

func TestNew_EmptySchemaPath_ReturnsError(t *testing.T) {
	_, err := New(Config{SchemaPath: ""}, validSPAFS())
	if err == nil {
		t.Fatalf("New should fail when SchemaPath is empty")
	}
}

func TestNew_MissingSPAIndex_ReturnsError(t *testing.T) {
	cfg := Config{SchemaPath: writeSchemaFile(t), Port: 0, Listen: "127.0.0.1"}
	emptyFS := fstest.MapFS{}
	_, err := New(cfg, emptyFS)
	if err == nil {
		t.Fatalf("New should fail when SPA index is missing")
	}
	if !strings.Contains(err.Error(), "index.html") {
		t.Fatalf("error should mention index.html, got: %v", err)
	}
}

func TestNew_NilSPAFS_ReturnsError(t *testing.T) {
	cfg := Config{SchemaPath: writeSchemaFile(t), Port: 0, Listen: "127.0.0.1"}
	_, err := New(cfg, nil)
	if err == nil {
		t.Fatalf("New should fail when spaFS is nil")
	}
}

// newTestHandler は Server を構築し、newMux 経由で HTTP ハンドラを返す。
// ListenAndServe を介さずに httptest.Server で配信ロジックのみを検証する。
func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	cfg := Config{SchemaPath: writeSchemaFile(t), Port: 0, Listen: "127.0.0.1"}
	srv, err := New(cfg, validSPAFS())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return srv.newMux()
}

func TestSPA_IndexHTMLServed(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "test-spa") {
		t.Fatalf("body should contain SPA marker, got: %q", body)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("Content-Type: got %q, want text/html prefix", got)
	}
}

func TestSPA_NonRootPath_404(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/no-such-page")
	if err != nil {
		t.Fatalf("GET /no-such-page: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestAssets_AppJSServed(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/assets/app.js")
	if err != nil {
		t.Fatalf("GET /assets/app.js: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "console.log") {
		t.Fatalf("body should contain asset content, got: %q", body)
	}
}

func TestAssets_MissingFile_404(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/assets/missing.js")
	if err != nil {
		t.Fatalf("GET /assets/missing.js: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

// TestRun_GracefulShutdown_OnContextCancel は Run が ctx キャンセルで
// graceful shutdown して nil を返すことを検証する（要件 10.4）。
//
// 0 番ポート（カーネル割り当て）でリッスンし、起動完了を確認してから ctx を
// キャンセルする。ListenAndServe はカーネル割り当てポートでも動作する。
func TestRun_GracefulShutdown_OnContextCancel(t *testing.T) {
	cfg := Config{SchemaPath: writeSchemaFile(t), Port: 0, Listen: "127.0.0.1"}
	srv, err := New(cfg, validSPAFS())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- srv.Run(ctx)
	}()

	// ListenAndServe の起動完了を待つ。0 番ポート利用のため Listen 完了の
	// 確認は明示的に行えない（Run の戻り値だけが手がかり）。短い猶予の後に
	// ctx をキャンセルすれば signal-context 経由で shutdown 経路に入る。
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error on graceful shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Run did not return within timeout after ctx cancel")
	}
}

// TestRun_GracefulShutdown_OnSIGTERM は Run が SIGTERM で graceful shutdown
// することを検証する（要件 10.4）。SIGTERM は Linux/macOS でサポートされる。
func TestRun_GracefulShutdown_OnSIGTERM(t *testing.T) {
	cfg := Config{SchemaPath: writeSchemaFile(t), Port: 0, Listen: "127.0.0.1"}
	srv, err := New(cfg, validSPAFS())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- srv.Run(context.Background())
	}()

	time.Sleep(100 * time.Millisecond)
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error on SIGTERM shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Run did not return within timeout after SIGTERM")
	}
}

// TestRun_ListenError_PropagatesError は使用中ポートで Listen 失敗時に
// エラーが上位へ伝播することを検証する。
func TestRun_ListenError_PropagatesError(t *testing.T) {
	// まず空きポートを掴む占有用サーバを立てる。
	occupy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer occupy.Close()

	// httptest.NewServer が確保したアドレスを取り出して、同じポートを
	// 別の Server に渡して衝突させる。
	addr := occupy.Listener.Addr().String()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("net.SplitHostPort(%q): %v", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("strconv.Atoi(%q): %v", portStr, err)
	}

	cfg := Config{SchemaPath: writeSchemaFile(t), Port: port, Listen: host}
	srv, err := New(cfg, validSPAFS())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- srv.Run(context.Background())
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("Run should fail when port is already in use")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Run did not return within timeout for port conflict")
	}
}
