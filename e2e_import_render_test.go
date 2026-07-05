// e2e_import_render_test.go は import → render の連携 E2E テスト（タスク 11.1）。
//
// 一時 SQLite ファイルから `runImport` で `.erdm` テキストを生成し、続けて
// `runRender` 経路に流し込んで複数形式（`.dot` / `.png` / `.html` / `.pg.sql` /
// `.sqlite3.sql`）の出力が生成できることを確認する（要件 1.1 / 1.3 / 9.1 / 9.2）。
//
// 外部描画コマンド（Graphviz `dot`）が不在の環境では `t.Skip` でテストをスキップ
// する（既存 `cmd_test.go` パターンを踏襲）。
package main

import (
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestE2E_SQLiteImportToRender は SQLite フィクスチャ → import → render の
// 連携を 1 関数で検証する。
//
// 手順:
//  1. 一時ディレクトリに SQLite フィクスチャ（`users` ＋ `articles`）を作成する。
//  2. `runImport([...])` で `.erdm` を生成する。
//  3. 生成された `.erdm` ファイルが空ファイルでないことを確認する。
//  4. `runRender([...])` で `.dot` / `.png` / `.html` / `.pg.sql` / `.sqlite3.sql` を
//     生成する（`dot` 不在時はスキップ）。
//  5. 5 種出力ファイルがそれぞれ非空サイズで生成されていることを確認する。
func TestE2E_SQLiteImportToRender(t *testing.T) {
	if _, err := exec.LookPath("dot"); err != nil {
		t.Skip("dot command not available; skipping e2e import→render test")
	}
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "fixture.db")
	createE2EFixtureDB(t, dbPath)

	erdmPath := filepath.Join(dir, "fixture.erdm")
	if err := runImport([]string{"--dsn=" + dbPath, "--out=" + erdmPath}); err != nil {
		t.Fatalf("runImport failed: %v", err)
	}
	assertNonEmptyFile(t, erdmPath)

	if err := runRender([]string{"-output_dir", dir, erdmPath}); err != nil {
		t.Fatalf("runRender failed: %v", err)
	}
	for _, ext := range []string{".dot", ".png", ".html", ".pg.sql", ".sqlite3.sql"} {
		assertNonEmptyFile(t, filepath.Join(dir, "fixture"+ext))
	}
}

// createE2EFixtureDB は SQLite ファイルへ `users` ＋ `articles` の最小スキーマを
// 投入する。本テストの目的は import → render 経路の連携であり、フィクスチャ DDL は
// 統合テスト（10.3）と重複しすぎないよう最小化する。
func createE2EFixtureDB(t *testing.T, dbPath string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	stmts := []string{
		`CREATE TABLE users (
            id INTEGER PRIMARY KEY,           -- 利用者ID
            name TEXT NOT NULL                -- 表示名
        )`,
		`CREATE TABLE articles (
            id INTEGER PRIMARY KEY,           -- 記事ID
            user_id INTEGER NOT NULL,
            title TEXT NOT NULL,              -- 題目
            FOREIGN KEY (user_id) REFERENCES users(id)
        )`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("exec ddl: %v", err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	// 防御: 作成直後にファイルがディスク上に存在することを確認する。
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("fixture db not created at %s: %v", dbPath, err)
	}
}
