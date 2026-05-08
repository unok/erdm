package server

import (
	"encoding/json"
	"net/http"

	"github.com/unok/erdm/internal/layout"
)

// handleLayout は /api/layout の HTTP メソッドを GET / PUT に振り分ける。
// それ以外のメソッドは 405 で拒絶する（要件 5.5 / 6.1〜6.3）。
func (s *Server) handleLayout(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetLayout(w, r)
	case http.MethodPut:
		s.handlePutLayout(w, r)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed",
			"method "+r.Method+" not allowed for /api/layout")
	}
}

// handleGetLayout は座標ストアから現在の座標を JSON で返す（要件 5.5 / 6.1 / 6.5 / 6.6）。
//
// `internal/layout.Load` は不存在時に空 Layout + nil を返す仕様（タスク 5.1）
// のため、ファイル不存在は 200 + 空 JSON オブジェクト `{}` として自然に表現
// される（要件 6.5）。破損時は *LoadError が返り、500 + エラー JSON を返す
// （要件 6.6）。
func (s *Server) handleGetLayout(w http.ResponseWriter, _ *http.Request) {
	l, loadErr := layout.Load(s.layoutPath)
	if loadErr != nil {
		writeJSONError(w, http.StatusInternalServerError, "layout_load_error", loadErr.Cause)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(l); err != nil {
		return
	}
}

// handlePutLayout は受信した座標 JSON を座標ストアへ原子的置換で保存する
// （要件 6.1 / 6.2 / 6.3 / 10.2 / 10.3）。
//
// 書込み禁止モードでは 403 を返す。プロセス内ロックは PUT /api/schema と
// 同じ Server.mu を共有して直列化する（要件 10.2）。原子的置換は
// `internal/layout.Save` 内で実施済（タスク 5.1）。
func (s *Server) handlePutLayout(w http.ResponseWriter, r *http.Request) {
	if s.cfg.NoWrite {
		writeJSONError(w, http.StatusForbidden, "read_only_mode",
			"server is in --no-write mode")
		return
	}

	var l layout.Layout
	if err := json.NewDecoder(r.Body).Decode(&l); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	defer r.Body.Close()

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := layout.Save(s.layoutPath, l); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "layout_save_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}
