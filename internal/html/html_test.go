// Package html のスナップショットテスト。
//
// 各ケースは *model.Schema を Go リテラルで構築し、Render の出力をゴールデン
// ファイル（testdata/golden/*.html）と比較する。
//
// 旧 templates/html.tmpl 出力との直接 diff はバッチ 8.1（横断品質）で網羅し、
// 本テストは新出力をゴールデンで固定することに特化する。
//
// Requirements: 9.1
package html

import (
	"strings"
	"testing"

	"github.com/unok/erdm/internal/model"
	"github.com/unok/erdm/internal/testutil/golden"
)

// renderGolden は Render の出力をゴールデンファイルと比較するヘルパ。
func renderGolden(t *testing.T, s *model.Schema, imageFilename, goldenName string) {
	t.Helper()
	got, err := Render(s, imageFilename)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	golden.Compare(t, got, "testdata/golden/"+goldenName)
}

func TestRender_Simple(t *testing.T) {
	// 単純な 2 テーブル + 1 FK。グループなし、論理名なし、コメントなし。
	s := &model.Schema{
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
	renderGolden(t, s, "simple.png", "01_simple.html")
}

func TestRender_Full(t *testing.T) {
	// 論理名・複合 PK・FK・UNIQUE インデックス・コメント・デフォルト値・全部入り。
	s := &model.Schema{
		Title: "full",
		Tables: []model.Table{
			{
				Name:        "customers",
				LogicalName: "顧客",
				Columns: []model.Column{
					{
						Name:         "id",
						LogicalName:  "顧客ID",
						Type:         "int",
						IsPrimaryKey: true,
						Comments:     []string{"顧客の主キー"},
					},
					{
						Name:        "email",
						LogicalName: "メールアドレス",
						Type:        "varchar(255)",
						IsUnique:    true,
					},
					{
						Name:        "status",
						LogicalName: "状態",
						Type:        "varchar(16)",
						AllowNull:   true,
						Default:     "'active'",
						Comments:    []string{"active|inactive|pending", "初期値は active"},
					},
				},
				PrimaryKeys: []int{0},
				Indexes: []model.Index{
					{Name: "uk_customers_email", Columns: []string{"email"}, IsUnique: true},
				},
			},
			{
				Name:        "orders",
				LogicalName: "注文",
				Columns: []model.Column{
					{Name: "id", LogicalName: "注文ID", Type: "int", IsPrimaryKey: true},
					{
						Name:        "customer_id",
						LogicalName: "顧客ID",
						Type:        "int",
						FK: &model.FK{
							TargetTable:            "customers",
							CardinalitySource:      "0..*",
							CardinalityDestination: "1",
						},
					},
					{Name: "ordered_at", LogicalName: "注文日時", Type: "timestamp", Default: "CURRENT_TIMESTAMP"},
				},
				PrimaryKeys: []int{0},
				Indexes: []model.Index{
					{Name: "ix_orders_customer", Columns: []string{"customer_id"}},
					{Name: "ix_orders_customer_ordered", Columns: []string{"customer_id", "ordered_at"}},
				},
			},
		},
	}
	renderGolden(t, s, "full.png", "02_full.html")
}

// TestRender_NilSchema は nil 入力をエラーで弾くことを確認する（Fail Fast）。
func TestRender_NilSchema(t *testing.T) {
	got, err := Render(nil, "x.png")
	if err == nil {
		t.Fatalf("Render(nil) returned no error; got=%q", string(got))
	}
}

// TestRender_ContainsImageFilename は ImageFilename が <img src="..."> に
// 反映されることをスナップショットとは独立に確認する。
func TestRender_ContainsImageFilename(t *testing.T) {
	s := &model.Schema{
		Title: "x",
		Tables: []model.Table{
			{
				Name:        "t",
				Columns:     []model.Column{{Name: "id", Type: "int", IsPrimaryKey: true}},
				PrimaryKeys: []int{0},
			},
		},
	}
	got, err := Render(s, "diagram.png")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(got), `src="diagram.png"`) {
		t.Errorf("expected ImageFilename to be reflected in <img src>; got:\n%s", string(got))
	}
}

// TestRender_EscapesTitle はタイトルに含まれる HTML 特殊文字が
// エスケープされることを確認する（XSS 対策の維持）。
func TestRender_EscapesTitle(t *testing.T) {
	s := &model.Schema{
		Title: `<script>alert("x")</script>`,
		Tables: []model.Table{
			{
				Name:        "t",
				Columns:     []model.Column{{Name: "id", Type: "int", IsPrimaryKey: true}},
				PrimaryKeys: []int{0},
			},
		},
	}
	got, err := Render(s, "diagram.png")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(string(got), "<script>alert") {
		t.Errorf("expected <script> to be escaped, got raw script tag in output")
	}
}
