// integration_sqlite_test.go は SQLite を実体ファイルへ起こしてフィクスチャ DDL を
// 投入し、`Introspect` → `serializer.Serialize` の経路で生成された `.erdm` テキストを
// 期待値および往復冪等性で検証する統合テスト（タスク 10.3）。
//
// 検証点（tasks.md / 00-plan.md §"Task 10.3"）:
//   - 一時ファイルにフィクスチャ DDL を投入し、出力テキストの期待値一致と既存パーサ
//     往復の冪等性を確認する（要件 2.3 / 9.1 / 9.2 / 9.3）。
//   - DDL 原文からのコメント抽出が成功するケース／取得不能ケースの両方で物理名
//     フォールバックが効くことを確認する（要件 8.3 / 8.6）。
//
// 依存ドライバ（`modernc.org/sqlite`）は test-only blank import として本ファイル
// 内で登録する。本番バイナリへの影響は無い（test 構成成果物のみが本 import を
// 含む）。
package introspect

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	// テスト専用 blank import: integration_*_test.go は本番ソースから driver を
	// 切り離す方針（要件 12.3）を維持するため、テストビルド限定で driver を
	// 登録する。erdm.go の blank import と並列だが、import 範囲はテストバイナリに
	// 閉じる。
	_ "modernc.org/sqlite"

	"github.com/unok/erdm/internal/parser"
	"github.com/unok/erdm/internal/serializer"
)

// sqliteFixtureDDL は統合テストで投入する SQLite スキーマのフィクスチャ DDL。
//
// 設計意図:
//   - `users`: 単一カラム PK（自動 ROWID）、UNIQUE 制約、行末 `--` コメント付き
//     カラムを含み、要件 4.x / 8.3 を網羅する。
//   - `articles`: `users.id` への単一 FK、補助インデックス、コメントなしカラムを含む。
//   - `tag_assignments`: 複合 PK（`article_id, tag`）と複合 FK（`article_id`）を含む。
//   - `tags`: テーブル直前／末尾コメント無しの「論理名 = 物理名フォールバック」検証用。
//
// SQLite はテーブルレベルのコメント機構を持たないため、本フィクスチャでは
// テーブルコメントを宣言できない。論理名はカラム単位で `--` 行末コメントから
// 抽出され、抽出不能なカラム／テーブルは物理名にフォールバックする（要件 8.6）。
const sqliteFixtureDDL = `
CREATE TABLE users (
    id INTEGER PRIMARY KEY,             -- 利用者ID
    email TEXT NOT NULL UNIQUE,         -- メールアドレス
    name TEXT NOT NULL                  -- 表示名
);

CREATE TABLE articles (
    id INTEGER PRIMARY KEY,             -- 記事ID
    user_id INTEGER NOT NULL,
    title TEXT NOT NULL,
    body TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX i_articles_user_id_title ON articles(user_id, title);

CREATE TABLE tag_assignments (
    article_id INTEGER NOT NULL,        -- 記事ID
    tag TEXT NOT NULL,                  -- タグ名
    PRIMARY KEY (article_id, tag),
    FOREIGN KEY (article_id) REFERENCES articles(id)
);

CREATE TABLE tags (
    name TEXT PRIMARY KEY,
    description TEXT
);
`

// expectedSQLiteErdm は sqliteFixtureDDL から生成される `.erdm` テキストの期待値。
//
// 期待値の根拠:
//   - 1 行目に `# Title: ...`。タイトルは SQLite DSN のファイル名ベース（"fixture"）。
//   - テーブル間に空行 1 行（serializer.Serialize の規定）。
//   - カラム属性順は `[NN] → [U] → [=default] → [-erd]`（serializer.format 規定）。
//   - 単一 PK の `id` は自動 ROWID 連番列として `[INTEGER]` のまま、デフォルト値抑止。
//   - FK 関係はカーディナリティ `0..*--1` ／ `1..*--1` を取る。
//   - 補助インデックス `i_articles_user_id_title` のみが残り、PK／UNIQUE 制約由来の
//     インデックスは除外される。
//   - 論理名: コメント抽出可能なカラムは抽出値（"利用者ID" 等）、それ以外のカラム／
//     テーブルは物理名フォールバックで `<phys>/<phys>` 形式となる（要件 8.6 ＝
//     ユーザー指示「基本コメントは論理名になるように」の保証点）。
const expectedSQLiteErdm = `# Title: fixture

users/users
    +id/利用者ID [INTEGER]
    email/メールアドレス [TEXT][NN][U]
    name/表示名 [TEXT][NN]

articles/articles
    +id/記事ID [INTEGER]
    user_id/user_id [INTEGER][NN] 1..*--1 users
    title/title [TEXT][NN]
    body/body [TEXT]
    index i_articles_user_id_title (user_id, title)

tag_assignments/tag_assignments
    +article_id/記事ID [INTEGER][NN] 1..*--1 articles
    +tag/タグ名 [TEXT][NN]

tags/tags
    +name/name [TEXT]
    description/description [TEXT]
`

