// Package ddl のスナップショットテスト。
//
// 各ケースは *model.Schema を Go リテラルで構築し、RenderPG / RenderSQLite
// の出力を dialect 別ゴールデンファイル（testdata/golden/{pg,sqlite}/*.sql）と
// 比較する。
//
// 旧 templates/{pg,sqlite3}_ddl.tmpl 出力との直接 diff はバッチ 8.1（横断品質）
// で網羅し、本テストは新出力をゴールデンで固定することに特化する。
//
// Requirements: 5.6, 9.1
package ddl

import (
	"strings"
	"testing"

	"github.com/unok/erdm/internal/model"
	"github.com/unok/erdm/internal/testutil/golden"
)

// dialect ごとの Render 関数を統一的に扱うためのテーブル駆動エントリ。
type dialect struct {
	name   string
	render func(*model.Schema) ([]byte, error)
}

var dialects = []dialect{
	{name: "pg", render: RenderPG},
	{name: "sqlite", render: RenderSQLite},
}

// schemaSimple は単純な 2 テーブル + FK + 単一主キーのフィクスチャ。
func schemaSimple() *model.Schema {
	return &model.Schema{
		Title: "simple",
		Tables: []model.Table{
			{
				Name: "users",
				Columns: []model.Column{
					{Name: "id", Type: "int", IsPrimaryKey: true},
					{Name: "name", Type: "varchar(64)"},
				},
				PrimaryKeys: []int{0},
			},
			{
				Name: "posts",
				Columns: []model.Column{
					{Name: "id", Type: "int", IsPrimaryKey: true},
					{
						Name: "user_id", Type: "int",
						FK: &model.FK{
							TargetTable:            "users",
							CardinalitySource:      "0..*",
							CardinalityDestination: "1",
						},
					},
				},
				PrimaryKeys: []int{0},
			},
		},
	}
}

// schemaFull は NOT NULL / UNIQUE / DEFAULT / 複合 PK / インデックス
// （unique / normal）を全て含む全部入りフィクスチャ。
func schemaFull() *model.Schema {
	return &model.Schema{
		Title: "full",
		Tables: []model.Table{
			{
				Name: "customers",
				Columns: []model.Column{
					{Name: "id", Type: "int", IsPrimaryKey: true},
					{Name: "email", Type: "varchar(255)", IsUnique: true},
					{Name: "status", Type: "varchar(16)", AllowNull: true, Default: "'active'"},
				},
				PrimaryKeys: []int{0},
				Indexes: []model.Index{
					{Name: "uk_customers_email", Columns: []string{"email"}, IsUnique: true},
				},
			},
			{
				Name: "order_items",
				Columns: []model.Column{
					{
						Name: "order_id", Type: "int", IsPrimaryKey: true,
						FK: &model.FK{
							TargetTable:            "orders",
							CardinalitySource:      "1..*",
							CardinalityDestination: "1",
						},
					},
					{Name: "line_no", Type: "int", IsPrimaryKey: true},
					{Name: "qty", Type: "int", Default: "1"},
				},
				PrimaryKeys: []int{0, 1},
				Indexes: []model.Index{
					{Name: "ix_order_items_order", Columns: []string{"order_id"}},
					{Name: "ix_order_items_order_line", Columns: []string{"order_id", "line_no"}},
				},
			},
			{
				Name: "orders",
				Columns: []model.Column{
					{Name: "id", Type: "int", IsPrimaryKey: true},
					{Name: "customer_id", Type: "int",
						FK: &model.FK{
							TargetTable:            "customers",
							CardinalitySource:      "0..*",
							CardinalityDestination: "1",
						},
					},
				},
				PrimaryKeys: []int{0},
			},
		},
	}
}

func TestRender_Simple(t *testing.T) {
	s := schemaSimple()
	for _, d := range dialects {
		t.Run(d.name, func(t *testing.T) {
			got, err := d.render(s)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			golden.Compare(t, got, "testdata/golden/"+d.name+"/01_simple.sql")
		})
	}
}

func TestRender_Full(t *testing.T) {
	s := schemaFull()
	for _, d := range dialects {
		t.Run(d.name, func(t *testing.T) {
			got, err := d.render(s)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			golden.Compare(t, got, "testdata/golden/"+d.name+"/02_full.sql")
		})
	}
}

// TestRender_NilSchema は nil 入力をエラーで弾くことを確認する（Fail Fast）。
func TestRender_NilSchema(t *testing.T) {
	for _, d := range dialects {
		t.Run(d.name, func(t *testing.T) {
			got, err := d.render(nil)
			if err == nil {
				t.Fatalf("Render(nil) returned no error; got=%q", string(got))
			}
		})
	}
}

// TestRenderPG_RequiredKeywords は PG DDL 出力に必須キーワード一式が含まれることを
// ゴールデンとは独立に確認する（要件 8.1 / 8.2 の保証点）。
func TestRenderPG_RequiredKeywords(t *testing.T) {
	got, err := RenderPG(schemaFull())
	if err != nil {
		t.Fatalf("RenderPG: %v", err)
	}
	out := string(got)
	required := []string{
		"DROP TABLE IF EXISTS customers CASCADE",
		"DROP TABLE IF EXISTS order_items CASCADE",
		"CREATE TABLE customers (",
		"PRIMARY KEY (id)",
		"PRIMARY KEY (order_id, line_no)",
		"NOT NULL",
		"UNIQUE",
		"DEFAULT 'active'",
		"CREATE UNIQUE INDEX uk_customers_email ON customers (email)",
		"CREATE INDEX ix_order_items_order_line ON order_items (order_id, line_no)",
	}
	for _, kw := range required {
		if !strings.Contains(out, kw) {
			t.Errorf("PG output missing required substring %q\n--- output ---\n%s", kw, out)
		}
	}
}

// TestRenderSQLite_NoCascade は SQLite 出力に CASCADE が含まれないことを確認する
// （SQLite3 は DROP TABLE ... CASCADE を未サポート）。
func TestRenderSQLite_NoCascade(t *testing.T) {
	got, err := RenderSQLite(schemaSimple())
	if err != nil {
		t.Fatalf("RenderSQLite: %v", err)
	}
	out := string(got)
	if strings.Contains(out, "CASCADE") {
		t.Errorf("SQLite output must not contain CASCADE; got:\n%s", out)
	}
	if !strings.Contains(out, "DROP TABLE IF EXISTS users;") {
		t.Errorf("SQLite output must contain `DROP TABLE IF EXISTS users;`; got:\n%s", out)
	}
}
