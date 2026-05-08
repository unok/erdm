package server

import (
	"io/fs"
	"net/http"
)

// handleSPA は GET / で SPA エントリ（index.html）を返す。
//
// "/" 以外のパスは ServeMux のサブツリーマッチで本ハンドラに到達するが、
// アセット以外の任意のパス（例えば /favicon.ico や未定義ルート）に対しては
// 404 を返してフォールバック挙動を明確にする。SPA のクライアントサイドルーティング
// は SPA 側で URL を扱う前提（design.md §C10）。
func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(s.spaFS, spaIndexFile)
	if err != nil {
		http.Error(w, "SPA index not available", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if _, err := w.Write(data); err != nil {
		// ResponseWriter への書き込み失敗はクライアント側切断などが主因。
		// ヘッダ送出済みのため http.Error は呼べない。ログ責務は呼び出し側に委ねる。
		return
	}
}

// handleAssets は /assets/ 配下を SPA 埋め込み FS から配信する。
//
// http.FileServer は http.FileSystem を要求するため、io/fs.FS を http.FS で
// 包んで橋渡しする。ServeMux は "/assets/" でマッチしたパスをそのままハンドラへ渡すため、
// FS 側でも /assets/ を含むパスでファイルを引けるよう、FS のルート構造に従う前提。
func (s *Server) handleAssets() http.Handler {
	return http.FileServer(http.FS(s.spaFS))
}

// handleAPINotFound は登録済み /api/{schema,layout,export/...} 以外の
// `/api/` 配下リクエストに対する 404 ハンドラ。
//
// http.ServeMux は最長一致で登録ハンドラを選ぶため、`mux.HandleFunc("/api/", ...)`
// は 上記実装パスに該当しない `/api/...` のフォールバックとしてのみ呼ばれる。
// 他のエラー応答との一貫性のため `errorEnvelope` 形式の JSON で返す。
func (s *Server) handleAPINotFound(w http.ResponseWriter, r *http.Request) {
	writeJSONError(w, http.StatusNotFound, "not_found",
		"unknown API endpoint: "+r.URL.Path)
}
