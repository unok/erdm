package introspect

import (
	"strings"
	"testing"
)

// newMySQLIntrospector はスキーマ未指定時、空文字列を保持する。
// MySQL は既定スキーマを「接続先 DB そのもの」とする方針で（要件 3.3）、
// 既定値の解決自体は接続段階／ドライバ層で行うため、コンストラクタは
// 入力を素通しする責務に留める。
func TestNewMySQLIntrospector_PreservesEmptySchema(t *testing.T) {
	t.Parallel()
	got := newMySQLIntrospector(nil, "")
	if got.schema != "" {
		t.Fatalf("schema = %q, want %q", got.schema, "")
	}
}

// 明示指定された schema は素通しで採用される（要件 3.3）。
func TestNewMySQLIntrospector_HonorsExplicitSchema(t *testing.T) {
	t.Parallel()
	got := newMySQLIntrospector(nil, "shop")
	if got.schema != "shop" {
		t.Fatalf("schema = %q, want %q", got.schema, "shop")
	}
}

// normalizeMySQLAutoIncrement は EXTRA に "auto_increment" を含むときだけ
// Default を空にクリアする（要件 4.7）。それ以外の組合せは入力をそのまま返す。
func TestNormalizeMySQLAutoIncrement(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		extra       string
		columnDef   string
		wantDefault string
	}{
		{
			name:        "auto_increment with default zero is cleared",
			extra:       "auto_increment",
			columnDef:   "0",
			wantDefault: "",
		},
		{
			name:        "auto_increment with empty default stays empty",
			extra:       "auto_increment",
			columnDef:   "",
			wantDefault: "",
		},
		{
			name:        "no extra preserves literal default",
			extra:       "",
			columnDef:   "0",
			wantDefault: "0",
		},
		{
			name:        "default_generated only preserves current_timestamp",
			extra:       "DEFAULT_GENERATED",
			columnDef:   "CURRENT_TIMESTAMP",
			wantDefault: "CURRENT_TIMESTAMP",
		},
		{
			name:        "auto_increment combined with other extras still clears",
			extra:       "auto_increment DEFAULT_GENERATED",
			columnDef:   "0",
			wantDefault: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeMySQLAutoIncrement(c.extra, c.columnDef)
			if got != c.wantDefault {
				t.Fatalf("normalizeMySQLAutoIncrement(%q,%q) = %q, want %q",
					c.extra, c.columnDef, got, c.wantDefault)
			}
		})
	}
}

// SQL 定数のスモークテスト。SQL の細かな構文ではなく、`information_schema`
// の主要キーワードがクエリから抜け落ちていないかをガードする。
// 本格的な振る舞い検証はタスク 10.2 の統合テストの責務。
func TestMySQLSQLConstantsCarryRequiredKeywords(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		sql  string
		want []string
	}{
		{
			name: "tables",
			sql:  sqlSelectMySQLTables,
			want: []string{"information_schema.tables", "BASE TABLE", "TABLE_COMMENT", "TABLE_SCHEMA = ?"},
		},
		{
			name: "columns",
			sql:  sqlSelectMySQLColumns,
			want: []string{"information_schema.columns", "ORDINAL_POSITION", "COLUMN_TYPE", "COLUMN_COMMENT", "EXTRA", "IS_NULLABLE"},
		},
		{
			name: "primary keys",
			sql:  sqlSelectMySQLPrimaryKeys,
			want: []string{"key_column_usage", "'PRIMARY'", "ORDINAL_POSITION"},
		},
		{
			name: "unique constraints",
			sql:  sqlSelectMySQLUniqueConstraints,
			want: []string{"table_constraints", "'UNIQUE'", "key_column_usage"},
		},
		{
			name: "foreign keys",
			sql:  sqlSelectMySQLForeignKeys,
			want: []string{"referential_constraints", "key_column_usage", "REFERENCED_TABLE_NAME", "ORDINAL_POSITION"},
		},
		{
			name: "indexes",
			sql:  sqlSelectMySQLIndexes,
			want: []string{"information_schema.statistics", "NON_UNIQUE", "SEQ_IN_INDEX", "!= 'PRIMARY'"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			for _, kw := range c.want {
				if !strings.Contains(c.sql, kw) {
					t.Errorf("%s SQL missing keyword %q", c.name, kw)
				}
			}
		})
	}
}
