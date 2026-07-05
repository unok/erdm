// error_response_test.go はタスク 8.2（HTTP API のエラー応答と並行/原子性
// の通し検証）に対応する横断テスト。
//
// design.md §エラー処理で定義された 8 分類のエラー（入力 / 不存在 / 破損 /
// 権限 / 依存不在 / I/O / 競合 / シグナル）のそれぞれについて、
//
//   - 想定 HTTP ステータスコード
//   - 共通エラー JSON 形式 `{ "error": { "code", "message", "detail"? } }`
//
// の一貫性を検証する。本テストは既存の `integration_test.go` を破壊せず、
// 共通アサーションヘルパ `assertErrorEnvelope` を新規に提供することで
// エラー JSON 形式の一貫性を集約検証する。
//
// 並行（要件 10.2）/ 原子的置換（要件 10.3）/ graceful shutdown（要件 10.4）
// の通し検証は既存の `TestIntegration_ConcurrentPuts_Serialized` /
// `TestIntegration_ConcurrentSchemaAndLayout_Serialized` /
// `TestIntegration_PutSchema_RenameFailure_LeavesOriginal` /
// `TestIntegration_PutLayout_RenameFailure_LeavesOriginal` /
// `TestIntegration_GracefulShutdown_OnContextCancel` で網羅済のため、
// 本ファイルでは「8 分類が共通 JSON 形式を遵守する」観点に特化する。
package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// errorEnvelopeForAssert は assertErrorEnvelope 内部で使うレスポンス形状。
// production 側 errors.go の errorEnvelope/errorResponse と同じ JSON 構造を
// 独立に定義することで「production 側構造体が変わったらここで気付ける」
// 二重定義の安全網としても機能する。
type errorEnvelopeForAssert struct {
	Error struct {
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Detail  json.RawMessage `json:"detail,omitempty"`
	} `json:"error"`
}

// assertErrorEnvelope は body が共通エラー JSON 形式に従うことを検証する。
//
//   - "error" オブジェクトが存在する
//   - "error.code" が wantCode と一致する
//   - "error.message" が空でない（人間可読な説明が省略されていない）
//
// detail はオプショナル（要件 5.9 / 6.6 / 7.9 のパースエラー時のみ）のため
// 検証しない。
func assertErrorEnvelope(t *testing.T, body []byte, wantCode string) {
	t.Helper()
	var env errorEnvelopeForAssert
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("error body is not JSON: %v\nbody=%s", err, body)
	}
	if env.Error.Code != wantCode {
		t.Errorf("error.code=%q want %q\nbody=%s", env.Error.Code, wantCode, body)
	}
	if env.Error.Message == "" {
		t.Errorf("error.message is empty\nbody=%s", body)
	}
}

// TestErrorResponse_InputCategory_PutSchemaParseError は入力エラー分類
// （400 + parse_error）が共通 JSON 形式を返すことを確認する。
//
// Requirements: 7.9, 5.9
func TestErrorResponse_InputCategory_PutSchemaParseError(t *testing.T) {
	_, ts, _ := setupIntegration(t, Config{})

	bad := []byte("# Title: t\nusers @groups[]\n    +id [bigserial][NN][U]\n")
	status, body := mustPut(t, ts.URL+"/api/schema", bad)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d (want 400) body=%s", status, body)
	}
	assertErrorEnvelope(t, body, "parse_error")
}

// TestErrorResponse_InputCategory_PutLayoutInvalidJSON は layout の入力エラー
// 分類（400 + invalid_json）も共通 JSON 形式を返すことを確認する。
//
// Requirements: 6.6
func TestErrorResponse_InputCategory_PutLayoutInvalidJSON(t *testing.T) {
	_, ts, _ := setupIntegration(t, Config{})

	status, body := mustPut(t, ts.URL+"/api/layout", []byte("not a json"))
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d (want 400) body=%s", status, body)
	}
	assertErrorEnvelope(t, body, "invalid_json")
}

// TestErrorResponse_InputCategory_ExportDDLInvalidDialect は不正な dialect 指定
// が 400 + invalid_dialect で返ることを確認する。
//
// Requirements: 5.6, 8.1
func TestErrorResponse_InputCategory_ExportDDLInvalidDialect(t *testing.T) {
	_, ts, _ := setupIntegration(t, Config{})

	status, body, _ := mustGet(t, ts.URL+"/api/export/ddl?dialect=oracle")
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d (want 400) body=%s", status, body)
	}
	assertErrorEnvelope(t, body, "invalid_dialect")
}

// TestErrorResponse_NotFoundCategory_GetSchemaMissing は不存在分類
// （schema ファイルが PUT 後に外部削除された場合の 500 + schema_read_error）
// が共通 JSON 形式を返すことを確認する。要件 5.9 の「ファイル削除時 500」。
//
// Requirements: 5.9
func TestErrorResponse_NotFoundCategory_GetSchemaMissing(t *testing.T) {
	_, ts, schemaPath := setupIntegration(t, Config{})

	if err := os.Remove(schemaPath); err != nil {
		t.Fatalf("remove schema: %v", err)
	}
	status, body, _ := mustGet(t, ts.URL+"/api/schema")
	if status != http.StatusInternalServerError {
		t.Fatalf("status=%d (want 500) body=%s", status, body)
	}
	assertErrorEnvelope(t, body, "schema_read_error")
}

