// Package dot のスナップショットテスト。
//
// 各ケースは *model.Schema を Go リテラルで構築し、Render の出力をゴールデン
// ファイル（testdata/golden/*.dot）と比較する。
//
// 旧テンプレ（リポジトリルート templates/*.tmpl）出力との差分は、要件 1.1〜1.5
// 由来のグラフ既定属性（rankdir/splines/nodesep/ranksep/concentrate）と要件 1.6
// 由来の親→子方向反転（headlabel/taillabel の入れ替えを含む）に限定される。
// 旧出力との直接 diff はバッチ 8.1（横断品質）で網羅し、本テストは新出力を
// ゴールデンで固定することに特化する。
//
// Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8, 1.9, 2.10, 2.11, 2.12, 3.4, 3.6, 9.6
package dot

import (
	"strings"
	"testing"

	"github.com/unok/erdm/internal/model"
	"github.com/unok/erdm/internal/testutil/golden"
)

// renderGolden は Render の出力をゴールデンファイルと比較するヘルパ。
func renderGolden(t *testing.T, s *model.Schema, goldenName string) {
	t.Helper()
	got, err := Render(s)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	golden.Compare(t, []byte(got), "testdata/golden/"+goldenName)
}

func TestRender_SimpleUngrouped(t *testing.T) {
	// 要件 1.1〜1.5（既定属性）/ 1.6（親→子）/ 2.12（ungrouped はトップレベル）
	s := &model.Schema{
		Title: "simple_ungrouped",
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
	renderGolden(t, s, "01_simple_ungrouped.dot")
}

func TestRender_PrimaryGroupOnly(t *testing.T) {
	// 要件 2.10（cluster_<name> 配下に配置）
	s := &model.Schema{
		Title:  "primary_group_only",
		Groups: []string{"auth"},
		Tables: []model.Table{
			{
				Name:   "users",
				Groups: []string{"auth"},
				Columns: []model.Column{
					{Name: "id", Type: "int", IsPrimaryKey: true},
					{Name: "name", Type: "varchar(64)"},
				},
				PrimaryKeys: []int{0},
			},
			{
				Name:   "sessions",
				Groups: []string{"auth"},
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
	renderGolden(t, s, "02_primary_group_only.dot")
}

func TestRender_SecondaryOnly(t *testing.T) {
	// 要件 2.11（secondary は DOT に現れない）
	// audit は登場順登録されているが、どのテーブルも primary としていない。
	// → cluster_audit は出力されない。
	s := &model.Schema{
		Title:  "secondary_only",
		Groups: []string{"core", "audit"},
		Tables: []model.Table{
			{
				Name:   "users",
				Groups: []string{"core", "audit"},
				Columns: []model.Column{
					{Name: "id", Type: "int", IsPrimaryKey: true},
					{Name: "name", Type: "varchar(64)"},
				},
				PrimaryKeys: []int{0},
			},
			{
				Name:   "events",
				Groups: []string{"core", "audit"},
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
	renderGolden(t, s, "03_secondary_only.dot")
}

func TestRender_UngroupedMixed(t *testing.T) {
	// 要件 2.10 + 2.12（cluster と top-level の混在）
	s := &model.Schema{
		Title:  "ungrouped_mixed",
		Groups: []string{"auth"},
		Tables: []model.Table{
			{
				Name:   "users",
				Groups: []string{"auth"},
				Columns: []model.Column{
					{Name: "id", Type: "int", IsPrimaryKey: true},
					{Name: "name", Type: "varchar(64)"},
				},
				PrimaryKeys: []int{0},
			},
			{
				Name: "guests",
				Columns: []model.Column{
					{Name: "id", Type: "int", IsPrimaryKey: true},
					{Name: "token", Type: "varchar(32)", IsUnique: true},
				},
				PrimaryKeys: []int{0},
			},
		},
	}
	renderGolden(t, s, "04_ungrouped_mixed.dot")
}

func TestRender_MultiFKToSameParent(t *testing.T) {
	// 要件 1.7（同一親子間の複数 FK は独立エッジ）
	s := &model.Schema{
		Title: "multi_fk_to_same_parent",
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
				Name: "audit_logs",
				Columns: []model.Column{
					{Name: "id", Type: "int", IsPrimaryKey: true},
					{
						Name: "actor_id", Type: "int",
						FK: &model.FK{
							TargetTable:            "users",
							CardinalitySource:      "0..*",
							CardinalityDestination: "1",
						},
					},
					{
						Name: "target_id", Type: "int",
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
	renderGolden(t, s, "05_multi_fk_to_same_parent.dot")
}

func TestRender_WithoutErdExcluded(t *testing.T) {
	// 要件 1.8（ERD 非表示カラム由来のエッジ・ノード行を除外）
	s := &model.Schema{
		Title: "without_erd_excluded",
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
				Name: "legacy",
				Columns: []model.Column{
					{Name: "id", Type: "int", IsPrimaryKey: true},
					{
						Name: "user_id", Type: "int",
						WithoutErd: true,
						FK: &model.FK{
							TargetTable:            "users",
							CardinalitySource:      "0..*",
							CardinalityDestination: "1",
						},
					},
					{Name: "note", Type: "text"},
				},
				PrimaryKeys: []int{0},
			},
		},
	}
	renderGolden(t, s, "06_without_erd_excluded.dot")
}

func TestRender_CompositePKAndCardinality(t *testing.T) {
	// 要件 1.6（head/tail label 入れ替え後の正しい配置）+ 複合 PK 表示
	s := &model.Schema{
		Title: "composite_pk_and_cardinality",
		Tables: []model.Table{
			{
				Name:        "customers",
				LogicalName: "顧客",
				Columns: []model.Column{
					{Name: "id", Type: "int", IsPrimaryKey: true},
					{Name: "name", Type: "varchar(64)"},
				},
				PrimaryKeys: []int{0},
			},
			{
				Name: "orders",
				Columns: []model.Column{
					{Name: "id", Type: "int", IsPrimaryKey: true},
					{
						Name: "customer_id", Type: "int",
						FK: &model.FK{
							TargetTable:            "customers",
							CardinalitySource:      "0..*",
							CardinalityDestination: "1",
						},
					},
					{Name: "status", Type: "varchar(16)"},
				},
				PrimaryKeys: []int{0},
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
					{Name: "qty", Type: "int"},
				},
				PrimaryKeys: []int{0, 1},
			},
		},
	}
	renderGolden(t, s, "07_composite_pk_and_cardinality.dot")
}

// TestRender_DefaultAttributesAlwaysPresent は出力が必ず要件 1.1〜1.5 の既定属性を
// 含むことを、ゴールデンとは独立に確認する（要件 3.6 の差分明示を支える）。
func TestRender_DefaultAttributesAlwaysPresent(t *testing.T) {
	required := []string{
		"rankdir=LR",
		"splines=ortho",
		"nodesep=0.8",
		"ranksep=1.2",
		"concentrate=false",
	}
	s := &model.Schema{
		Title: "minimal",
		Tables: []model.Table{
			{
				Name: "t",
				Columns: []model.Column{
					{Name: "id", Type: "int", IsPrimaryKey: true},
				},
				PrimaryKeys: []int{0},
			},
		},
	}
	got, err := Render(s)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, attr := range required {
		if !strings.Contains(got, attr) {
			t.Errorf("output missing required attribute %q\n--- output ---\n%s", attr, got)
		}
	}
}

// TestRender_NilSchema は nil 入力をエラーで弾くことを確認する（Fail Fast）。
func TestRender_NilSchema(t *testing.T) {
	got, err := Render(nil)
	if err == nil {
		t.Fatalf("Render(nil) returned no error; got=%q", got)
	}
}

// TestRender_EdgeFallsBackToTableWhenParentHasNoPK は、親テーブルが PK を
// 持たない場合に親側のポート指定が省略され、テーブル枠への接続にフォール
// バックすることを確認する。子側 FK 列のポート指定は維持される。
func TestRender_EdgeFallsBackToTableWhenParentHasNoPK(t *testing.T) {
	s := &model.Schema{
		Title: "no_pk_parent",
		Tables: []model.Table{
			{
				Name:        "raw_log",
				Columns:     []model.Column{{Name: "value", Type: "text"}},
				PrimaryKeys: nil, // 意図的に PK 無し
			},
			{
				Name: "consumers",
				Columns: []model.Column{
					{Name: "id", Type: "int", IsPrimaryKey: true},
					{
						Name: "raw_log_ref", Type: "int",
						FK: &model.FK{
							TargetTable:            "raw_log",
							CardinalitySource:      "0..*",
							CardinalityDestination: "1",
						},
					},
				},
				PrimaryKeys: []int{0},
			},
		},
	}
	got, err := Render(s)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// 親 raw_log に PK が無いので tail ポートは付かず、子側 FK 列ポートだけが付く。
	if !strings.Contains(got, "raw_log -> consumers:raw_log_ref:w") {
		t.Errorf("expected `raw_log -> consumers:raw_log_ref:w` (tail without port, head with FK port), got:\n%s", got)
	}
	// 親側に :id:e が現れていないことも併せて確認（誤って出すと誤接続になる）。
	if strings.Contains(got, "raw_log:") {
		t.Errorf("parent raw_log should not have a port suffix, got:\n%s", got)
	}
}

// TestRender_EdgeOrientation は親→子方向反転の意味的整合を機械的に検証する。
//
// posts.user_id → users.id の FK において、
//   - 矢尾（tail、Parent 側）= users
//   - 矢頭（head、Child 側）= posts
//   - taillabel = CardinalityDestination（親側）
//   - headlabel = CardinalitySource（子側）
//
// が出力に正しく反映されることを substring 比較で確認する（要件 1.6 と
// design.md §C5 の方向反転規則）。
func TestRender_EdgeOrientation(t *testing.T) {
	s := &model.Schema{
		Title: "edge_orientation",
		Tables: []model.Table{
			{
				Name: "users",
				Columns: []model.Column{
					{Name: "id", Type: "int", IsPrimaryKey: true},
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
							CardinalitySource:      "SRC",
							CardinalityDestination: "DST",
						},
					},
				},
				PrimaryKeys: []int{0},
			},
		},
	}
	got, err := Render(s)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// ポート付きエッジ `users:id:e -> posts:user_id:w` は親→子方向の正規化を
	// 維持しつつ、接続点を「親 PK 列の東側」「子 FK 列の西側」に固定する形で
	// 拡張された表記。方向反転の意味的整合は変わらない。
	if !strings.Contains(got, "users:id:e -> posts:user_id:w") {
		t.Errorf("expected edge `users:id:e -> posts:user_id:w` (parent PK column -> child FK column), got:\n%s", got)
	}
	if !strings.Contains(got, `headlabel = "SRC"`) {
		t.Errorf("expected `headlabel = \"SRC\"` (子側=Source), got:\n%s", got)
	}
	if !strings.Contains(got, `taillabel = "DST"`) {
		t.Errorf("expected `taillabel = \"DST\"` (親側=Destination), got:\n%s", got)
	}
}