// TestIntegration_SQLite_RoundtripAndExpected は SQLite 統合テストのメインケース
// （タスク 10.3）。一時ファイルにフィクスチャ DDL を投入し、Introspect → Serialize の
// 出力が `expectedSQLiteErdm` とバイト一致することを確認した上で、
// 既存パーサ往復で冪等性を持つことを確認する（要件 2.3 / 9.1 / 9.2 / 9.3）。
func TestIntegration_SQLite_RoundtripAndExpected(t *testing.T) {
	t.Parallel()
	dsn := setupSQLiteFixture(t, sqliteFixtureDDL)
	ctx := context.Background()

	schema, err := Introspect(ctx, Options{
		Driver: DriverSQLite,
		DSN:    dsn,
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
	if string(got) != expectedSQLiteErdm {
		t.Fatalf("erdm mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, expectedSQLiteErdm)
	}

	parsed, perr := parser.Parse(got)
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
	if string(got2) != string(got) {
		t.Fatalf("roundtrip not byte-identical:\n--- got2 ---\n%s\n--- got ---\n%s", got2, got)
	}
}

// TestIntegration_SQLite_LogicalNameFallback はコメント抽出可否の双方で論理名が
// 確実に確定することを確認する（要件 8.3 ／ 8.6）。
//
// 要点:
//   - DDL 原文からコメント抽出に成功するカラム（`users.id` の "利用者ID"）では
//     コメント値が論理名として採用される（要件 8.3 経路）。
//   - DDL 原文からコメント抽出に失敗するカラム（`articles.title`、`tags.name` 等
//     「-- コメント」が無いカラム）では物理名がそのまま論理名となり、論理名が
//     空文字列にならない（要件 8.6 経路）。
func TestIntegration_SQLite_LogicalNameFallback(t *testing.T) {
	t.Parallel()
	dsn := setupSQLiteFixture(t, sqliteFixtureDDL)
	schema, err := Introspect(context.Background(), Options{
		Driver: DriverSQLite,
		DSN:    dsn,
	})
	if err != nil {
		t.Fatalf("Introspect failed: %v", err)
	}

	cases := []struct {
		table  string
		column string
		want   string
	}{
		{table: "users", column: "id", want: "利用者ID"},          // DDL 行末コメント抽出成功
		{table: "users", column: "email", want: "メールアドレス"},     // 同上
		{table: "articles", column: "title", want: "title"},   // コメント無し → 物理名フォールバック
		{table: "articles", column: "body", want: "body"},     // 同上
		{table: "tags", column: "name", want: "name"},         // 同上（テーブル丸ごと無コメント）
		{table: "tags", column: "description", want: "description"},
	}
	for _, c := range cases {
		col := findColumn(t, schema, c.table, c.column)
		if col.LogicalName != c.want {
			t.Errorf("%s.%s logical name = %q, want %q", c.table, c.column, col.LogicalName, c.want)
		}
	}

	// テーブル `tags` はコメント機構非対応で空コメント、論理名は物理名と一致。
	tagsTable := findTable(t, schema, "tags")
	if tagsTable.LogicalName != "tags" {
		t.Errorf("table tags logical name = %q, want %q", tagsTable.LogicalName, "tags")
	}
}

// TestIntegration_SQLite_ReadOnly は Introspect の処理中も以降も DB の中身が
// 変化しないことを確認する（要件 10.1 ／ 10.3 の SQLite 適用）。
//
// SQLite では PG/MySQL のような明示的 READ ONLY トランザクションは存在しないが、
// `sqliteIntrospector` は SELECT/PRAGMA のみを発行する規律を運用で保つ
// （sqlite.go コメント参照）。本テストはその運用契約を行件数比較で担保する。
func TestIntegration_SQLite_ReadOnly(t *testing.T) {
	t.Parallel()
	dsn := setupSQLiteFixture(t, sqliteFixtureDDL+`
INSERT INTO users (id, email, name) VALUES (1, 'a@example.com', 'Alice');
INSERT INTO users (id, email, name) VALUES (2, 'b@example.com', 'Bob');
`)
	before := countSQLiteRows(t, dsn, "users")
	if _, err := Introspect(context.Background(), Options{Driver: DriverSQLite, DSN: dsn}); err != nil {
		t.Fatalf("Introspect failed: %v", err)
	}
	after := countSQLiteRows(t, dsn, "users")
	if before != after {
		t.Fatalf("users row count changed: before=%d after=%d (Introspect must not write)", before, after)
	}
}

// setupSQLiteFixture は一時ディレクトリに SQLite ファイルを作成し、ddls を実行
// した状態で DSN（ファイルパス）を返す。`fixture` 固定名を採用して、
// `resolveTitle` の SQLite 規則が "fixture" を生成することと一貫させる
// （expectedSQLiteErdm の `# Title: fixture` と整合）。
func setupSQLiteFixture(t *testing.T, ddls string) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "fixture.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, stmt := range splitSQLStatements(ddls) {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec ddl %q: %v", stmt, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture db: %v", err)
	}
	return dbPath
}

// splitSQLStatements は raw string で渡された複数文 SQL を `;` で分割する素朴な
// ヘルパ。フィクスチャ DDL は文字列リテラル中に `;` を含まない前提で運用する。
func splitSQLStatements(text string) []string {
	parts := strings.Split(text, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

// countSQLiteRows は SQLite ファイルに対して `SELECT COUNT(*)` を発行し、対象
// テーブルの行数を返す。読み取り専用性検証で `Introspect` 前後の差分が無いことを
// 確認するために使う。
func countSQLiteRows(t *testing.T, dbPath, table string) int {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}
