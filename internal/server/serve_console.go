package server

import (
	"log"
	"net"
	"net/http"
	"strconv"
	"time"
)

// listenBaseURL はブラウザで開く際の http:// ベース URL を組み立てる。
// IPv6 のリッスンアドレスは net.JoinHostPort がホスト部を角括弧で囲む。
func listenBaseURL(host string, port int) string {
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port))
}

// logStartup は serve モード起動直後に設定概要を標準エラーへ出す。
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
	log.Printf("erdm serve: listen %s (base URL %s)", addr, listenBaseURL(s.cfg.Listen, s.cfg.Port))
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

// loggingResponseWriter は WriteHeader で渡された応答ステータスを記録する。
// Write のみで本文を返すハンドラは既定 200 として記録する。
type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *loggingResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
