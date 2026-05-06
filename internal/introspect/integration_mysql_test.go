// integration_mysql_test.go は稼働中の MySQL に対するイントロスペクション統合
// テスト（タスク 10.2）。
//
// 実 DB 起動の取り扱い:
//   - 環境変数 `ERDM_TEST_MYSQL_DSN` が設定されているときのみ走る。未設定時は
//     `t.Skip` で自動スキップする。
//   - 採用方針の根拠は coder-decisions.md §"testcontainers-go を採用しない決定"
//     を参照。
//
// 検証点（tasks.md / 00-plan.md §"Task 10.2"）:
//   - フィクスチャ DDL を投入し、Introspect → Serialize の出力が期待値と一致する。
//   - 既存パーサ往復で冪等性を持つ。
//   - 読み取り専用 TX のもとで意図せぬ書き込みが発生していない。
//   - コメント取得経路（COLUMN_COMMENT / TABLE_COMMENT）が要件どおり動作する。
package introspect

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	// テスト専用 blank import: integration_mysql_test.go はテストビルドでのみ
	// go-sql-driver/mysql を `database/sql.Open("mysql", ...)` 用に登録する。
	_ "github.com/go-sql-driver/mysql"

	"github.com/unok/erdm/internal/parser"
	"github.com/unok/erdm/internal/serializer"
)

// myFixtureDDL は MySQL 統合テストで投入するスキーマ。
//
// 設計意図:
//   - 既存ゴミデータと衝突しないよう FK 子→親順で `DROP TABLE IF EXISTS` を発行
//     してからフィクスチャを再構築する。
//   - `i_my_users`: AUTO_INCREMENT PK + UNIQUE 制約 + テーブル/カラム COMMENT
//     → 要件 4.4 ／ 4.7 ／ 8.2 を網羅。
//   - `i_my_articles`: 単一 FK + 補助インデックス + テーブル COMMENT
//     → 要件 6.x ／ 7.x ／ 8.2 を網羅。
//   - `i_my_article_tags`: 複合 PK と複合 FK
//     → 要件 5.x ／ 6.4 を網羅。
//   - `i_my_tags`: テーブル / カラム COMMENT 未設定 → 物理名フォールバック（要件 8.6）。
const myFixtureDDL = `
DROP TABLE IF EXISTS i_my_article_tags;
DROP TABLE IF EXISTS i_my_articles;
DROP TABLE IF EXISTS i_my_users;
DROP TABLE IF EXISTS i_my_tags;

CREATE TABLE i_my_users (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY COMMENT '利用者ID',
    email VARCHAR(255) NOT NULL UNIQUE COMMENT 'メールアドレス',
    name VARCHAR(255) NOT NULL COMMENT '表示名'
) COMMENT='利用者';

CREATE TABLE i_my_articles (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY COMMENT '記事ID',
    user_id BIGINT NOT NULL,
    title VARCHAR(255) NOT NULL COMMENT '題目',
    body TEXT,
    INDEX i_my_articles_user_title (user_id, title),
    FOREIGN KEY (user_id) REFERENCES i_my_users(id)
) COMMENT='記事';

CREATE TABLE i_my_article_tags (
    article_id BIGINT NOT NULL,
    tag VARCHAR(255) NOT NULL COMMENT 'タグ名',
    PRIMARY KEY (article_id, tag),
    FOREIGN KEY (article_id) REFERENCES i_my_articles(id)
) COMMENT='記事タグ';

CREATE TABLE i_my_tags (
    name VARCHAR(255) NOT NULL PRIMARY KEY,
    description TEXT
);
`

// expectedMyErdm は myFixtureDDL から生成される `.erdm` テキストの期待値。
//
// MySQL 仕様メモ:
//   - PK カラムは PRIMARY KEY 制約により暗黙 NOT NULL。serializer はそのまま `[NN]` を出力。
//   - AUTO_INCREMENT の Default は要件 4.7 で抑止される。
//   - `COLUMN_TYPE` を採用するため `bigint` と `varchar(255)` のように長さ情報も保持される
//     （要件 4.2）。
//   - `i_my_tags` のテーブル／カラム COMMENT 未設定は物理名フォールバック（要件 8.6）。
//   - テーブル順序は MySQL の `information_schema.tables` 規定順（通常 ASCII 名順）。
//     i_my_article_tags → i_my_articles → i_my_tags → i_my_users の昇順となる。
const expectedMyErdm = `# Title: integration_mysql

i_my_article_tags/記事タグ
    +article_id/article_id [bigint][NN] 1..*--1 i_my_articles
    +tag/タグ名 [varchar(255)][NN]

i_my_articles/記事
    +id/記事ID [bigint][NN]
    user_id/user_id [bigint][NN] 1..*--1 i_my_users
    title/題目 [varchar(255)][NN]
    body/body [text]
    index i_my_articles_user_title (user_id, title)

i_my_tags/i_my_tags
    +name/name [varchar(255)][NN]
    description/description [text]

i_my_users/利用者
    +id/利用者ID [bigint][NN]
    email/メールアドレス [varchar(255)][NN][U]
    name/表示名 [varchar(255)][NN]
`

