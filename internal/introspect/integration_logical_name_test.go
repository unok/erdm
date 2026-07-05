// integration_logical_name_test.go は 3 DBMS 横断で「コメント未設定の物理テーブル
// が論理名欄に物理名を採用した形式で出力される」ことを保証する統合テスト
// （タスク 10.4）。
//
// 本テストはユーザー指示「基本コメントは論理名になるように」（要件 8.6）の
// 機能保証点として責務を独立させて配置する。10.1 ／ 10.2 ／ 10.3 のメインケース
// テストでもフォールバックの確認は行うが、本ファイルは 1 つの目的に責務を
// 集中させる（00-plan.md §"Task 10.4"）。
//
// 検証戦略:
//   - 各 DBMS のフィクスチャに「コメント完全未設定の `*_no_comment_*` テーブル」を
//     用意し、その出力が `<phys>/<phys>` の物理名フォールバック形式となることを
//     アサートする。
//   - SQLite は常に走り（Docker 不要）、PG / MySQL は環境変数 DSN が設定されて
//     いるときに走る（未設定時は `t.Skip`）。
//   - サブテストで DBMS を分割し、1 つの DBMS が走らなくとも他は独立に走る。
package introspect

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unok/erdm/internal/model"
	"github.com/unok/erdm/internal/serializer"
)

// TestIntegration_LogicalNameFallback_AllDBMS は 3 DBMS 横断のフォールバック保証。
//
// 期待される出力パターン: `<phys>/<phys>` をテーブル宣言行と各カラム宣言行の
// 双方で確認する。
func TestIntegration_LogicalNameFallback_AllDBMS(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		t.Parallel()
		out := sqliteFallbackOutput(t)
		assertLogicalNameFallback(t, out, "i_fb_no_comment")
	})
	t.Run("postgres", func(t *testing.T) {
		t.Parallel()
		dsn := requireEnv(t, "ERDM_TEST_POSTGRES_DSN")
		loadPGFixture(t, dsn, `
DROP TABLE IF EXISTS i_fb_no_comment_pg CASCADE;
CREATE TABLE i_fb_no_comment_pg (
    id bigserial PRIMARY KEY,
    label text
);
`)
		schema, err := Introspect(context.Background(), Options{
			Driver: DriverPostgreSQL,
			DSN:    dsn,
			Title:  "fb_pg",
		})
		if err != nil {
			t.Fatalf("Introspect failed: %v", err)
		}
		assertLogicalNameFallback(t, mustSerialize(t, schema), "i_fb_no_comment_pg")
	})
	t.Run("mysql", func(t *testing.T) {
		t.Parallel()
		dsn := requireEnv(t, "ERDM_TEST_MYSQL_DSN")
		loadMySQLFixture(t, dsn, `
DROP TABLE IF EXISTS i_fb_no_comment_my;
CREATE TABLE i_fb_no_comment_my (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    label VARCHAR(255)
);
`)
		schema, err := Introspect(context.Background(), Options{
			Driver: DriverMySQL,
			DSN:    dsn,
			Title:  "fb_mysql",
		})
		if err != nil {
			t.Fatalf("Introspect failed: %v", err)
		}
		assertLogicalNameFallback(t, mustSerialize(t, schema), "i_fb_no_comment_my")
	})
}

// sqliteFallbackOutput は SQLite で「コメント完全未設定」のフィクスチャを構築し、
// `Introspect → Serialize` の出力テキストを返す。
//
// SQLite はテーブルコメント機構を持たないため、CREATE TABLE 文に `--` コメントを
// 一切置かないことで「コメント抽出不能 → 物理名フォールバック」のケースを再現する。
func sqliteFallbackOutput(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "fb.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE i_fb_no_comment (
    id INTEGER PRIMARY KEY,
    label TEXT
)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	schema, err := Introspect(context.Background(), Options{
		Driver: DriverSQLite,
		DSN:    dbPath,
		Title:  "fb_sqlite",
	})
	if err != nil {
		t.Fatalf("Introspect failed: %v", err)
	}
	return mustSerialize(t, schema)
}

// assertLogicalNameFallback は出力テキスト中の対象テーブルが `<phys>/<phys>` 形式で
// 出力されており、テーブル宣言行とカラム宣言行のいずれも論理名が物理名と一致する
// ことを確認する（要件 8.6）。
func assertLogicalNameFallback(t *testing.T, output, table string) {
	t.Helper()
	tableDecl := table + "/" + table
	if !strings.Contains(output, tableDecl) {
		t.Fatalf("table declaration %q not found.\n--- output ---\n%s", tableDecl, output)
	}
	for _, want := range []string{"id/id", "label/label"} {
		if !strings.Contains(output, want) {
			t.Fatalf("column logical fallback %q not found in table %q.\n--- output ---\n%s", want, table, output)
		}
	}
}

// mustSerialize は schema を serializer.Serialize で文字列化する。失敗時は
// テストを停止する。
func mustSerialize(t *testing.T, schema *model.Schema) string {
	t.Helper()
	if err := schema.Validate(); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	got, err := serializer.Serialize(schema)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}
	return string(got)
}

// requireEnv は環境変数 key の値を返す。未設定なら `t.Skip` で停止する。
// 統合テストの「環境を持っていれば走る／無ければスキップ」セマンティクスを
// 集約する共通ヘルパ。
func requireEnv(t *testing.T, key string) string {
	t.Helper()
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		t.Skipf("%s not set; skipping integration test", key)
	}
	return v
}
