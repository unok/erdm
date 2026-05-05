package server

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/unok/erdm/internal/ddl"
	"github.com/unok/erdm/internal/dot"
	"github.com/unok/erdm/internal/model"
	"github.com/unok/erdm/internal/parser"
)

// dotCommand は SVG/PNG 生成で起動する Graphviz の実行ファイル名。
// 値は固定（whitelist）で、ユーザ入力は -T 引数の値としてのみ渡る。
// シェル経由ではなく exec.Command で直接起動するためシェルインジェクションのリスクはない。
const dotCommand = "dot"

// handleExport は /api/export/{ddl,svg,png} を path で振り分ける（design.md §C10）。
//
// http.ServeMux は "/api/export/" の prefix マッチでこのハンドラに到達するため、
// ハンドラ内でフルパスを判別して 3 つのサブハンドラに委譲する。未対応パス
// （例: /api/export/foo）は 404 + エラー JSON で返す。
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/export/ddl":
		s.handleExportDDL(w, r)
	case "/api/export/svg":
		s.handleExportSVG(w, r)
	case "/api/export/png":
		s.handleExportPNG(w, r)
	default:
		writeJSONError(w, http.StatusNotFound, "not_found",
			"unknown export endpoint: "+r.URL.Path)
	}
}

// handleExportDDL は /api/export/ddl を処理する（要件 5.6 / 8.1 / 8.2）。
//
// dialect クエリパラメータ未指定時は "pg" を既定値とする（design.md §C10 表）。
// dot コマンド不在時も DDL は通常動作する（要件 9.4）。
func (s *Server) handleExportDDL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed",
			"method "+r.Method+" not allowed for /api/export/ddl")
		return
	}

	dialect := r.URL.Query().Get("dialect")
	if dialect == "" {
		dialect = "pg"
	}

	schema, err := s.loadAndParseSchema(w)
	if err != nil {
		return
	}

	var (
		data      []byte
		renderErr error
	)
	switch dialect {
	case "pg":
		data, renderErr = ddl.RenderPG(schema)
	case "sqlite3":
		data, renderErr = ddl.RenderSQLite(schema)
	default:
		writeJSONError(w, http.StatusBadRequest, "invalid_dialect",
			"unsupported dialect: "+dialect)
		return
	}
	if renderErr != nil {
		writeJSONError(w, http.StatusInternalServerError, "ddl_render_error", renderErr.Error())
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if _, err := w.Write(data); err != nil {
		return
	}
}

// handleExportSVG は /api/export/svg を処理する（要件 5.7 / 8.3 / 9.4）。
func (s *Server) handleExportSVG(w http.ResponseWriter, r *http.Request) {
	s.handleExportImage(w, r, "svg", "image/svg+xml")
}

// handleExportPNG は /api/export/png を処理する（要件 5.8 / 8.4 / 9.4）。
func (s *Server) handleExportPNG(w http.ResponseWriter, r *http.Request) {
	s.handleExportImage(w, r, "png", "image/png")
}

// handleExportImage は SVG/PNG 共通の処理。format は dot の -T に渡す形式名。
//
// dot コマンド不在時は 503 を返す（要件 9.4）。スキーマ読込・パース・DOT 生成・
// dot コマンド実行のいずれかが失敗した場合はエラー JSON で返す。
func (s *Server) handleExportImage(w http.ResponseWriter, r *http.Request, format, mediaType string) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed",
			"method "+r.Method+" not allowed for /api/export/"+format)
		return
	}
	if !s.cfg.HasDot {
		writeJSONError(w, http.StatusServiceUnavailable, "graphviz_not_available",
			"dot command not found in PATH")
		return
	}

	schema, err := s.loadAndParseSchema(w)
	if err != nil {
		return
	}
	dotText, err := dot.Render(schema)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "dot_render_error", err.Error())
		return
	}
	data, err := runDot(r.Context(), dotText, format)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "dot_command_failed", err.Error())
		return
	}
	w.Header().Set("Content-Type", mediaType)
	if _, err := w.Write(data); err != nil {
		return
	}
}

// loadAndParseSchema は s.cfg.SchemaPath を読み込み、parser.Parse で *model.Schema へ変換する。
//
// 失敗時は w にエラー JSON を書き込んで non-nil error を返す。呼び出し側は
// error != nil なら追加の応答を行わずに return する。
func (s *Server) loadAndParseSchema(w http.ResponseWriter) (*model.Schema, error) {
	body, readErr := os.ReadFile(s.cfg.SchemaPath)
	if readErr != nil {
		writeJSONError(w, http.StatusInternalServerError, "schema_read_error", readErr.Error())
		return nil, readErr
	}
	schema, parseErr := parser.Parse(body)
	if parseErr != nil {
		writeJSONErrorWithDetail(w, http.StatusInternalServerError,
			"schema_parse_error", parseErr.Message,
			map[string]int{"line": parseErr.Line, "column": parseErr.Column})
		return nil, parseErr
	}
	return schema, nil
}

// runDot は DOT テキストを stdin から渡して `dot -T <format>` を実行し、
// stdout の画像バイト列を返す。失敗時は stderr の内容をエラーメッセージに含める。
//
// ctx を CommandContext へ渡すことで、リクエスト中断時に dot プロセスも
// キャンセルされる。
func runDot(ctx context.Context, dotText, format string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, dotCommand, "-T", format)
	cmd.Stdin = strings.NewReader(dotText)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("dot -T %s: %w (stderr: %s)", format, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
