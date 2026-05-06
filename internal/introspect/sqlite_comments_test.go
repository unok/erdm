package introspect

import (
	"reflect"
	"testing"
)

// extractSQLiteColumnComments は CREATE TABLE 原文からカラム宣言行末の行コメント
// （`-- ...`）を抽出する純粋関数（要件 8.3）。
//
// 表駆動で主要パターンを固定する:
//   - 単一行に複数識別子が含まれる定義（`CREATE TABLE t (col TYPE, -- ...`）。
//   - 1 行 1 カラムの定義。
//   - コメントなし DDL（マップ空）。
//   - コメント本文に `--` を含むケース（最初の `--` 以降を全部採用）。
//   - knownColumns に含まれない識別子は採用しない（誤検出防止）。
//   - 識別子のクオート方言（"...", `, [...]）。
func TestExtractSQLiteColumnComments(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		ddl     string
		columns []string
		want    map[string]string
	}{
		{
			name:    "inline first column with same-line comment",
			ddl:     "CREATE TABLE users (id INTEGER, -- 利用者ID\n  name TEXT)",
			columns: []string{"id", "name"},
			want:    map[string]string{"id": "利用者ID"},
		},
		{
			name: "multiple columns each with trailing comment",
			ddl: "CREATE TABLE users (\n" +
				"  id INTEGER PRIMARY KEY, -- 識別子\n" +
				"  email TEXT NOT NULL, -- メールアドレス\n" +
				"  name TEXT -- 表示名\n" +
				")",
			columns: []string{"id", "email", "name"},
			want: map[string]string{
				"id":    "識別子",
				"email": "メールアドレス",
				"name":  "表示名",
			},
		},
		{
			name:    "ddl without trailing comments yields empty map",
			ddl:     "CREATE TABLE t (id INTEGER, name TEXT)",
			columns: []string{"id", "name"},
			want:    map[string]string{},
		},
		{
			name:    "comment with embedded double dash keeps everything after first marker",
			ddl:     "CREATE TABLE t (\n  id INTEGER, -- xxx -- yyy\n  name TEXT)",
			columns: []string{"id", "name"},
			want:    map[string]string{"id": "xxx -- yyy"},
		},
		{
			name:    "unknown identifier is not recorded",
			ddl:     "CREATE TABLE t (\n  unknown_col INTEGER, -- not assigned\n  id INTEGER)",
			columns: []string{"id"},
			want:    map[string]string{},
		},
		{
			name:    "double-quoted identifier",
			ddl:     "CREATE TABLE t (\n  \"id\" INTEGER, -- 識別子\n  name TEXT)",
			columns: []string{"id", "name"},
			want:    map[string]string{"id": "識別子"},
		},
		{
			name:    "backticked identifier",
			ddl:     "CREATE TABLE t (\n  `name` TEXT, -- 表示名\n  id INTEGER)",
			columns: []string{"id", "name"},
			want:    map[string]string{"name": "表示名"},
		},
		{
			name:    "bracketed identifier",
			ddl:     "CREATE TABLE t (\n  [id] INTEGER, -- 識別子\n  name TEXT)",
			columns: []string{"id", "name"},
			want:    map[string]string{"id": "識別子"},
		},
		{
			name:    "empty ddl returns empty map",
			ddl:     "",
			columns: []string{"id"},
			want:    map[string]string{},
		},
		{
			name:    "empty knownColumns returns empty map",
			ddl:     "CREATE TABLE t (id INTEGER, -- ID\n  name TEXT)",
			columns: nil,
			want:    map[string]string{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := extractSQLiteColumnComments(c.ddl, c.columns)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("extractSQLiteColumnComments(...) = %v, want %v", got, c.want)
			}
		})
	}
}

// unquoteSQLiteIdentifier は 3 種類のクオートを取り除き、クオート無しは素通し
// する純粋ヘルパ。
func TestUnquoteSQLiteIdentifier(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "unquoted preserved", in: "id", want: "id"},
		{name: "double quoted unwrapped", in: `"id"`, want: "id"},
		{name: "backticked unwrapped", in: "`id`", want: "id"},
		{name: "bracketed unwrapped", in: "[id]", want: "id"},
		{name: "single character unchanged", in: "a", want: "a"},
		{name: "empty string preserved", in: "", want: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := unquoteSQLiteIdentifier(c.in); got != c.want {
				t.Fatalf("unquoteSQLiteIdentifier(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
