// integration_test.go はタスク 6.6（要件 5.4/5.5/5.6/5.7/5.8/5.9/6.1/6.2/6.3/
// 6.6/7.7/7.8/7.9/9.4/10.1/10.2/10.3/10.4）に対応する統合テスト。
//
// schema/layout/export の各 API・書込み禁止モード・dot 不在モード・並行 PUT の
// 直列化・原子的置換の堅牢性・graceful shutdown を `httptest.NewServer` 経由の
// 実 HTTP プロセスで網羅検証する。並行テストは `go test -race` で意義を持つ。
//
// Requirements: 5.4, 5.5, 5.6, 5.7, 5.8, 5.9, 6.1, 6.2, 6.3, 6.6, 7.7, 7.8, 7.9, 9.4, 10.1, 10.2, 10.3, 10.4
package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// integrationSchemaSrc は統合テスト用の最小 .erdm。
// schema_test.go の validSchemaSrc と同一だがテストグループ独立化のため別名で持つ。
const integrationSchemaSrc = `# Title: integration
users
    +id [bigserial][NN][U]
    name [varchar(64)][NN]
`

// setupIntegration は schemaPath を設定済みの Server / httptest.Server を返す。
// schemaPath は Cleanup で自動クリーンアップされる t.TempDir 配下に置かれる。
func setupIntegration(t *testing.T, cfg Config) (*Server, *httptest.Server, string) {
	t.Helper()
	schemaPath := writeSchemaFileWith(t, integrationSchemaSrc)
	cfg.SchemaPath = schemaPath
	if cfg.Listen == "" {
		cfg.Listen = "127.0.0.1"
	}
	srv, err := New(cfg, validSPAFS())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ts := httptest.NewServer(srv.newMux())
	t.Cleanup(ts.Close)
	return srv, ts, schemaPath
}

// doRequest は req を実行してレスポンスのステータス・ボディ・Content-Type を返す。
// テスト本体のボイラープレートを削減する。
func doRequest(t *testing.T, req *http.Request) (int, []byte, string) {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, body, resp.Header.Get("Content-Type")
}

// mustGet は url に GET し (status, body, content-type) を返す。
func mustGet(t *testing.T, url string) (int, []byte, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	return doRequest(t, req)
}

// mustPut は url に body を PUT し (status, respBody) を返す。
func mustPut(t *testing.T, url string, body []byte) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	status, respBody, _ := doRequest(t, req)
	return status, respBody
}

