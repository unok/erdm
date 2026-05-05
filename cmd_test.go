// cmd_test.go は erdm CLI のサブコマンド（render/serve）のユニットテスト。
//
// Requirements: 3.5, 4.1, 9.1, 10.1
package main

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unok/erdm/internal/testutil/fixtures"
)

// TestRunRender_DOT_AllFixtures は doc/sample 配下の全サンプルに対し
// `runRender` を呼び出し、5 種出力ファイルが生成されることを確認する（要件 3.5）。
//
// `dot` 不在環境では `runRender` 自体が早期エラーを返すためテスト全体をスキップする。
func TestRunRender_DOT_AllFixtures(t *testing.T) {
	if _, err := exec.LookPath("dot"); err != nil {
		t.Skip("dot command not available; skipping integration test")
	}
	for _, name := range fixtures.NamesAll() {
		t.Run(name, func(t *testing.T) {
			tmpDir := t.TempDir()
			inputPath := writeFixtureToTempFile(t, name)

			if err := runRender([]string{"-output_dir", tmpDir, inputPath}); err != nil {
				t.Fatalf("runRender failed: %v", err)
			}
			for _, ext := range []string{".dot", ".png", ".html", ".pg.sql", ".sqlite3.sql"} {
				assertNonEmptyFile(t, filepath.Join(tmpDir, name+ext))
			}
		})
	}
}

// TestRunRender_DOT_FormatExplicit は `--format=dot` を明示指定しても
// 既定動作と同じ 5 種出力が生成されることを確認する（要件 4.1）。
func TestRunRender_DOT_FormatExplicit(t *testing.T) {
	if _, err := exec.LookPath("dot"); err != nil {
		t.Skip("dot command not available; skipping integration test")
	}
	tmpDir := t.TempDir()
	inputPath := writeFixtureToTempFile(t, "test")

	if err := runRender([]string{"-output_dir", tmpDir, "--format=dot", inputPath}); err != nil {
		t.Fatalf("runRender failed: %v", err)
	}
	for _, ext := range []string{".dot", ".png", ".html", ".pg.sql", ".sqlite3.sql"} {
		assertNonEmptyFile(t, filepath.Join(tmpDir, "test"+ext))
	}
}

// TestRunRender_NonExistentInput_Error は存在しない入力ファイル指定時に
// runRender がエラーを返すことを確認する（要件 10.1）。
//
// `dot` 不在環境ではこのチェック自体が `dot` 不存在エラーで返るため、
// 不在環境ではスキップする。
func TestRunRender_NonExistentInput_Error(t *testing.T) {
	if _, err := exec.LookPath("dot"); err != nil {
		t.Skip("dot command not available; skipping integration test")
	}
	tmpDir := t.TempDir()
	missing := filepath.Join(tmpDir, "no_such_file.erdm")
	err := runRender([]string{"-output_dir", tmpDir, missing})
	if err == nil {
		t.Fatalf("runRender should fail for non-existent input")
	}
	if !strings.Contains(err.Error(), "input file") {
		t.Fatalf("error should mention input file, got: %v", err)
	}
}

// TestRunRender_ELK_FileOutput は `-output_dir` を明示指定した状態で
// `--format=elk` を実行すると `<outputDir>/<basename>.elk.json` が
// 生成され、内容が JSON として有効であることを既存サンプル全件で確認する
// （要件 4.1）。
func TestRunRender_ELK_FileOutput(t *testing.T) {
	for _, name := range fixtures.NamesAll() {
		t.Run(name, func(t *testing.T) {
			tmpDir := t.TempDir()
			inputPath := writeFixtureToTempFile(t, name)

			if err := runRender([]string{"-output_dir", tmpDir, "--format=elk", inputPath}); err != nil {
				t.Fatalf("runRender failed: %v", err)
			}
			outPath := filepath.Join(tmpDir, name+".elk.json")
			data, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatalf("read %s: %v", outPath, err)
			}
			if len(data) == 0 {
				t.Fatalf("%s is empty", outPath)
			}
			var v any
			if err := json.Unmarshal(data, &v); err != nil {
				t.Fatalf("output %s is not valid JSON: %v", outPath, err)
			}
		})
	}
}

// TestRunRender_ELK_Stdout は `-output_dir` 未指定（既定値）で
// `--format=elk` を実行すると標準出力へ JSON が書き出されることを確認する
// （要件 4.1）。`os.Pipe` で `os.Stdout` を一時的に置換してキャプチャする。
func TestRunRender_ELK_Stdout(t *testing.T) {
	inputPath := writeFixtureToTempFile(t, "test")

	captured := captureStdout(t, func() {
		if err := runRender([]string{"--format=elk", inputPath}); err != nil {
			t.Fatalf("runRender failed: %v", err)
		}
	})
	if len(captured) == 0 {
		t.Fatalf("stdout is empty for --format=elk without -output_dir")
	}
	var v any
	if err := json.Unmarshal(captured, &v); err != nil {
		t.Fatalf("stdout is not valid JSON: %v", err)
	}
}

