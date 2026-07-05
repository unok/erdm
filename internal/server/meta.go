package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
)

// metaResponse は GET /api/meta のレスポンス本体。
//
// Basename は起動時に指定された `.erdm` ファイル名から拡張子を除いたステム部分。
// SPA はこれをエクスポート（DDL/SVG/PNG）のダウンロードファイル名に用いる
// （例: `xix.erdm` → `xix.pg.sql` / `xix.svg` / `xix.png`）。
type metaResponse struct {
	Basename string `json:"basename"`
}

// schemaBasename は SchemaPath からダウンロード用のステム名を導出する。
// `foo/bar.erdm` → `bar`、`foo.bar.erdm` → `foo.bar`（最後の拡張子のみ除去）。
// 拡張子が無い場合はファイル名をそのまま返す。空文字の場合は "schema" を返す。
func schemaBasename(schemaPath string) string {
	base := filepath.Base(schemaPath)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	if base == "" || base == "." {
		return "schema"
	}
	return base
}

// handleMeta は GET /api/meta を処理し、SPA が必要とするサーバ側メタ情報を返す。
func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed",
			"method "+r.Method+" not allowed for /api/meta")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(metaResponse{Basename: schemaBasename(s.cfg.SchemaPath)})
}