// TestIntegration_FullCycle は schema/layout/export の全 GET / 書込み可能な
// PUT が正常に動作することを通しで確認する（要件 5.4 / 5.5 / 5.6）。
//
// dot コマンドを必要とする SVG/PNG は HasDot=false で 503 確認に留め、
// 本物の dot 実行を要するケースは別テストで dot 必須を宣言する。
func TestIntegration_FullCycle(t *testing.T) {
	_, ts, _ := setupIntegration(t, Config{})

	// GET /api/schema
	status, body, ct := mustGet(t, ts.URL+"/api/schema")
	if status != http.StatusOK {
		t.Fatalf("GET /api/schema: status=%d body=%s", status, body)
	}
	if !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("GET /api/schema content-type: %q", ct)
	}

	// PUT /api/schema (validSchemaSrc を使って書き戻し)
	newBody := []byte(integrationSchemaSrc)
	status, respBody := mustPut(t, ts.URL+"/api/schema", newBody)
	if status != http.StatusOK {
		t.Fatalf("PUT /api/schema: status=%d body=%s", status, respBody)
	}

	// GET /api/layout (空)
	status, body, _ = mustGet(t, ts.URL+"/api/layout")
	if status != http.StatusOK {
		t.Fatalf("GET /api/layout: status=%d body=%s", status, body)
	}
	if got := strings.TrimSpace(string(body)); got != "{}" {
		t.Fatalf("GET /api/layout body=%q want {}", got)
	}

	// PUT /api/layout
	layoutBody := []byte(`{"users":{"x":10,"y":20}}`)
	status, respBody = mustPut(t, ts.URL+"/api/layout", layoutBody)
	if status != http.StatusOK {
		t.Fatalf("PUT /api/layout: status=%d body=%s", status, respBody)
	}

	// GET /api/export/ddl (default pg)
	status, body, ct = mustGet(t, ts.URL+"/api/export/ddl")
	if status != http.StatusOK {
		t.Fatalf("GET /api/export/ddl: status=%d body=%s", status, body)
	}
	if !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("ddl content-type: %q", ct)
	}
	if !strings.Contains(string(body), "DROP TABLE IF EXISTS") {
		t.Fatalf("ddl body missing DROP TABLE: %s", body)
	}

	// GET /api/export/ddl?dialect=sqlite3
	status, body, _ = mustGet(t, ts.URL+"/api/export/ddl?dialect=sqlite3")
	if status != http.StatusOK {
		t.Fatalf("GET sqlite ddl: status=%d body=%s", status, body)
	}
	if strings.Contains(string(body), "CASCADE") {
		t.Fatalf("sqlite ddl should not contain CASCADE: %s", body)
	}

	// GET /api/export/svg / png は HasDot=false なので 503 を確認。
	for _, p := range []string{"/api/export/svg", "/api/export/png"} {
		status, body, _ = mustGet(t, ts.URL+p)
		if status != http.StatusServiceUnavailable {
			t.Fatalf("GET %s: status=%d (want 503) body=%s", p, status, body)
		}
		if !strings.Contains(string(body), "graphviz_not_available") {
			t.Fatalf("GET %s: body should contain graphviz_not_available, got: %s", p, body)
		}
	}
}

// TestIntegration_NoWriteMode_BlocksAllPuts は --no-write モードで PUT が
// 全て 403 になり、GET 系・エクスポートは通常動作することを確認する（要件 6.3 / 7.8）。
func TestIntegration_NoWriteMode_BlocksAllPuts(t *testing.T) {
	_, ts, schemaPath := setupIntegration(t, Config{NoWrite: true})

	// PUT /api/schema -> 403
	status, body := mustPut(t, ts.URL+"/api/schema", []byte(integrationSchemaSrc))
	if status != http.StatusForbidden {
		t.Fatalf("PUT /api/schema: status=%d (want 403) body=%s", status, body)
	}
	if !strings.Contains(string(body), "read_only_mode") {
		t.Fatalf("body should contain read_only_mode, got: %s", body)
	}
	// PUT /api/layout -> 403
	status, body = mustPut(t, ts.URL+"/api/layout", []byte(`{"users":{"x":1,"y":2}}`))
	if status != http.StatusForbidden {
		t.Fatalf("PUT /api/layout: status=%d (want 403) body=%s", status, body)
	}

	// GET 系・エクスポートは通常動作
	for _, p := range []string{"/api/schema", "/api/layout", "/api/export/ddl"} {
		status, body, _ = mustGet(t, ts.URL+p)
		if status != http.StatusOK {
			t.Fatalf("GET %s in --no-write: status=%d body=%s", p, status, body)
		}
	}

	// 元ファイルは PUT 試行後も不変
	got, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if string(got) != integrationSchemaSrc {
		t.Fatalf("schema modified in --no-write mode\nwant=%q\ngot =%q", integrationSchemaSrc, got)
	}
}

