// cmd_import_test.go は erdm CLI の import サブコマンド（runImport）の
// 主要ケースをユニットテストで固定する。
//
// 本テストは「引数受理」「出力経路選択」「親ディレクトリ存在検査」「実 SQLite
// 接続を伴う最小経路の往復」を担保する最低限のテスト。本格的な網羅は
// タスク 9.6（CLI 引数解析の本格テスト）／ 9.7（既存サブコマンド非干渉）の
// スコープ。
//
// 実 SQLite テストはピュア Go の modernc.org/sqlite を blank import で
// erdm.go から取り込んでいるため、CGO 不要・追加ツール不要で CI 上を走る。
package main

import (
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// TestRunImport_NoDSN_Usage は --dsn が空のときに usage 文字列を含むエラーが
// 返ることを確認する（要件 1.6）。`flag.ContinueOnError` で usage 出力は
// 標準エラーへ流す動作も `runImport` の `fs.SetOutput(os.Stderr)` で担保。
func TestRunImport_NoDSN_Usage(t *testing.T) {
	err := runImport(nil)
	if err == nil {
		t.Fatalf("runImport should fail with no --dsn")
	}
	if !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("error should contain usage text, got: %v", err)
	}
}

// TestRunImport_EmptyDSN_Usage は --dsn= の値が空のときも usage エラーが返ることを
// 確認する。
func TestRunImport_EmptyDSN_Usage(t *testing.T) {
	err := runImport([]string{"--dsn="})
	if err == nil {
		t.Fatalf("runImport should fail with empty --dsn")
	}
	if !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("error should contain usage text, got: %v", err)
	}
}

// TestRunImport_UnsupportedDriver は --driver で未知の値を渡した場合に
// ドライバ確定段階でエラーが返ることを確認する（要件 2.4）。
func TestRunImport_UnsupportedDriver(t *testing.T) {
	err := runImport([]string{"--dsn=any", "--driver=oracle"})
	if err == nil {
		t.Fatalf("runImport should fail for unsupported driver")
	}
	if !strings.Contains(err.Error(), "unsupported driver") {
		t.Fatalf("error should mention unsupported driver, got: %v", err)
	}
}

