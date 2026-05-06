// integration_postgres_test.go は稼働中の PostgreSQL に対するイントロスペクション
// 統合テスト（タスク 10.1）。
//
// 実 DB 起動の取り扱い:
//   - 環境変数 `ERDM_TEST_POSTGRES_DSN` が設定されているときのみ走る。未設定時は
//     `t.Skip` で自動スキップする（CI 互換性確保 / 環境依存テスト）。
//   - 採用方針の根拠は coder-decisions.md §"testcontainers-go を採用しない決定"
//     を参照。要点: testcontainers-go は ~80 件の test-only 推移依存を `go.mod` に
//     持ち込むため、より軽量な「環境変数 DSN 提供方式」を採用した。
//
// 検証点（tasks.md / 00-plan.md §"Task 10.1"）:
//   - 起動済 PG にテーブル定義・コメント・複合主キー・複合外部キー・補助
//     インデックス・自動連番列を含むフィクスチャを投入し、生成された出力テキストが
//     期待文字列と一致する。
//   - 既存パーサで再パース可能であり、検証を通過し、再シリアライズで内容が安定する。
//   - 読み取り専用トランザクションのもとで意図せぬ書き込みが発生していない
//     （フィクスチャ既存行件数を Introspect 前後で比較）。
package introspect

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	// テスト専用 blank import: integration_postgres_test.go はテストビルドでのみ
	// pgx/v5/stdlib を `database/sql.Open("pgx", ...)` 用に登録する。
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/unok/erdm/internal/parser"
	"github.com/unok/erdm/internal/serializer"
)

// pgFixtureDDL は PG 統合テストで投入するスキーマ。
//
// 設計意図:
//   - 既存ゴミデータと衝突しないよう FK 子→親順で `DROP TABLE IF EXISTS` を発行
//     してからフィクスチャを再構築する。
//   - `i_pg_users` は `bigserial` PK、UNIQUE 制約あり、テーブル／カラムコメントあり
//     → 要件 4.7（自動連番抑止 + bigserial 表記）／ 8.1（pg_description コメント）を網羅。
//   - `i_pg_articles` は `i_pg_users.id` への単一 FK（NOT NULL）、補助インデックスあり
//     → 要件 6.x ／ 7.x を網羅。
//   - `i_pg_article_tags` は複合 PK（article_id, tag）と複合 FK（article_id → articles）
//     → 要件 5.x ／ 6.4 を網羅。
//   - `i_pg_tags` はテーブル／カラムコメント未設定で物理名フォールバック（要件 8.6）。
//   - 補助インデックス `i_pg_articles_user_title` は `articles(user_id, title)`。
const pgFixtureDDL = `
DROP TABLE IF EXISTS i_pg_article_tags CASCADE;
DROP TABLE IF EXISTS i_pg_articles CASCADE;
DROP TABLE IF EXISTS i_pg_users CASCADE;
DROP TABLE IF EXISTS i_pg_tags CASCADE;

CREATE TABLE i_pg_users (
    id bigserial PRIMARY KEY,
    email text NOT NULL UNIQUE,
    name text NOT NULL
);
COMMENT ON TABLE  i_pg_users        IS '利用者';
COMMENT ON COLUMN i_pg_users.id     IS '利用者ID';
COMMENT ON COLUMN i_pg_users.email  IS 'メールアドレス';
COMMENT ON COLUMN i_pg_users.name   IS '表示名';

CREATE TABLE i_pg_articles (
    id bigserial PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES i_pg_users(id),
    title text NOT NULL,
    body text
);
COMMENT ON TABLE  i_pg_articles       IS '記事';
COMMENT ON COLUMN i_pg_articles.id    IS '記事ID';
COMMENT ON COLUMN i_pg_articles.title IS '題目';

CREATE INDEX i_pg_articles_user_title ON i_pg_articles(user_id, title);

CREATE TABLE i_pg_article_tags (
    article_id bigint NOT NULL,
    tag text NOT NULL,
    PRIMARY KEY (article_id, tag),
    FOREIGN KEY (article_id) REFERENCES i_pg_articles(id)
);
COMMENT ON TABLE  i_pg_article_tags        IS '記事タグ';
COMMENT ON COLUMN i_pg_article_tags.tag    IS 'タグ名';

CREATE TABLE i_pg_tags (
    name text PRIMARY KEY,
    description text
);
`