// TestIntegration_MySQL_RoundtripAndExpected は MySQL 統合テストのメインケース。
// `ERDM_TEST_MYSQL_DSN` が未設定なら自動スキップする。
func TestIntegration_MySQL_RoundtripAndExpected(t *testing.T) {
	dsn := os.Getenv("ERDM_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("ERDM_TEST_MYSQL_DSN not set; skipping MySQL integration test")
	}
	ctx := context.Background()
	loadMySQLFixture(t, dsn, myFixtureDDL)

	schema, err := Introspect(ctx, Options{
		Driver: DriverMySQL,
		DSN:    dsn,
		Title:  "integration_mysql",
	})
	if err != nil {
		t.Fatalf("Introspect failed: %v", err)
	}
	if err := schema.Validate(); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	got, err := serializer.Serialize(schema)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}
	gotText := filterFixtureTables(string(got), []string{
		"i_my_article_tags",
		"i_my_articles",
		"i_my_tags",
		"i_my_users",
	}, "integration_mysql")
	if gotText != expectedMyErdm {
		t.Fatalf("erdm mismatch.\n--- got ---\n%s\n--- want ---\n%s", gotText, expectedMyErdm)
	}

	parsed, perr := parser.Parse([]byte(gotText))
	if perr != nil {
		t.Fatalf("re-parse failed: %v", perr)
	}
	if err := parsed.Validate(); err != nil {
		t.Fatalf("re-parse validate failed: %v", err)
	}
	got2, err := serializer.Serialize(parsed)
	if err != nil {
		t.Fatalf("re-serialize failed: %v", err)
	}
	if string(got2) != gotText {
		t.Fatalf("roundtrip not byte-identical:\n--- got2 ---\n%s\n--- got ---\n%s", got2, gotText)
	}
}

// TestIntegration_MySQL_ReadOnly は Introspect の前後で対象テーブルの行件数が
// 変化しないことを確認する（要件 10.1 / 10.2 の MySQL 適用）。
func TestIntegration_MySQL_ReadOnly(t *testing.T) {
	dsn := os.Getenv("ERDM_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("ERDM_TEST_MYSQL_DSN not set; skipping MySQL integration test")
	}
	ctx := context.Background()
	loadMySQLFixture(t, dsn, myFixtureDDL+`
INSERT INTO i_my_users (email, name) VALUES ('a@example.com', 'Alice');
INSERT INTO i_my_users (email, name) VALUES ('b@example.com', 'Bob');
`)
	before := countMySQLRows(t, dsn, "i_my_users")
	if _, err := Introspect(ctx, Options{Driver: DriverMySQL, DSN: dsn, Title: "integration_mysql"}); err != nil {
		t.Fatalf("Introspect failed: %v", err)
	}
	after := countMySQLRows(t, dsn, "i_my_users")
	if before != after {
		t.Fatalf("i_my_users row count changed: before=%d after=%d (Introspect must not write)", before, after)
	}
}

// loadMySQLFixture は対象 DSN に対してフィクスチャ DDL を投入する。
//
// 既存ゴミデータと衝突しないよう、フィクスチャ先頭の `DROP TABLE IF EXISTS` で
// 子→親順に削除してから CREATE する。
func loadMySQLFixture(t *testing.T, dsn, ddl string) {
	t.Helper()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open mysql: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, stmt := range splitSQLStatements(ddl) {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec ddl %q: %v", stmt, err)
		}
	}
}

// countMySQLRows は MySQL に対して COUNT(*) を発行し、対象テーブルの行数を返す。
func countMySQLRows(t *testing.T, dsn, table string) int {
	t.Helper()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open mysql: %v", err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}