// TestRunImport_OutputDirNotFound は --out の親ディレクトリ不在時に
// 所定メッセージで非ゼロ終了相当のエラーを返すことを確認する（要件 11.4）。
func TestRunImport_OutputDirNotFound(t *testing.T) {
	dbPath := newSQLiteFixtureForCmd(t, []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`,
	})
	missingDir := filepath.Join(t.TempDir(), "no-such-dir")
	outPath := filepath.Join(missingDir, "out.erdm")
	err := runImport([]string{"--dsn=" + dbPath, "--out=" + outPath})
	if err == nil {
		t.Fatalf("runImport should fail when output parent directory does not exist")
	}
	if !strings.Contains(err.Error(), "output directory not found") {
		t.Fatalf("error should mention 'output directory not found', got: %v", err)
	}
}

// TestRunImport_SQLite_OutFile_WritesNonEmpty は --out 指定 + SQLite 実接続経路で
// `.erdm` ファイルが書き出されることを確認する（要件 1.3 / 12.3）。
func TestRunImport_SQLite_OutFile_WritesNonEmpty(t *testing.T) {
	dbPath := newSQLiteFixtureForCmd(t, []string{
		`CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL
		)`,
	})
	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "schema.erdm")

	if err := runImport([]string{"--dsn=" + dbPath, "--out=" + outPath}); err != nil {
		t.Fatalf("runImport failed: %v", err)
	}
	st, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("stat %s: %v", outPath, err)
	}
	if st.Size() == 0 {
		t.Fatalf("%s is empty", outPath)
	}
	// 書き出された内容にテーブル名 `users` が含まれることを確認（最低限の往復確認）。
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read %s: %v", outPath, err)
	}
	if !strings.Contains(string(data), "users") {
		t.Fatalf("output should contain table name 'users'; got:\n%s", string(data))
	}
}

// TestRunImport_SQLite_StdoutWhenNoOut は --out 未指定時に標準出力へ erdm が
// 書き出されることを確認する（要件 1.4）。
func TestRunImport_SQLite_StdoutWhenNoOut(t *testing.T) {
	dbPath := newSQLiteFixtureForCmd(t, []string{
		`CREATE TABLE items (id INTEGER PRIMARY KEY, label TEXT)`,
	})
	captured := captureStdout(t, func() {
		if err := runImport([]string{"--dsn=" + dbPath}); err != nil {
			t.Fatalf("runImport failed: %v", err)
		}
	})
	if len(captured) == 0 {
		t.Fatalf("stdout is empty for runImport without --out")
	}
	if !strings.Contains(string(captured), "items") {
		t.Fatalf("stdout should contain table name 'items'; got:\n%s", string(captured))
	}
}

// TestRunImport_TitleOverride は --title 明示指定時にスキーマタイトルが
// その値になることを確認する（要件 9.5）。
func TestRunImport_TitleOverride(t *testing.T) {
	dbPath := newSQLiteFixtureForCmd(t, []string{
		`CREATE TABLE t (id INTEGER PRIMARY KEY)`,
	})
	captured := captureStdout(t, func() {
		if err := runImport([]string{"--dsn=" + dbPath, "--title=Override"}); err != nil {
			t.Fatalf("runImport failed: %v", err)
		}
	})
	if !strings.Contains(string(captured), "Override") {
		t.Fatalf("stdout should contain title 'Override'; got:\n%s", string(captured))
	}
}

// --- 以下、タスク 9.6 で追加した網羅ケース ---

// TestRunImport_DriverOverrideMismatchesDSN は `--driver=mysql` と
// `--dsn=postgres://...` を組み合わせたとき、明示指定の driver=mysql が
// 優先採用された結果として MySQL DSN パース段階でエラーになることを確認する
// （要件 1.5：明示指定 > 推定）。本テストは「PG の URL を MySQL として処理した
// 場合の失敗モード」を捉えることで、明示指定が DSN プレフィックスより
// 優先されるという契約を担保する。
func TestRunImport_DriverOverrideMismatchesDSN(t *testing.T) {
	err := runImport([]string{
		"--dsn=postgres://user:secret@host:5432/db",
		"--driver=mysql",
	})
	if err == nil {
		t.Fatalf("runImport should fail when --driver=mysql is forced on a postgres:// DSN")
	}
	// `parseMySQLDSN` は go-sql-driver/mysql の `ParseDSN` 経由でエラーを返す
	// ため、エラー文言には「mysql DSN」相当の語が含まれる。
	msg := err.Error()
	if !strings.Contains(msg, "mysql") {
		t.Fatalf("error %q should mention mysql to indicate driver=mysql was honored", msg)
	}
	// 原 DSN のパスワード "secret" が含まれないことを確認（要件 10.4）。
	if strings.Contains(msg, "secret") {
		t.Fatalf("error message leaked password: %v", err)
	}
}

// TestRunImport_SchemaFlagIsHarmlessOnSQLite は `--schema` を SQLite で
// 指定しても処理が成功し、テーブルが取得されることを確認する（要件 3.4）。
// SQLite はスキーマ概念を持たないため、`--schema` の値は introspector で
// 無視される設計（design.md §C2 / introspect.go fetchRawTables）。
func TestRunImport_SchemaFlagIsHarmlessOnSQLite(t *testing.T) {
	dbPath := newSQLiteFixtureForCmd(t, []string{
		`CREATE TABLE products (id INTEGER PRIMARY KEY, sku TEXT NOT NULL)`,
	})
	captured := captureStdout(t, func() {
		err := runImport([]string{"--dsn=" + dbPath, "--schema=ignored_for_sqlite"})
		if err != nil {
			t.Fatalf("runImport should succeed when --schema is given on SQLite: %v", err)
		}
	})
	if !strings.Contains(string(captured), "products") {
		t.Fatalf("output should contain table name 'products' regardless of --schema; got:\n%s", captured)
	}
}

// TestRunImport_OutFile_BytesMatchStdout は `--out` 指定時にファイルへ
// 書き出された内容が、同 DSN を `--out` 未指定で実行した際の標準出力結果と
// バイト一致することを確認する（要件 1.3 / 1.4）。
//
// 出力経路（標準出力 vs ファイル）が分岐するだけで、生成内容は同じである
// という契約（design.md §"runImport"）を回帰検出する。
func TestRunImport_OutFile_BytesMatchStdout(t *testing.T) {
	dbPath := newSQLiteFixtureForCmd(t, []string{
		`CREATE TABLE items (id INTEGER PRIMARY KEY, label TEXT)`,
	})

	stdoutBytes := captureStdout(t, func() {
		if err := runImport([]string{"--dsn=" + dbPath}); err != nil {
			t.Fatalf("runImport (stdout) failed: %v", err)
		}
	})

	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "schema.erdm")
	if err := runImport([]string{"--dsn=" + dbPath, "--out=" + outPath}); err != nil {
		t.Fatalf("runImport (out file) failed: %v", err)
	}
	fileBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read %s: %v", outPath, err)
	}
	if string(stdoutBytes) != string(fileBytes) {
		t.Fatalf("stdout vs file output mismatch:\nstdout:\n%s\nfile:\n%s", stdoutBytes, fileBytes)
	}
}

