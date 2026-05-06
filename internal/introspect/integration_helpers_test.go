// integration_helpers_test.go は PG ／ MySQL ／ SQLite 統合テスト群の共有ヘルパ。
//
// 統合テスト 4 ファイル（10.1 / 10.2 / 10.3 / 10.4）の共通照会ロジックを集約する
// （ナレッジ「DRY」）。テスト内で使うのでファイル名が `_test.go` で終わり、
// 本番ビルドへは含まれない。
package introspect

import (
	"testing"

	"github.com/unok/erdm/internal/model"
)

// findTable は schema.Tables から物理名で対象テーブルを線形探索して返す。
// 見つからない場合は t.Fatalf で停止する。
func findTable(t *testing.T, schema *model.Schema, name string) *model.Table {
	t.Helper()
	if schema == nil {
		t.Fatalf("schema is nil; cannot find table %q", name)
	}
	for i := range schema.Tables {
		if schema.Tables[i].Name == name {
			return &schema.Tables[i]
		}
	}
	t.Fatalf("table %q not found in schema (tables=%v)", name, tableNames(schema))
	return nil
}

// findColumn は table.Columns から物理名で対象カラムを線形探索して返す。
// 見つからない場合は t.Fatalf で停止する。
func findColumn(t *testing.T, schema *model.Schema, table, column string) *model.Column {
	t.Helper()
	tbl := findTable(t, schema, table)
	for i := range tbl.Columns {
		if tbl.Columns[i].Name == column {
			return &tbl.Columns[i]
		}
	}
	t.Fatalf("column %q not found in table %q", column, table)
	return nil
}

// tableNames は findTable 失敗時のデバッグ出力で利用するヘルパ。
func tableNames(schema *model.Schema) []string {
	if schema == nil {
		return nil
	}
	out := make([]string, len(schema.Tables))
	for i, t := range schema.Tables {
		out[i] = t.Name
	}
	return out
}
