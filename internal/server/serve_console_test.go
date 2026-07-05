package server

import (
	"net/http/httptest"
	"testing"
)

func TestListenBaseURL_IPv4(t *testing.T) {
	got := listenBaseURL("127.0.0.1:8080")
	want := "http://127.0.0.1:8080"
	if got != want {
		t.Fatalf("listenBaseURL: got %q, want %q", got, want)
	}
}

// net.Listener.Addr().String() は IPv6 ホストを角括弧付きで返すため、
// その形式をそのまま受け取れることを確認する。
func TestListenBaseURL_IPv6(t *testing.T) {
	got := listenBaseURL("[::1]:3123")
	want := "http://[::1]:3123"
	if got != want {
		t.Fatalf("listenBaseURL: got %q, want %q", got, want)
	}
}

func TestListenBaseURL_AllInterfaces(t *testing.T) {
	got := listenBaseURL("0.0.0.0:3123")
	want := "http://0.0.0.0:3123"
	if got != want {
		t.Fatalf("listenBaseURL: got %q, want %q", got, want)
	}
}

// 最初の WriteHeader だけが記録され、後続の（net/http では無視される）
// 余分な WriteHeader でログ用ステータスが上書きされないことを確認する。
func TestLoggingResponseWriter_FirstWriteHeaderWins(t *testing.T) {
	rec := httptest.NewRecorder()
	lw := &loggingResponseWriter{ResponseWriter: rec, status: 200}
	lw.WriteHeader(404)
	lw.WriteHeader(500)
	if lw.status != 404 {
		t.Fatalf("status: got %d, want 404 (first WriteHeader)", lw.status)
	}
}

// Write が先行した場合は暗黙の 200 が確定し、その後の WriteHeader は
// 実際には送られないため記録も 200 のまま維持されることを確認する。
func TestLoggingResponseWriter_WriteImplies200(t *testing.T) {
	rec := httptest.NewRecorder()
	lw := &loggingResponseWriter{ResponseWriter: rec, status: 200}
	if _, err := lw.Write([]byte("body")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	lw.WriteHeader(500)
	if lw.status != 200 {
		t.Fatalf("status: got %d, want 200 (implicit by Write)", lw.status)
	}
}