// TestErrorResponse_NotFoundCategory_ExportPath は未対応の export エンドポイント
// が 404 + not_found で返ることを確認する。
//
// Requirements: 5.9
func TestErrorResponse_NotFoundCategory_ExportPath(t *testing.T) {
	_, ts, _ := setupIntegration(t, Config{})

	status, body, _ := mustGet(t, ts.URL+"/api/export/unknown")
	if status != http.StatusNotFound {
		t.Fatalf("status=%d (want 404) body=%s", status, body)
	}
	assertErrorEnvelope(t, body, "not_found")
}

// TestErrorResponse_CorruptedCategory_GetLayoutCorrupted は破損分類
// （layout JSON 破損で 500 + layout_load_error）が共通 JSON 形式を返すことを
// 確認する。
//
// Requirements: 6.6
func TestErrorResponse_CorruptedCategory_GetLayoutCorrupted(t *testing.T) {
	srv, ts, _ := setupIntegration(t, Config{})

	if err := os.WriteFile(srv.layoutPath, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write corrupted layout: %v", err)
	}
	status, body, _ := mustGet(t, ts.URL+"/api/layout")
	if status != http.StatusInternalServerError {
		t.Fatalf("status=%d (want 500) body=%s", status, body)
	}
	assertErrorEnvelope(t, body, "layout_load_error")
}

// TestErrorResponse_PermissionCategory_NoWriteSchema は権限分類
// （--no-write モードで 403 + read_only_mode）が共通 JSON 形式を返すことを
// 確認する。
//
// Requirements: 6.3, 7.8
func TestErrorResponse_PermissionCategory_NoWriteSchema(t *testing.T) {
	_, ts, _ := setupIntegration(t, Config{NoWrite: true})

	status, body := mustPut(t, ts.URL+"/api/schema", []byte(integrationSchemaSrc))
	if status != http.StatusForbidden {
		t.Fatalf("status=%d (want 403) body=%s", status, body)
	}
	assertErrorEnvelope(t, body, "read_only_mode")
}

// TestErrorResponse_PermissionCategory_NoWriteLayout は layout 側の権限分類
// （403 + read_only_mode）も同じ JSON 形式を返すことを確認する。
//
// Requirements: 6.3
func TestErrorResponse_PermissionCategory_NoWriteLayout(t *testing.T) {
	_, ts, _ := setupIntegration(t, Config{NoWrite: true})

	status, body := mustPut(t, ts.URL+"/api/layout", []byte(`{"users":{"x":1,"y":2}}`))
	if status != http.StatusForbidden {
		t.Fatalf("status=%d (want 403) body=%s", status, body)
	}
	assertErrorEnvelope(t, body, "read_only_mode")
}

// TestErrorResponse_DependencyMissingCategory_NoDotSVG は依存不在分類
// （dot コマンド不在で 503 + graphviz_not_available）が共通 JSON 形式を
// 返すことを確認する。
//
// Requirements: 9.4
func TestErrorResponse_DependencyMissingCategory_NoDotSVG(t *testing.T) {
	_, ts, _ := setupIntegration(t, Config{HasDot: false})

	status, body, _ := mustGet(t, ts.URL+"/api/export/svg")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status=%d (want 503) body=%s", status, body)
	}
	assertErrorEnvelope(t, body, "graphviz_not_available")
}

// TestErrorResponse_DependencyMissingCategory_NoDotPNG は PNG エンドポイントも
// 同じ依存不在分類で共通 JSON 形式を返すことを確認する。
//
// Requirements: 9.4
func TestErrorResponse_DependencyMissingCategory_NoDotPNG(t *testing.T) {
	_, ts, _ := setupIntegration(t, Config{HasDot: false})

	status, body, _ := mustGet(t, ts.URL+"/api/export/png")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status=%d (want 503) body=%s", status, body)
	}
	assertErrorEnvelope(t, body, "graphviz_not_available")
}

// TestErrorResponse_IOCategory_PutSchemaRenameFailure は I/O 分類
// （rename 失敗で 500 + schema_rename_error）が共通 JSON 形式を返すことを
// 確認する。POSIX 環境（非 root）専用。Windows/root では skip。
//
// Requirements: 10.3
func TestErrorResponse_IOCategory_PutSchemaRenameFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode bits not enforced on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses POSIX permission checks")
	}

	_, ts, schemaPath := setupIntegration(t, Config{})

	dir := filepath.Dir(schemaPath)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	body := []byte("# Title: blocked\nusers\n    +id [bigserial][NN][U]\n")
	status, respBody := mustPut(t, ts.URL+"/api/schema", body)
	if status != http.StatusInternalServerError {
		t.Fatalf("status=%d (want 500) body=%s", status, respBody)
	}
	// 親ディレクトリ書込禁止のため、CreateTemp 失敗 → schema_write_error または
	// Rename 失敗 → schema_rename_error のいずれか。両方とも I/O 分類として
	// 共通 JSON 形式を遵守することを検証する。
	var env errorEnvelopeForAssert
	if err := json.Unmarshal(respBody, &env); err != nil {
		t.Fatalf("error body is not JSON: %v\nbody=%s", err, respBody)
	}
	if env.Error.Code != "schema_write_error" && env.Error.Code != "schema_rename_error" {
		t.Errorf("error.code=%q want schema_write_error or schema_rename_error\nbody=%s",
			env.Error.Code, respBody)
	}
	if env.Error.Message == "" {
		t.Errorf("error.message is empty\nbody=%s", respBody)
	}
}

