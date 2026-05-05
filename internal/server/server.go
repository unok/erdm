// Package server は erdm serve の HTTP サーバ実装を提供する（design.md §C10）。
//
// 公開境界は Config / Server / New / Run のみ。内部ハンドラは package private
// で保持し、外部からは触れない（design.md §C10 / ナレッジ「パブリック API の公開範囲」）。
//
// 提供する API ハンドラは GET/PUT /api/schema, GET/PUT /api/layout,
// GET /api/export/{ddl,svg,png}。未定義の /api/ パスは 404 + JSON エラーで返す。
// SPA 配信は GET /、アセットは /assets/ 配下、graceful shutdown は SIGINT/SIGTERM
// または親 ctx キャンセルで動作する（要件 5.3 / 5.12 / 9.3 / 10.4）。
package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// shutdownTimeout は SIGINT/SIGTERM 受信後に進行中リクエスト完了を待つ猶予時間。
// 要件 10.4 を満たすため固定値 5 秒で運用する（短すぎず長すぎない実用値）。
const shutdownTimeout = 5 * time.Second

// spaIndexFile は SPA エントリ HTML のファイル名。embed.FS 内のルート相対パスを期待する。
const spaIndexFile = "index.html"

// Config は erdm serve の起動時設定。
//
// SchemaPath は対象 .erdm の絶対 / 相対パス、Port / Listen は HTTP リッスン先、
// NoWrite は書き込み禁止モード（タスク 6.2 / 6.3 で利用）、HasDot は dot コマンド
// 検出結果（タスク 6.4 で利用）を表す。
type Config struct {
	SchemaPath string
	Port       int
	Listen     string
	NoWrite    bool
	HasDot     bool
}

// Server は HTTP サーバの状態と依存を保持する。
//
// mu は PUT /api/schema / PUT /api/layout の書き込みを直列化するための
// プロセス内ロック（要件 10.2）。layoutPath は座標 JSON のファイルパスで
// `<SchemaPath>.layout.json` の形式（design.md §論理データモデル）。
// New 内で計算してキャッシュし、各リクエストでの再計算を避ける。
type Server struct {
	cfg        Config
	spaFS      fs.FS
	mu         sync.Mutex // 書き込み API（PUT /api/schema, PUT /api/layout）の直列化用
	layoutPath string     // 座標 JSON のファイルパス（SchemaPath + ".layout.json"）
	server     *http.Server
}

// New は Config と SPA 埋め込み FS から Server を構築する。
//
// 起動時前提チェック（要件 10.1）として SchemaPath の通常ファイル存在を検証し、
// SPA エントリ（index.html）の存在を embed.FS 内でヘルスチェックする（要件 5.12 / 9.3）。
// いずれかが満たされない場合は明確なエラーで停止する。
func New(cfg Config, spaFS fs.FS) (*Server, error) {
	if err := validateSchemaFile(cfg.SchemaPath); err != nil {
		return nil, err
	}
	if err := validateSPAIndex(spaFS); err != nil {
		return nil, err
	}
	return &Server{
		cfg:        cfg,
		spaFS:      spaFS,
		layoutPath: cfg.SchemaPath + ".layout.json",
	}, nil
}

// Run は HTTP サーバを起動し、ctx のキャンセルまたは SIGINT / SIGTERM 受信で
// graceful shutdown する（要件 10.4）。
//
// ListenAndServe で http.ErrServerClosed 以外のエラーが発生した場合はそのまま返す。
// shutdown 自体のエラーは進行中リクエストが timeout を超えた場合に伴うため、
// ListenAndServe 由来のエラーを優先する。
func (s *Server) Run(ctx context.Context) error {
	mux := s.newMux()
	addr := net.JoinHostPort(s.cfg.Listen, strconv.Itoa(s.cfg.Port))
	s.server = &http.Server{Addr: addr, Handler: mux}

	signalCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-signalCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		return <-errCh
	}
}

// newMux は本サーバの HTTP ルーティングを構築する。
//
// "/" / "/assets/" は SPA 配信。"/api/schema" / "/api/layout" / "/api/export/"
// は実装済み API。それ以外の "/api/" 配下は 404 + JSON エラーで明示的に
// 返す（http.ServeMux の最長一致により未登録パスのみここに到達する）。
func (s *Server) newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleSPA)
	mux.Handle("/assets/", s.handleAssets())
	mux.HandleFunc("/api/schema", s.handleSchema)
	mux.HandleFunc("/api/layout", s.handleLayout)
	mux.HandleFunc("/api/export/", s.handleExport)
	mux.HandleFunc("/api/", s.handleAPINotFound)
	return mux
}

// validateSchemaFile は SchemaPath が通常ファイルとして読み取り可能かを検査する（要件 10.1）。
func validateSchemaFile(path string) error {
	if path == "" {
		return errors.New("schema file: path is empty")
	}
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("schema file: %w", err)
	}
	if st.IsDir() {
		return fmt.Errorf("schema file: %s is a directory, want a regular file", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("schema file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("schema file: close: %w", err)
	}
	return nil
}

// validateSPAIndex は SPA 埋め込み FS のルートに index.html が存在することを検査する（要件 5.12 / 9.3）。
func validateSPAIndex(spaFS fs.FS) error {
	if spaFS == nil {
		return errors.New("SPA filesystem is nil")
	}
	if _, err := fs.ReadFile(spaFS, spaIndexFile); err != nil {
		return fmt.Errorf("SPA %s not found in embedded FS: %w", spaIndexFile, err)
	}
	return nil
}