// TestRunRender_ELK_NoDotRequired は ELK 形式が `dot` コマンドを必要と
// しないことを確認する（要件 9.4）。`PATH` を空にして `dot` を引けない
// 状態を作っても `runRender` が成功する。
func TestRunRender_ELK_NoDotRequired(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := writeFixtureToTempFile(t, "test")

	origPath, hadPath := os.LookupEnv("PATH")
	t.Setenv("PATH", "")
	t.Cleanup(func() {
		if hadPath {
			_ = os.Setenv("PATH", origPath)
			return
		}
		_ = os.Unsetenv("PATH")
	})

	if _, err := exec.LookPath("dot"); err == nil {
		t.Fatalf("PATH cleanup did not hide dot; test precondition violated")
	}
	if err := runRender([]string{"-output_dir", tmpDir, "--format=elk", inputPath}); err != nil {
		t.Fatalf("runRender should succeed without dot for --format=elk: %v", err)
	}
	assertNonEmptyFile(t, filepath.Join(tmpDir, "test.elk.json"))
}

// TestRunRender_ELK_ParseError は不正な `.erdm` 内容で `--format=elk` が
// パースエラーを返すことを確認する。
func TestRunRender_ELK_ParseError(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "broken.erdm")
	if err := os.WriteFile(inputPath, []byte("this is not a valid erdm file"), 0644); err != nil {
		t.Fatalf("write broken fixture: %v", err)
	}
	err := runRender([]string{"-output_dir", tmpDir, "--format=elk", inputPath})
	if err == nil {
		t.Fatalf("runRender should fail for invalid erdm input")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Fatalf("error should mention parse, got: %v", err)
	}
}

// TestRunRender_UnknownFormat_Error は未知のフォーマット指定時にエラーが返ることを確認する。
func TestRunRender_UnknownFormat_Error(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := writeFixtureToTempFile(t, "test")
	err := runRender([]string{"-output_dir", tmpDir, "--format=svg", inputPath})
	if err == nil {
		t.Fatalf("runRender should fail for unknown format")
	}
	if !strings.Contains(err.Error(), "unknown format") {
		t.Fatalf("error should mention unknown format, got: %v", err)
	}
}

// TestRunRender_NoArgs_Usage は引数なしで usage を返すことを確認する。
func TestRunRender_NoArgs_Usage(t *testing.T) {
	err := runRender(nil)
	if err == nil {
		t.Fatalf("runRender should fail with no args")
	}
	if !strings.Contains(err.Error(), "Usage:") {
		t.Fatalf("error should contain usage text, got: %v", err)
	}
}

// TestRunServe_InvalidPortFlag_Error は引数解析が flag.ContinueOnError で
// 不正な数値を弾くことを確認する（serve サブコマンドの解析ルートが本実装後も
// 機能していることを担保する）。
func TestRunServe_InvalidPortFlag_Error(t *testing.T) {
	inputPath := writeFixtureToTempFile(t, "test")
	err := runServe([]string{"--port=not-a-number", inputPath})
	if err == nil {
		t.Fatalf("runServe should fail for invalid --port value")
	}
}

// TestRunServe_NoArgs_Usage は serve サブコマンドが入力ファイルなしで usage を返すことを確認する。
func TestRunServe_NoArgs_Usage(t *testing.T) {
	err := runServe(nil)
	if err == nil {
		t.Fatalf("runServe should fail with no args")
	}
	if !strings.Contains(err.Error(), "Usage:") {
		t.Fatalf("error should contain usage text, got: %v", err)
	}
}

// TestRunServe_NonExistentInput_Error は serve でも入力ファイル不存在時に
// エラーが返ることを確認する（要件 10.1）。
func TestRunServe_NonExistentInput_Error(t *testing.T) {
	tmpDir := t.TempDir()
	missing := filepath.Join(tmpDir, "no_such_file.erdm")
	err := runServe([]string{missing})
	if err == nil {
		t.Fatalf("runServe should fail for non-existent input")
	}
	if !strings.Contains(err.Error(), "input file") {
		t.Fatalf("error should mention input file, got: %v", err)
	}
}

// TestStripExt は単一拡張子の除去を検証する。
func TestStripExt(t *testing.T) {
	cases := []struct{ in, want string }{
		{"foo.erdm", "foo"},
		{"foo", "foo"},
		{"a.b.c", "a.b"},
	}
	for _, c := range cases {
		if got := stripExt(c.in); got != c.want {
			t.Fatalf("stripExt(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

// writeFixtureToTempFile は fixtures.LoadFixture でサンプルを取得し、
// 一時ディレクトリに `<name>.erdm` として書き出してパスを返す。
func writeFixtureToTempFile(t *testing.T, name string) string {
	t.Helper()
	data, err := fixtures.LoadFixture(name)
	if err != nil {
		t.Fatalf("LoadFixture(%q): %v", name, err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, name+".erdm")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
	return path
}

// assertNonEmptyFile は path が存在し、サイズが 0 でないことを保証する。
func assertNonEmptyFile(t *testing.T, path string) {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if st.Size() == 0 {
		t.Fatalf("%s is empty", path)
	}
}

// captureStdout は fn の実行中に os.Stdout へ書き出されたバイト列を返す。
//
// `--format=elk` の標準出力ブランチを検証するため、`os.Pipe` で
// 書き込み側を `os.Stdout` に差し替え、終了後に元へ戻す。
func captureStdout(t *testing.T, fn func()) []byte {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan []byte, 1)
	go func() {
		buf, _ := io.ReadAll(r)
		done <- buf
	}()

	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	return <-done
}