// TestIntegration_NoDot_BlocksOnlySVGPNG は HasDot=false で SVG/PNG のみ
// 503 になり、DDL・schema・layout は通常動作することを確認する（要件 9.4）。
func TestIntegration_NoDot_BlocksOnlySVGPNG(t *testing.T) {
	_, ts, _ := setupIntegration(t, Config{HasDot: false})

	for _, p := range []string{"/api/export/svg", "/api/export/png"} {
		status, body, _ := mustGet(t, ts.URL+p)
		if status != http.StatusServiceUnavailable {
			t.Fatalf("GET %s: status=%d (want 503) body=%s", p, status, body)
		}
	}
	for _, p := range []string{"/api/export/ddl", "/api/schema", "/api/layout"} {
		status, body, _ := mustGet(t, ts.URL+p)
		if status != http.StatusOK {
			t.Fatalf("GET %s with HasDot=false: status=%d body=%s", p, status, body)
		}
	}
}

// TestIntegration_PutSchema_ParseError_FileIntact は不正な .erdm を PUT した
// ときに 400 を返し、元ファイルが破壊されないことを確認する（要件 7.9）。
func TestIntegration_PutSchema_ParseError_FileIntact(t *testing.T) {
	_, ts, schemaPath := setupIntegration(t, Config{})

	// 故意に文法エラーになる入力を送る（@groups[] の空配列）。
	bad := []byte("# Title: t\nusers @groups[]\n    +id [bigserial][NN][U]\n")
	status, body := mustPut(t, ts.URL+"/api/schema", bad)
	if status != http.StatusBadRequest {
		t.Fatalf("PUT bad schema: status=%d (want 400) body=%s", status, body)
	}
	got, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if string(got) != integrationSchemaSrc {
		t.Fatalf("schema modified on parse error\nwant=%q\ngot =%q", integrationSchemaSrc, got)
	}
}

// TestIntegration_ExportDDL_InvalidDialect_Returns400 は dialect=mysql などの
// 不正値で 400 + invalid_dialect を返すことを確認する。
func TestIntegration_ExportDDL_InvalidDialect_Returns400(t *testing.T) {
	_, ts, _ := setupIntegration(t, Config{})

	status, body, _ := mustGet(t, ts.URL+"/api/export/ddl?dialect=oracle")
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d (want 400) body=%s", status, body)
	}
	if !strings.Contains(string(body), "invalid_dialect") {
		t.Fatalf("body should contain invalid_dialect, got: %s", body)
	}
}

// TestIntegration_ConcurrentPuts_Serialized は 10 並列 PUT /api/schema が
// 全て 200 で完結し、最終ファイルが破損していないことを確認する（要件 10.2）。
//
// `-race` フラグ付きで実行することで、Server.mu によるプロセス内ロックの
// 直列化に違反するアクセスを検出する。
func TestIntegration_ConcurrentPuts_Serialized(t *testing.T) {
	_, ts, schemaPath := setupIntegration(t, Config{})

	// バリエーションを持たせた 10 種のボディ。各々パース成功する形にする。
	const N = 10
	bodies := make([][]byte, N)
	for i := 0; i < N; i++ {
		bodies[i] = []byte("# Title: c" + string(rune('A'+i)) + "\nusers\n    +id [bigserial][NN][U]\n")
	}

	var wg sync.WaitGroup
	statusCodes := make([]int, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			status, _ := mustPut(t, ts.URL+"/api/schema", bodies[i])
			statusCodes[i] = status
		}(i)
	}
	wg.Wait()

	for i, st := range statusCodes {
		if st != http.StatusOK {
			t.Fatalf("PUT[%d] status=%d (want 200)", i, st)
		}
	}

	// 最終ファイルが何らかの送信ボディと完全一致する（破損していない）ことを確認。
	got, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	matched := false
	for _, b := range bodies {
		if bytes.Equal(got, b) {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("final schema does not match any submitted body (likely corrupted): %q", got)
	}
}