// expectedPGErdm は pgFixtureDDL から生成される `.erdm` テキストの期待値。
//
// 期待値の根拠（design.md §"型表記" / 要件 4.x / 5.x / 6.x / 7.x / 8.x）:
//   - タイトルは Options.Title が空のため DSN 由来の DB 名（`resolveTitle` 規定）。
//     本テストでは Options.Title を明示指定して値を固定する。
//   - bigserial PK は型 `bigserial`、デフォルト抑止（要件 4.7）。
//   - 単一 PK で UNIQUE な `email` は `[NN][U]`、参照元 NOT NULL ＋ UNIQUE 性なし
//     カラムへの単一 FK は `1..*--1`。
//   - 複合 FK の構成先頭カラムにのみ関係を付与（要件 6.4）。
//   - `i_pg_tags.name` のテーブル／カラムコメント未設定は `name/name` 物理名
//     フォールバックで出力される（要件 8.6）。
//   - テーブル順序は `information_schema.tables` の規定順（要件 3.6）。本フィクスチャ
//     では宣言順（users → articles → article_tags → tags）に揃うことを期待する。
// PostgreSQL は PRIMARY KEY 制約を NOT NULL に拡張するため、PK カラムは
// `[NN]` 属性を持つ（要件 4.3 を PG が自動で満たす挙動）。serializer はこの
// `[NN]` を素直に出力する。
const expectedPGErdm = `# Title: integration_pg

i_pg_users/利用者
    +id/利用者ID [bigserial][NN]
    email/メールアドレス [text][NN][U]
    name/表示名 [text][NN]

i_pg_articles/記事
    +id/記事ID [bigserial][NN]
    user_id/user_id [bigint][NN] 1..*--1 i_pg_users
    title/題目 [text][NN]
    body/body [text]
    index i_pg_articles_user_title (user_id, title)

i_pg_article_tags/記事タグ
    +article_id/article_id [bigint][NN] 1..*--1 i_pg_articles
    +tag/タグ名 [text][NN]

i_pg_tags/i_pg_tags
    +name/name [text][NN]
    description/description [text]
`