// TestErrorResponse_AllCategoriesShareEnvelope は 8 分類のうち、各代表
// エラーが「`error` ラッパ + `code` + `message` を持つ」共通形式を
// 漏れなく遵守していることをテーブル駆動で集中検証する（要件 5.9 / 6.3 /
// 6.6 / 7.7 / 7.8 / 7.9 / 9.4）。
//
// 競合（10.2）/ I/O 詳細（10.3）/ シグナル（10.4）の通し検証は別テスト
// （integration_test.go の並行・rename・graceful shutdown）で網羅済の
// ため、本テストでは「JSON 形式の一貫性」観点のみを集約する。
//
// Requirements: 5.9, 6.3, 6.6, 7.7, 7.8, 7.9, 9.4, 10.2, 10.3, 10.4
func TestErrorResponse_AllCategoriesShareEnvelope(t *testing.T) {
	type endpointCheck struct {
		name      string
		method    string
		path      string
		body      []byte
		wantCode  string
		wantHTTP  int
		setupCfg  Config
		setupHook func(t *testing.T, srv *Server, schemaPath string)
	}

	cases := []endpointCheck{
		{
			name:     "input/parse_error",
			method:   http.MethodPut,
			path:     "/api/schema",
			body:     []byte("# Title: t\nusers @groups[]\n    +id [bigserial][NN][U]\n"),
			wantCode: "parse_error",
			wantHTTP: http.StatusBadRequest,
		},
		{
			name:     "input/invalid_json",
			method:   http.MethodPut,
			path:     "/api/layout",
			body:     []byte("not a json"),
			wantCode: "invalid_json",
			wantHTTP: http.StatusBadRequest,
		},
		{
			name:     "input/invalid_dialect",
			method:   http.MethodGet,
			path:     "/api/export/ddl?dialect=oracle",
			wantCode: "invalid_dialect",
			wantHTTP: http.StatusBadRequest,
		},
		{
			name:     "notfound/not_found",
			method:   http.MethodGet,
			path:     "/api/export/unknown",
			wantCode: "not_found",
			wantHTTP: http.StatusNotFound,
		},
		{
			name:     "permission/read_only_mode",
			method:   http.MethodPut,
			path:     "/api/schema",
			body:     []byte(integrationSchemaSrc),
			wantCode: "read_only_mode",
			wantHTTP: http.StatusForbidden,
			setupCfg: Config{NoWrite: true},
		},
		{
			name:     "dependency/graphviz_not_available",
			method:   http.MethodGet,
			path:     "/api/export/svg",
			wantCode: "graphviz_not_available",
			wantHTTP: http.StatusServiceUnavailable,
			setupCfg: Config{HasDot: false},
		},
		{
			name:     "method/method_not_allowed",
			method:   http.MethodDelete,
			path:     "/api/schema",
			wantCode: "method_not_allowed",
			wantHTTP: http.StatusMethodNotAllowed,
		},
		{
			name:     "notfound/schema_read_error",
			method:   http.MethodGet,
			path:     "/api/schema",
			wantCode: "schema_read_error",
			wantHTTP: http.StatusInternalServerError,
			setupHook: func(t *testing.T, _ *Server, schemaPath string) {
				if err := os.Remove(schemaPath); err != nil {
					t.Fatalf("remove schema: %v", err)
				}
			},
		},
		{
			name:     "corrupted/layout_load_error",
			method:   http.MethodGet,
			path:     "/api/layout",
			wantCode: "layout_load_error",
			wantHTTP: http.StatusInternalServerError,
			setupHook: func(t *testing.T, srv *Server, _ string) {
				if err := os.WriteFile(srv.layoutPath, []byte("{not json"), 0o644); err != nil {
					t.Fatalf("write corrupted layout: %v", err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, ts, schemaPath := setupIntegration(t, tc.setupCfg)
			if tc.setupHook != nil {
				tc.setupHook(t, srv, schemaPath)
			}
			req, err := http.NewRequest(tc.method, ts.URL+tc.path, strings.NewReader(string(tc.body)))
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			status, body, ct := doRequest(t, req)
			if status != tc.wantHTTP {
				t.Fatalf("status=%d want=%d body=%s", status, tc.wantHTTP, body)
			}
			if !strings.HasPrefix(ct, "application/json") {
				t.Errorf("content-type=%q want application/json", ct)
			}
			assertErrorEnvelope(t, body, tc.wantCode)
		})
	}
}