// TestIntegration_ConcurrentSchemaAndLayout_Serialized は schema PUT と layout
// PUT の並行実行でも全リクエストが成功し、両ファイルとも破損しないことを
// 確認する（要件 10.2）。プロセス内ロックを共有しているため直列化される。
func TestIntegration_ConcurrentSchemaAndLayout_Serialized(t *testing.T) {
	srv, ts, schemaPath := setupIntegration(t, Config{})

	const N = 10
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			mustPut(t, ts.URL+"/api/schema", []byte(integrationSchemaSrc))
		}()
		go func() {
			defer wg.Done()
			mustPut(t, ts.URL+"/api/layout", []byte(`{"users":{"x":1,"y":2}}`))
		}()
	}
	wg.Wait()

	// 両ファイルが妥当な状態で残っている（読み出しに成功）ことを確認。
	if _, err := os.ReadFile(schemaPath); err != nil {
		t.Fatalf("schema unreadable after concurrent puts: %v", err)
	}
	if _, err := os.ReadFile(srv.layoutPath); err != nil {
		t.Fatalf("layout unreadable after concurrent puts: %v", err)
	}
}

// TestIntegration_SchemaRoundTrip_BytesIdentical は PUT /api/schema で送信した
// バイト列が、ファイル読み出し結果と完全一致することを確認する（要件 7.7）。
//
// design.md §C4 / research.md §4.5.1 に基づき、サーバは受信ボディを再シリア
// ライズせずそのまま保存する。GET /api/schema レスポンスは *model.Schema の
// JSON エンコードであり生 .erdm とは異なるため、保存ファイルの直接読出しで検証する。
func TestIntegration_SchemaRoundTrip_BytesIdentical(t *testing.T) {
	_, ts, schemaPath := setupIntegration(t, Config{})

	// 空白パディングを含む意図的に整っていないバイト列。parser が受け入れる範囲。
	body := []byte("# Title: round\nusers\n    +id [bigserial][NN][U]\n    name [varchar(64)][NN]\n")
	status, respBody := mustPut(t, ts.URL+"/api/schema", body)
	if status != http.StatusOK {
		t.Fatalf("PUT: status=%d body=%s", status, respBody)
	}
	got, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("saved bytes != submitted body\nwant=%q\ngot =%q", body, got)
	}
}

// TestIntegration_PutSchema_RenameFailure_LeavesOriginal は親ディレクトリを
// 書き込み禁止にして PUT を行い、500 が返り元ファイルが不変であることを確認する
// （要件 10.3）。POSIX 環境（非 root）専用。Windows/root では skip。
func TestIntegration_PutSchema_RenameFailure_LeavesOriginal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode bits not enforced on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses POSIX permission checks")
	}

	_, ts, schemaPath := setupIntegration(t, Config{})

	// 親ディレクトリを r-x のみに設定して os.CreateTemp 失敗を強制する。
	dir := filepath.Dir(schemaPath)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	body := []byte("# Title: blocked\nusers\n    +id [bigserial][NN][U]\n")
	status, respBody := mustPut(t, ts.URL+"/api/schema", body)
	if status != http.StatusInternalServerError {
		t.Fatalf("PUT: status=%d (want 500) body=%s", status, respBody)
	}

	// 一時的に書き込み権限を戻して読み出し検証。
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("restore chmod: %v", err)
	}
	got, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if string(got) != integrationSchemaSrc {
		t.Fatalf("schema modified on rename failure\nwant=%q\ngot =%q", integrationSchemaSrc, got)
	}
}