// TestIntegration_Postgres_RoundtripAndExpected は PG 統合テストのメインケース。
// `ERDM_TEST_POSTGRES_DSN` が未設定なら自動スキップする。
func TestIntegration_Postgres_RoundtripAndExpected(t *testing.T) {
	dsn := os.Getenv("ERDM_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("ERDM_TEST_POSTGRES_DSN not set; skipping PostgreSQL integration test")
	}
	ctx := context.Background()
	loadPGFixture(t, dsn, pgFixtureDDL)

	schema, err := Introspect(ctx, Options{
		Driver: DriverPostgreSQL,
		DSN:    dsn,
		Title:  "integration_pg",
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
		"i_pg_users",
		"i_pg_articles",
		"i_pg_article_tags",
		"i_pg_tags",
	}, "integration_pg")
	if gotText != expectedPGErdm {
		t.Fatalf("erdm mismatch.\n--- got ---\n%s\n--- want ---\n%s", gotText, expectedPGErdm)
	}

	// 再パース → 検証 → 再シリアライズで往復冪等性を確認（要件 9.2 / 9.3）。
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

// TestIntegration_Postgres_ArrayTypes は配列カラムと、値に `]` を含む default
// 式（`'{}'::integer[]` 等）が end-to-end で正しく取り込まれ、Introspect →
// Serialize → Parse → 再 Serialize の往復で同一テキストになることを検証する。
//
// 検証点:
//   - resolvePGType の ARRAY 経路（udt_name `_int4` → `integer[]` 等の写像）
//   - parser.peg の col_type 拡張（`[varchar[]]` 等の受理）
//   - parser builder の `\]` → `]` unescape と serializer 側の `]` → `\]` escape
//
// `ERDM_TEST_POSTGRES_DSN` 未設定時は自動スキップ。
func TestIntegration_Postgres_ArrayTypes(t *testing.T) {
	dsn := os.Getenv("ERDM_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("ERDM_TEST_POSTGRES_DSN not set; skipping PostgreSQL integration test")
	}
	const ddl = `
DROP TABLE IF EXISTS i_pg_arr CASCADE;
CREATE TABLE i_pg_arr (
    id bigserial PRIMARY KEY,
    tags text[] NOT NULL DEFAULT '{}'::text[],
    tag_ids integer[] NOT NULL DEFAULT '{}'::integer[],
    titles character varying[] NOT NULL,
    matrix numeric(10,2)[]
);
COMMENT ON TABLE  i_pg_arr        IS '配列型カラム';
`
	loadPGFixture(t, dsn, ddl)
	ctx := context.Background()
	schema, err := Introspect(ctx, Options{Driver: DriverPostgreSQL, DSN: dsn, Title: "integration_pg_arr"})
	if err != nil {
		t.Fatalf("Introspect: %v", err)
	}
	if err := schema.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	got, err := serializer.Serialize(schema)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	gotText := filterFixtureTables(string(got), []string{"i_pg_arr"}, "integration_pg_arr")
	wantSubstrings := []string{
		"tags/tags [text[]][NN][='{}'::text[\\]]",
		"tag_ids/tag_ids [integer[]][NN][='{}'::integer[\\]]",
		"titles/titles [varchar[]][NN]",
		// numeric(p,s)[] は data_type='ARRAY' / udt_name='_numeric' で取得される。
		// PG は precision/scale を data_type に含めず別カラムへ載せるため、現状の
		// 実装では `numeric[]` で出力される（precision 復元は本 PR スコープ外）。
		"matrix/matrix [numeric[]]",
	}
	for _, s := range wantSubstrings {
		if !strings.Contains(gotText, s) {
			t.Errorf("missing substring %q in:\n%s", s, gotText)
		}
	}
	// 再パース→再シリアライズの不動点性。
	parsed, perr := parser.Parse([]byte(gotText))
	if perr != nil {
		t.Fatalf("re-parse: %v\n%s", perr, gotText)
	}
	if err := parsed.Validate(); err != nil {
		t.Fatalf("re-parse validate: %v", err)
	}
	got2, err := serializer.Serialize(parsed)
	if err != nil {
		t.Fatalf("re-serialize: %v", err)
	}
	got2Text := filterFixtureTables(string(got2), []string{"i_pg_arr"}, "integration_pg_arr")
	if got2Text != gotText {
		t.Fatalf("not byte-identical:\n--- got2 ---\n%s\n--- got ---\n%s", got2Text, gotText)
	}
	// model 側の意味値検証: default は `]` を含む（escape 解除済）状態で保持される。
	var arrTbl *struct{ Default string }
	for _, tbl := range parsed.Tables {
		if tbl.Name != "i_pg_arr" {
			continue
		}
		for _, c := range tbl.Columns {
			if c.Name == "tags" {
				if c.Default != "'{}'::text[]" {
					t.Errorf("tags.Default=%q want '{}'::text[]", c.Default)
				}
			}
			if c.Name == "tag_ids" {
				if c.Default != "'{}'::integer[]" {
					t.Errorf("tag_ids.Default=%q want '{}'::integer[]", c.Default)
				}
			}
		}
	}
	_ = arrTbl
}

// TestIntegration_Postgres_ReadOnly は Introspect の前後で対象テーブルの行件数が
// 変化しないことを確認する（要件 10.1 / 10.2）。
func TestIntegration_Postgres_ReadOnly(t *testing.T) {
	dsn := os.Getenv("ERDM_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("ERDM_TEST_POSTGRES_DSN not set; skipping PostgreSQL integration test")
	}
	ctx := context.Background()
	loadPGFixture(t, dsn, pgFixtureDDL+`
INSERT INTO i_pg_users (email, name) VALUES ('a@example.com', 'Alice');
INSERT INTO i_pg_users (email, name) VALUES ('b@example.com', 'Bob');
`)
	before := countPGRows(t, dsn, "i_pg_users")
	if _, err := Introspect(ctx, Options{Driver: DriverPostgreSQL, DSN: dsn, Title: "integration_pg"}); err != nil {
		t.Fatalf("Introspect failed: %v", err)
	}
	after := countPGRows(t, dsn, "i_pg_users")
	if before != after {
		t.Fatalf("i_pg_users row count changed: before=%d after=%d (Introspect must not write)", before, after)
	}
}

// loadPGFixture は対象 DSN に対してフィクスチャ DDL を投入する。
//
// 既存ゴミデータと衝突しないよう、フィクスチャ先頭の `DROP TABLE IF EXISTS` で
// 子→親順に削除してから CREATE する設計（pgFixtureDDL 参照）。
func loadPGFixture(t *testing.T, dsn, ddl string) {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open pgx: %v", err)
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

// countPGRows は PG に対して COUNT(*) を発行し、対象テーブルの行数を返す。
func countPGRows(t *testing.T, dsn, table string) int {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open pgx: %v", err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

// filterFixtureTables は serializer.Serialize の出力からフィクスチャ対象テーブル
// だけを残し、共有 DB 上の他テーブルを取り除いた `.erdm` テキストを再構成する。
//
// 共有 PG／MySQL DB を使う統合テストの宿命として、フィクスチャ以外のテーブルが
// 同 DSN／DB に存在しうる。期待値テキスト（expectedPGErdm 等）はフィクスチャ
// テーブルのみを記述するため、テスト実行時は同種フィルタで絞り込む必要がある。
//
// 実装方針:
//   - 入力テキストを `\n\n` で分割し、各ブロックの 1 行目をテーブル宣言行とみなす。
//   - 1 ブロック目はタイトル行（`# Title: ...`）として固定。
//   - テーブル宣言行の物理名（`/` または ` ` で区切られる先頭トークン）が allow に
//     含まれるブロックのみを残す。
//   - allow の順序通りに整列し直す。
//   - タイトルは `# Title: <title>` で固定する（DSN 由来のタイトルは DB 構成依存）。
func filterFixtureTables(text string, allow []string, title string) string {
	allowed := make(map[string]int, len(allow))
	for i, n := range allow {
		allowed[n] = i
	}
	blocks := strings.Split(strings.TrimRight(text, "\n"), "\n\n")
	keep := make([]string, len(allow))
	for _, blk := range blocks {
		firstLine := blk
		if i := strings.IndexByte(blk, '\n'); i >= 0 {
			firstLine = blk[:i]
		}
		if strings.HasPrefix(firstLine, "#") {
			continue // タイトル行は別途固定値で先頭に追加する
		}
		// テーブル宣言行は `name[/logical][ @groups[...]]` 形式。先頭トークンを切り出す。
		end := len(firstLine)
		for i := 0; i < len(firstLine); i++ {
			if firstLine[i] == '/' || firstLine[i] == ' ' {
				end = i
				break
			}
		}
		name := firstLine[:end]
		if idx, ok := allowed[name]; ok {
			keep[idx] = blk
		}
	}
	var out strings.Builder
	out.WriteString("# Title: ")
	out.WriteString(title)
	out.WriteByte('\n')
	for _, blk := range keep {
		if blk == "" {
			continue
		}
		out.WriteByte('\n')
		out.WriteString(blk)
		out.WriteByte('\n')
	}
	return out.String()
}