// TestRunImport_TitleOverridesDSNDerivedTitle は `--title` 明示指定が
// SQLite DSN のファイル名ベース由来のタイトル（fixture）より優先されることを
// 確認する（要件 9.5）。既存 TestRunImport_TitleOverride は出力に "Override" が
// 含まれることのみ確認するが、本テストは「DSN 由来の値（fixture）が
// タイトルとして採用されないこと」を併せて検証する。
func TestRunImport_TitleOverridesDSNDerivedTitle(t *testing.T) {
	dbPath := newSQLiteFixtureForCmd(t, []string{
		`CREATE TABLE t (id INTEGER PRIMARY KEY)`,
	})
	captured := captureStdout(t, func() {
		if err := runImport([]string{"--dsn=" + dbPath, "--title=ExplicitTitle"}); err != nil {
			t.Fatalf("runImport failed: %v", err)
		}
	})
	out := string(captured)
	if !strings.Contains(out, "Title: ExplicitTitle") {
		t.Fatalf("output should contain '# Title: ExplicitTitle'; got:\n%s", out)
	}
	// SQLite DSN のファイル名ベースは `fixture`（newSQLiteFixtureForCmd の定義）。
	// title 行が "fixture" になっていないことを確認することで、DSN 由来の値が
	// 明示指定によって上書きされたことを担保する。
	if strings.Contains(out, "Title: fixture") {
		t.Fatalf("output should NOT contain DSN-derived title 'Title: fixture'; got:\n%s", out)
	}
}

// TestRunImport_UnknownFlag_WritesUsageToStderr は未知フラグを渡したときに
// `flag.ContinueOnError` が標準エラーへ usage を出力することを
// `captureStderr` で確認する（要件 1.6 のフィードバック品質）。
//
// `runImport` は `fs.SetOutput(os.Stderr)` を設定しており、`flag` パッケージは
// パース失敗時に「flag provided but not defined」とデフォルト usage を
// 出力先（os.Stderr）へ書き出す。本テストはこの配線が壊れていないことを
// 回帰検出する。
func TestRunImport_UnknownFlag_WritesUsageToStderr(t *testing.T) {
	read, restore := captureStderrForCmd(t)
	defer restore()
	err := runImport([]string{"--unknown-flag=value"})
	got := read()
	if err == nil {
		t.Fatalf("runImport should fail for unknown flag")
	}
	if !strings.Contains(got, "flag provided but not defined") {
		t.Fatalf("stderr should contain flag parse error, got: %q", got)
	}
}

// TestRunImport_OutFile_NoWritePermission は `--out` の親ディレクトリは存在
// するが書き込み権限がない場合に `runImport` がエラーを返すことを確認する
// （要件 11.1：書き出し失敗時の非ゼロ終了）。
//
// 権限制御テストは Unix 系のファイルシステムを前提とするため、Windows と
// `root` 実行時はスキップする。`runImport` は `os.WriteFile` の失敗を
// 「import: write <path>: ...」形式でラップして返すため、本テストはエラー
// メッセージにそのプレフィックスが含まれることで分岐成立を確認する。
func TestRunImport_OutFile_NoWritePermission(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on Windows; skipping")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root: file mode permission checks bypassed; skipping")
	}
	dbPath := newSQLiteFixtureForCmd(t, []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`,
	})
	readOnlyDir := t.TempDir()
	if err := os.Chmod(readOnlyDir, 0o555); err != nil {
		t.Fatalf("chmod readonly dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnlyDir, 0o755) })

	outPath := filepath.Join(readOnlyDir, "out.erdm")
	err := runImport([]string{"--dsn=" + dbPath, "--out=" + outPath})
	if err == nil {
		t.Fatalf("runImport should fail when output parent directory is read-only")
	}
	if !strings.Contains(err.Error(), "import: write") {
		t.Fatalf("error should start with 'import: write' to indicate file write phase, got: %v", err)
	}
}

// captureStderrForCmd は os.Stderr を一時的にパイプに差し替え、テスト中に
// 書き込まれた内容を文字列として取得するためのテストヘルパ（main パッケージ用）。
//
// internal/introspect の captureStderr と意図は同一だが、パッケージ境界を
// またげないため独自実装する（DRY 違反は許容、testutil への移動は YAGNI で
// 持ち越し）。
func captureStderrForCmd(t *testing.T) (read func() string, restore func()) {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w

	var (
		mu       sync.Mutex
		captured strings.Builder
		done     = make(chan struct{})
	)
	go func() {
		defer close(done)
		buf, _ := io.ReadAll(r)
		mu.Lock()
		captured.Write(buf)
		mu.Unlock()
	}()

	read = func() string {
		_ = w.Close()
		<-done
		mu.Lock()
		defer mu.Unlock()
		return captured.String()
	}
	restore = func() {
		os.Stderr = orig
		_ = r.Close()
	}
	return read, restore
}

// newSQLiteFixtureForCmd は一時ディレクトリに SQLite ファイルを作成し、
// 渡された DDL 群を実行した状態でファイルパスを返す共通ヘルパ。
//
// modernc.org/sqlite は erdm.go の blank import 経由で main パッケージ
// テストにも登録されるため、本テストファイル内で `sql.Open("sqlite", ...)` が
// 利用できる。
func newSQLiteFixtureForCmd(t *testing.T, ddls []string) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "fixture.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, ddl := range ddls {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("exec ddl %q: %v", ddl, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	return dbPath
}