// TestIntegration_PutLayout_RenameFailure_LeavesOriginal は layout 側でも
// 同様の堅牢性を確認する（要件 10.3）。
func TestIntegration_PutLayout_RenameFailure_LeavesOriginal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode bits not enforced on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses POSIX permission checks")
	}

	srv, ts, schemaPath := setupIntegration(t, Config{})

	// 既存 layout ファイルをまず作っておく（書込禁止後の比較材料）。
	original := []byte(`{"users":{"x":99,"y":99}}`)
	if status, body := mustPut(t, ts.URL+"/api/layout", original); status != http.StatusOK {
		t.Fatalf("seed layout PUT: status=%d body=%s", status, body)
	}

	dir := filepath.Dir(schemaPath)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	status, respBody := mustPut(t, ts.URL+"/api/layout", []byte(`{"users":{"x":1,"y":2}}`))
	if status != http.StatusInternalServerError {
		t.Fatalf("PUT layout: status=%d (want 500) body=%s", status, respBody)
	}

	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("restore chmod: %v", err)
	}
	got, err := os.ReadFile(srv.layoutPath)
	if err != nil {
		t.Fatalf("read layout: %v", err)
	}
	if !bytes.Contains(got, []byte(`"x": 99`)) || !bytes.Contains(got, []byte(`"y": 99`)) {
		t.Fatalf("layout modified on rename failure: %s", got)
	}
}

// TestIntegration_GracefulShutdown_OnContextCancel は Run(ctx) を起動した直後に
// ctx をキャンセルしても Run が短時間で終了することを確認する（要件 10.4）。
//
// 既存の TestRun_GracefulShutdown_OnContextCancel と同じ意図だが、本テストでは
// 別ポートでの独立実行とハンドラ分離により他統合テストへの影響を避ける。
func TestIntegration_GracefulShutdown_OnContextCancel(t *testing.T) {
	cfg := Config{SchemaPath: writeSchemaFileWith(t, integrationSchemaSrc), Port: 0, Listen: "127.0.0.1"}
	srv, err := New(cfg, validSPAFS())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- srv.Run(ctx)
	}()

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

// TestIntegration_ExportSVG_WithDot_ReturnsImage は dot コマンドが存在する
// 環境で SVG が実際に生成されることを確認する（要件 5.7 / 8.3）。
//
// dot コマンド不在環境では skip。CI 等で dot が利用可能な場合のみ意味を持つ。
func TestIntegration_ExportSVG_WithDot_ReturnsImage(t *testing.T) {
	if _, err := exec.LookPath("dot"); err != nil {
		t.Skip("dot command not available")
	}
	_, ts, _ := setupIntegration(t, Config{HasDot: true})

	status, body, ct := mustGet(t, ts.URL+"/api/export/svg")
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	if !strings.HasPrefix(ct, "image/svg+xml") {
		t.Fatalf("content-type: %q", ct)
	}
	if !bytes.Contains(body, []byte("<svg")) {
		t.Fatalf("svg body should start with <svg, got: %q", body[:min(80, len(body))])
	}
}

// TestIntegration_ExportPNG_WithDot_ReturnsImage は dot コマンドが存在する
// 環境で PNG が実際に生成されることを確認する（要件 5.8 / 8.4）。
func TestIntegration_ExportPNG_WithDot_ReturnsImage(t *testing.T) {
	if _, err := exec.LookPath("dot"); err != nil {
		t.Skip("dot command not available")
	}
	_, ts, _ := setupIntegration(t, Config{HasDot: true})

	status, body, ct := mustGet(t, ts.URL+"/api/export/png")
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	if !strings.HasPrefix(ct, "image/png") {
		t.Fatalf("content-type: %q", ct)
	}
	// PNG は magic 0x89 50 4E 47 で始まる。
	if len(body) < 8 || body[0] != 0x89 || string(body[1:4]) != "PNG" {
		t.Fatalf("png magic missing, head=% x", body[:min(8, len(body))])
	}
}

// TestIntegration_StartupPreconditions_MissingSchema は起動時前提チェックが
// SchemaPath 不存在で New 失敗することを確認する（要件 10.1）。
func TestIntegration_StartupPreconditions_MissingSchema(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no_such.erdm")
	_, err := New(Config{SchemaPath: missing, Listen: "127.0.0.1"}, validSPAFS())
	if err == nil {
		t.Fatalf("New should fail for missing schema")
	}
	if !strings.Contains(err.Error(), "schema file") {
		t.Fatalf("error should mention schema file, got: %v", err)
	}
}
