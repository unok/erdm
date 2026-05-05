// Package main は erdm CLI のエントリポイント。
//
// 第 1 引数で render / serve のサブコマンドに振り分ける（design.md §C1）。
//   - 既定（または `serve` 以外の第 1 引数）: render モード
//   - `serve`: serve モード（HTTP サーバ。本実装は tasks 6.x）
//
// render モードは旧 CLI（`erdm [-output_dir DIR] schema.erdm`）と完全互換で、
// 既存サンプルからの 5 種出力（`.dot` / `.png` / `.html` / `.pg.sql` / `.sqlite3.sql`）
// を出力ディレクトリへ生成する（要件 3.5 / 9.1）。
package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/unok/erdm/internal/ddl"
	"github.com/unok/erdm/internal/dot"
	"github.com/unok/erdm/internal/elk"
	"github.com/unok/erdm/internal/html"
	"github.com/unok/erdm/internal/parser"
	"github.com/unok/erdm/internal/server"
)

// spaDistFS は Vite ビルド成果物（frontend/dist 配下）を単一バイナリへ同梱する
// 埋め込み FS（要件 5.12 / 9.3）。ビルド前提として `make frontend` 等で
// `frontend/dist/index.html` および `frontend/dist/assets/*` が生成されている
// ことを要求する。`//go:embed all:` は dotfile 含めて再帰収集する指示子で、
// Vite が生成する hash 付きアセットも漏れなく取り込める。
//
//go:embed all:frontend/dist
var spaDistFS embed.FS

