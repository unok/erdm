package server

import (
	"log"
	"net/http"
	"time"
)

// listenBaseURL は実際にバインドされたリッスンアドレス（host:port）から
// ブラウザで開く http:// ベース URL を組み立てる。
// net.Listener.Addr().String() は IPv6 のホスト部を角括弧で囲んで返すため
// そのまま連結できる。
func listenBaseURL(addr string) string {
	return "http://" + addr
}

// logStartup は serve モード起動直後に設定概要を標準エラーへ出す。
// addr にはリスナーの実バインドアドレス（net.Listener.Addr().String()）を渡す。
// --port=0（OS 割当）の場合も実際に割り当てられたポートが表示される。
func (s *Server) logStartup(addr string) {
	mode := "read-write (PUT /api/schema, PUT /api/layout enabled)"
	if s.cfg.NoWrite {
		mode = "read-only (--no-write)"
	}
	dot := "unavailable (SVG/PNG export returns 503)"
	if s.cfg.HasDot {
		dot = "available (SVG/PNG export)"
	}
	log.Printf("erdm serve: Web UI + REST API")
	log.Printf("erdm serve: listen %s (base URL %s)", addr, listenBaseURL(addr))
	log.Printf("erdm serve: schema %s", s.cfg.SchemaPath)
	log.Printf("erdm serve: layout %s", s.layoutPath)
	log.Printf("erdm serve: mode %s", mode)
	log.Printf("erdm serve: graphviz dot %s", dot)
	log.Printf("erdm serve: access log on (stderr)")
}

// withAccessLog はリクエストごとにメソッド・パス・ステータス・処理時間・クライアントをログする。
func (s *Server) withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(lw, r)
		log.Printf("erdm serve: %s %s %d %s %s",
			r.Method, r.URL.RequestURI(), lw.status, time.Since(start).Round(time.Microsecond), r.RemoteAddr)
	})
}

// loggingResponseWriter は実際にクライアントへ送られた応答ステータスを記録する。
// net/http は最初の WriteHeader（または Write による暗黙の 200）以降の
// WriteHeader を無視するため、こちらも最初の 1 回だけを記録して
// 「送っていないステータスをログする」ズレを防ぐ。
type loggingResponseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *loggingResponseWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *loggingResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		// Write が先行した場合、net/http は暗黙に 200 を送出する。
		w.wroteHeader = true
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}
