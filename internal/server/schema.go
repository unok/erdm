package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/unok/erdm/internal/parser"
)

// maxSchemaBodyBytes は PUT /api/schema が受け付けるリクエストボディの上限。
// .erdm はテキストでありローカル運用前提でも、io.ReadAll の OOM 防止と
// 誤送信検出のため上限を置く（4 MiB は実用上のスキーマ規模に対し充分大きい）。
const maxSchemaBodyBytes = 4 * 1024 * 1024

// handleSchema は /api/schema の HTTP メソッドを GET / PUT に振り分ける。
// それ以外のメソッドは 405 で拒絶する（要件 5.4 / 7.7 / 7.8 / 7.9）。
func (s *Server) handleSchema(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetSchema(w, r)
	case http.MethodPut:
		s.handlePutSchema(w, r)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed",
			"method "+r.Method+" not allowed for /api/schema")
	}
}

// handleGetSchema は対象 .erdm を再パースして JSON で返す（要件 5.4 / 5.9）。
//
// パース成功時は *model.Schema をそのままエンコードする。サーバ側で再シリア
// ライズはしない（design.md §C4 / §6.2）。読み取り失敗・パース失敗は 500 を
// 返し、原因を JSON エラー本文に含める。
func (s *Server) handleGetSchema(w http.ResponseWriter, _ *http.Request) {
	body, err := os.ReadFile(s.cfg.SchemaPath)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "schema_read_error", err.Error())
		return
	}
	schema, parseErr := parser.Parse(body)
	if parseErr != nil {
		writeJSONErrorWithDetail(w, http.StatusInternalServerError,
			"schema_parse_error", parseErr.Message,
			map[string]int{"line": parseErr.Line, "column": parseErr.Column})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(schema); err != nil {
		return
	}
}

// handlePutSchema は受信ボディを妥当性検証のみ実施し、検証 OK ならボディそのものを
// 原子的置換で保存する（要件 7.7 / 7.8 / 7.9 / 10.2 / 10.3）。
//
// 重要: サーバ側で再シリアライズはしない（design.md §C4 / research.md §4.5.1）。
// `.erdm` の正規化責任は SPA 側（要件 7.6）にあり、サーバは受信バイトをそのまま保存する。
func (s *Server) handlePutSchema(w http.ResponseWriter, r *http.Request) {
	if s.cfg.NoWrite {
		writeJSONError(w, http.StatusForbidden, "read_only_mode",
			"server is in --no-write mode")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxSchemaBodyBytes)
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		// http.MaxBytesError の場合は 413 を返し、他のボディ読み取りエラーは
		// 引き続き 400 として扱う（要件 7.7 / 7.8 / 7.9 と整合）。
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request_body_too_large", err.Error())
			return
		}
		writeJSONError(w, http.StatusBadRequest, "request_body_error", err.Error())
		return
	}

	if _, parseErr := parser.Parse(body); parseErr != nil {
		writeJSONErrorWithDetail(w, http.StatusBadRequest,
			"parse_error", parseErr.Message,
			map[string]int{"line": parseErr.Line, "column": parseErr.Column})
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := writeSchemaAtomic(s.cfg.SchemaPath, body); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.code, err.cause.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// schemaWriteError はスキーマ書き込み中の失敗段階を識別する内部エラー。
// 呼び出し側（handlePutSchema）で外向きの error code に変換する。
type schemaWriteError struct {
	code  string
	cause error
}

func (e *schemaWriteError) Error() string { return e.cause.Error() }

// writeSchemaAtomic は path と同一ディレクトリに一時ファイルを作成し、
// `os.Rename` で原子的に置換する（要件 10.3）。POSIX の rename(2) は同一
// ファイルシステム上で原子的なため、書き込み途中のクラッシュ等で元ファイル
// が破壊されることはない。失敗時は一時ファイルを削除し元ファイルを保持する。
func writeSchemaAtomic(path string, body []byte) *schemaWriteError {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return &schemaWriteError{code: "schema_write_error", cause: err}
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		cleanup()
		return &schemaWriteError{code: "schema_write_error", cause: err}
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return &schemaWriteError{code: "schema_write_error", cause: err}
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return &schemaWriteError{code: "schema_rename_error", cause: err}
	}
	return nil
}
