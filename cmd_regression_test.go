// cmd_regression_test.go はタスク 9.7（既存サブコマンド非干渉の回帰テスト）に
// 対応する。要件 1.7「既存の render／serve サブコマンドの引数受理・出力動作が
// import サブコマンド追加によって変化しないこと」を、3 つのエントリ関数
// （runImport ／ runRender ／ runServe）が flag・出力・状態を一切共有しない
// 独立した黒箱であることの確認で担保する。
//
// 直接的な「main 関数が args[0] によって正しいエントリへ振り分けられる」
// 経路の検証は subprocess 化が必要で重いため、本テストでは下記 2 つの
// 回帰検出で代替する:
//   1. 各エントリ関数が他サブコマンドのフラグを受理しないこと（flag 衝突なし）
//   2. 既存サンプル `.erdm` の render 出力が決定論的（同入力 → 同バイト出力）で
//      あること（出力経路に新たな副作用が混入していないこと）
//
// 既存 cmd_test.go ／ cmd_compat_test.go の全ケースが green であることは
// `go test -count=1 ./...` の通常実行で担保される（プロジェクトポリシー）。

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unok/erdm/internal/testutil/fixtures"
)

// TestRegression_RunImport_RejectsRenderFlag は import エントリが render の
// `-output_dir` フラグを受理しないことを確認する。flag.ContinueOnError と
// 個別 FlagSet 設計（design.md §C1）により、サブコマンド間で flag 名が
// 漏洩しないことを回帰検出する（要件 1.7）。
func TestRegression_RunImport_RejectsRenderFlag(t *testing.T) {
	err := runImport([]string{"--dsn=foo.db", "-output_dir=/tmp"})
	if err == nil {
		t.Fatalf("runImport should reject render's -output_dir flag")
	}
	if !strings.Contains(err.Error(), "flag provided but not defined") &&
		!strings.Contains(err.Error(), "not defined") {
		t.Fatalf("error should indicate undefined flag, got: %v", err)
	}
}

// TestRegression_RunImport_RejectsServeFlag は import エントリが serve の
// `--port` フラグを受理しないことを確認する（要件 1.7）。
func TestRegression_RunImport_RejectsServeFlag(t *testing.T) {
	err := runImport([]string{"--dsn=foo.db", "--port=9999"})
	if err == nil {
		t.Fatalf("runImport should reject serve's --port flag")
	}
	if !strings.Contains(err.Error(), "not defined") {
		t.Fatalf("error should indicate undefined flag, got: %v", err)
	}
}

// TestRegression_RunRender_RejectsImportFlag は render エントリが import の
// `--dsn` フラグを受理しないことを確認する。import 追加によって render の
// flag 解釈に新規フラグが混入していないことを担保する（要件 1.7）。
func TestRegression_RunRender_RejectsImportFlag(t *testing.T) {
	err := runRender([]string{"--dsn=anything", "schema.erdm"})
	if err == nil {
		t.Fatalf("runRender should reject import's --dsn flag")
	}
	if !strings.Contains(err.Error(), "not defined") {
		t.Fatalf("error should indicate undefined flag, got: %v", err)
	}
}

// TestRegression_RunServe_RejectsImportFlag は serve エントリが import の
// `--dsn` フラグを受理しないことを確認する（要件 1.7）。
func TestRegression_RunServe_RejectsImportFlag(t *testing.T) {
	err := runServe([]string{"--dsn=anything", "schema.erdm"})
	if err == nil {
		t.Fatalf("runServe should reject import's --dsn flag")
	}
	if !strings.Contains(err.Error(), "not defined") {
		t.Fatalf("error should indicate undefined flag, got: %v", err)
	}
}

// TestRegression_RunImport_NoSideEffectOnRenderArtifacts は import 経路で
// render 系成果物（.dot ／ .png ／ .html ／ .pg.sql ／ .sqlite3.sql）が
// 生成されないことを確認する（要件 1.7）。
//
// 一時ディレクトリ内に SQLite DSN ファイルと `--out` パスを置いて runImport
// を実行し、出力ディレクトリ内に erdm 以外のレンダリング成果物が現れない
// ことを検証する。これにより runImport が runRender の出力経路へ流入する
// 配線ミスを回帰検出する。
func TestRegression_RunImport_NoSideEffectOnRenderArtifacts(t *testing.T) {
	dbPath := newSQLiteFixtureForCmd(t, []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY)`,
	})
	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "schema.erdm")

	if err := runImport([]string{"--dsn=" + dbPath, "--out=" + outPath}); err != nil {
		t.Fatalf("runImport failed: %v", err)
	}

	// 生成されてはならない render 系拡張子の集合。
	for _, ext := range []string{".dot", ".png", ".html", ".pg.sql", ".sqlite3.sql"} {
		matches, err := filepath.Glob(filepath.Join(outDir, "*"+ext))
		if err != nil {
			t.Fatalf("glob %s: %v", ext, err)
		}
		if len(matches) > 0 {
			t.Errorf("runImport unexpectedly produced render artifacts: %v", matches)
		}
	}
}

// TestRegression_RunRender_DeterministicBytesAcrossRuns は同一サンプルに対する
// `runRender` の `.dot` 出力が複数回実行でバイト一致することを確認する
// （要件 1.7：既存 render の出力動作が変化しない）。
//
// `dot` 不在環境では runRender 全体が早期エラーで返るためテスト全体を
// スキップする。本テストは「import 追加によって render の出力に偶発的
// 副作用（タイムスタンプ混入・並び順変動など）が混入していないこと」を
// 担保する最小回帰チェック。
func TestRegression_RunRender_DeterministicBytesAcrossRuns(t *testing.T) {
	if _, err := exec.LookPath("dot"); err != nil {
		t.Skip("dot command not available; skipping integration test")
	}
	for _, name := range fixtures.NamesAll() {
		t.Run(name, func(t *testing.T) {
			inputPath := writeFixtureToTempFile(t, name)

			runOnce := func() []byte {
				dir := t.TempDir()
				if err := runRender([]string{"-output_dir", dir, inputPath}); err != nil {
					t.Fatalf("runRender failed: %v", err)
				}
				dotBytes, err := os.ReadFile(filepath.Join(dir, name+".dot"))
				if err != nil {
					t.Fatalf("read dot: %v", err)
				}
				return dotBytes
			}
			first := runOnce()
			second := runOnce()
			if !bytes.Equal(first, second) {
				t.Fatalf("render output for %s changed between runs:\nfirst:\n%s\nsecond:\n%s",
					name, first, second)
			}
		})
	}
}