// 旧 CLI と整合する usage 文字列。`render` サブコマンドが既定動作のため
// `[render]` ではなく旧形式の表記を維持し、後方互換を読み手に明示する。
const usageRender = "Usage: erdm [-output_dir DIR] [--format=dot|elk] schema.erdm"
const usageServe = "Usage: erdm serve [--port=N] [--listen=ADDR] [--no-write] schema.erdm"

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "serve" {
		if err := runServe(args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := runRender(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// runRender は render モードの引数を解析し、フォーマットに応じて出力経路を分岐させる。
//
// 旧 CLI 互換のため `-output_dir DIR`（既定: カレントディレクトリ）と
// `--format=dot|elk`（既定: `dot`）の両方を受理する。
func runRender(args []string) error {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	outputDir := fs.String("output_dir", wd, "output directory")
	format := fs.String("format", "dot", "output format (dot|elk)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return errors.New(usageRender)
	}
	inputPath := fs.Arg(0)

	switch *format {
	case "dot":
		return renderDOT(*outputDir, inputPath)
	case "elk":
		return renderELK(*outputDir, isFlagExplicit(fs, "output_dir"), inputPath)
	default:
		return fmt.Errorf("unknown format: %s", *format)
	}
}

// isFlagExplicit は flag.FlagSet 上で name フラグが明示指定されたかを返す。
//
// `--format=elk` の出力先判定（design.md §C1）は、`-output_dir` の既定値と
// 「ユーザーが明示指定した同値」を区別する必要がある。`fs.Visit` は
// 明示的にセットされたフラグのみを巡回するため、これを利用する。
func isFlagExplicit(fs *flag.FlagSet, name string) bool {
	var found bool
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

// renderELK は ELK JSON を生成し、`-output_dir` の明示指定有無で
// 出力先を切り替える（design.md §C1、要件 4.1 / 9.4）。
//
// `outputDirExplicit == true` の場合は `<outputDir>/<basename>.elk.json` に
// 書き出し、`false` の場合は標準出力へ書き出す。`dot` コマンドの存在検査は
// 行わない（要件 9.4: ELK 形式は Graphviz 不要）。
func renderELK(outputDir string, outputDirExplicit bool, inputPath string) error {
	if err := requireFile(inputPath); err != nil {
		return err
	}
	src, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", inputPath, err)
	}
	schema, parseErr := parser.Parse(src)
	if parseErr != nil {
		return fmt.Errorf("parse %s: %w", inputPath, parseErr)
	}
	content, err := elk.Render(schema)
	if err != nil {
		return fmt.Errorf("render elk: %w", err)
	}
	if !outputDirExplicit {
		if _, err := os.Stdout.Write(content); err != nil {
			return fmt.Errorf("write stdout: %w", err)
		}
		return nil
	}
	if err := requireDir(outputDir); err != nil {
		return err
	}
	basename := stripExt(filepath.Base(inputPath))
	outPath := filepath.Join(outputDir, basename+".elk.json")
	if err := os.WriteFile(outPath, content, 0644); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}
	return nil
}

// renderDOT は旧 CLI と等価な 5 種出力（DOT/PNG/HTML/PG/SQLite）を出力ディレクトリへ生成する。
//
// PNG 生成のために外部 `dot` コマンド（Graphviz）の存在を必須前提とする（要件 9.1）。
// 不在時は標準エラーへ出力して非ゼロ終了する。
func renderDOT(outputDir, inputPath string) error {
	if _, err := exec.LookPath("dot"); err != nil {
		return fmt.Errorf("dot command not found in PATH; required for --format=dot: %w", err)
	}
	if err := requireFile(inputPath); err != nil {
		return err
	}
	if err := requireDir(outputDir); err != nil {
		return err
	}

	src, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", inputPath, err)
	}
	schema, parseErr := parser.Parse(src)
	if parseErr != nil {
		return fmt.Errorf("parse %s: %w", inputPath, parseErr)
	}

	basename := stripExt(filepath.Base(inputPath))
	dotPath := filepath.Join(outputDir, basename+".dot")
	pngPath := filepath.Join(outputDir, basename+".png")
	htmlPath := filepath.Join(outputDir, basename+".html")
	pgPath := filepath.Join(outputDir, basename+".pg.sql")
	sqlitePath := filepath.Join(outputDir, basename+".sqlite3.sql")

	dotText, err := dot.Render(schema)
	if err != nil {
		return fmt.Errorf("render dot: %w", err)
	}
	if err := os.WriteFile(dotPath, []byte(dotText), 0644); err != nil {
		return fmt.Errorf("write %s: %w", dotPath, err)
	}
	if err := exec.Command("dot", "-T", "png", "-o", pngPath, dotPath).Run(); err != nil {
		return fmt.Errorf("dot -T png: %w", err)
	}

	htmlBytes, err := html.Render(schema, filepath.Base(pngPath))
	if err != nil {
		return fmt.Errorf("render html: %w", err)
	}
	if err := os.WriteFile(htmlPath, htmlBytes, 0644); err != nil {
		return fmt.Errorf("write %s: %w", htmlPath, err)
	}

	pgBytes, err := ddl.RenderPG(schema)
	if err != nil {
		return fmt.Errorf("render pg ddl: %w", err)
	}
	if err := os.WriteFile(pgPath, pgBytes, 0644); err != nil {
		return fmt.Errorf("write %s: %w", pgPath, err)
	}

	sqliteBytes, err := ddl.RenderSQLite(schema)
	if err != nil {
		return fmt.Errorf("render sqlite ddl: %w", err)
	}
	if err := os.WriteFile(sqlitePath, sqliteBytes, 0644); err != nil {
		return fmt.Errorf("write %s: %w", sqlitePath, err)
	}
	return nil
}

// runServe は serve サブコマンドの引数を解析し、HTTP サーバを起動する。
//
// 引数解析（--port / --listen / --no-write）→ 入力ファイル検査（要件 10.1）→
// `dot` コマンド検出 → `server.New` で起動時前提チェック → `server.Run` で
// HTTP リッスン + graceful shutdown（要件 10.4）の流れ。
func runServe(args []string) error {
	// FlagSet 変数名を flagSet にしているのは、`io/fs` パッケージ（spaDistFS の
	// fs.Sub で利用）と短縮名 `fs` がシャドウしないようにするため。
	flagSet := flag.NewFlagSet("serve", flag.ContinueOnError)
	flagSet.SetOutput(os.Stderr)
	port := flagSet.Int("port", 8080, "HTTP listen port")
	listen := flagSet.String("listen", "127.0.0.1", "HTTP listen address")
	noWrite := flagSet.Bool("no-write", false, "disable write APIs (read-only mode)")
	if err := flagSet.Parse(args); err != nil {
		return err
	}
	if flagSet.NArg() == 0 {
		return errors.New(usageServe)
	}
	inputPath := flagSet.Arg(0)
	if err := requireFile(inputPath); err != nil {
		return err
	}
	// dot コマンドの可否は SVG/PNG エクスポート（tasks 6.4）で 503 判定に使う。
	// `server.Config.HasDot` へ注入し、API ハンドラ側で参照する。
	_, hasDotErr := exec.LookPath("dot")
	hasDot := hasDotErr == nil

	cfg := server.Config{
		SchemaPath: inputPath,
		Port:       *port,
		Listen:     *listen,
		NoWrite:    *noWrite,
		HasDot:     hasDot,
	}
	// fs.Sub で `frontend/dist` をルート化し、SPA は index.html / assets/... を
	// FS のルート相対で参照できる状態にする（server.spaIndexFile = "index.html"
	// および handleAssets の `/assets/` 直下配信が成立する）。
	spaFS, err := fs.Sub(spaDistFS, "frontend/dist")
	if err != nil {
		return fmt.Errorf("spa embed sub: %w", err)
	}
	srv, err := server.New(cfg, spaFS)
	if err != nil {
		return fmt.Errorf("server init: %w", err)
	}
	return srv.Run(context.Background())
}

// requireFile は path が存在し、かつディレクトリでない通常ファイルかを検査する。
// 不在・ディレクトリ・読み取り不可いずれの場合も標準エラー向けに分かりやすい
// エラーを返す（要件 10.1）。
func requireFile(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("input file: %w", err)
	}
	if st.IsDir() {
		return fmt.Errorf("input file: %s is a directory, want a regular file", path)
	}
	return nil
}

// requireDir は path がディレクトリとして存在することを検査する。
func requireDir(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("output_dir: %w", err)
	}
	if !st.IsDir() {
		return fmt.Errorf("output_dir: %s is not a directory", path)
	}
	return nil
}

// stripExt は basename から最後の拡張子を取り除く。`foo.erdm` → `foo`。
// `filepath.Ext` は `.tar.gz` の `.gz` のみ取れるため、旧 CLI の `path.Ext`
// 相当の単一拡張子除去を再現する。
func stripExt(base string) string {
	ext := filepath.Ext(base)
	if ext == "" {
		return base
	}
	return base[:len(base)-len(ext)]
}
