// Package elk のスナップショット／構造整合性テスト。
//
// 各ケースは *model.Schema を Go リテラルで構築し、Render の出力を
// ゴールデンファイル（testdata/golden/*.json）と比較する。加えて、生成された
// JSON が elkjs 互換の構造体へ再 unmarshal 可能であること、エッジの参照先
// ノードが ID 集合に含まれていること、ID がユニークであることを検証する
// （要件 4.7 のレイアウトエンジン整合性を構造レベルで担保）。
//
// 実 elkjs（Node.js）を起動した通過テストはタスク 7.8（フロントエンドテスト）
// で実施するため本パッケージのスコープ外。
//
// Requirements: 4.2, 4.3, 4.4, 4.5, 4.6, 4.7
package elk

import (
	"encoding/json"
	"testing"

	"github.com/unok/erdm/internal/model"
	"github.com/unok/erdm/internal/testutil/golden"
)

// renderGolden は Render 出力をゴールデンファイルと比較し、構造整合性も検査する。
func renderGolden(t *testing.T, s *model.Schema, goldenName string) {
	t.Helper()
	got, err := Render(s)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	golden.Compare(t, got, "testdata/golden/"+goldenName)
	assertStructuralIntegrity(t, got)
}

// assertStructuralIntegrity は出力 JSON が elkjs 互換構造へ再 unmarshal でき、
// エッジの参照先がノード ID 集合に含まれ、ID がユニークであることを検証する。
func assertStructuralIntegrity(t *testing.T, got []byte) {
	t.Helper()
	var root elkRoot
	if err := json.Unmarshal(got, &root); err != nil {
		t.Fatalf("unmarshal: %v\noutput:\n%s", err, string(got))
	}

	ids := collectNodeIDs(&root)
	if dup := firstDuplicate(ids); dup != "" {
		t.Errorf("duplicate node id: %q", dup)
	}

	idSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	for _, e := range root.Edges {
		if e.ID == "" {
			t.Errorf("edge id must not be empty")
		}
		if len(e.Sources) == 0 || len(e.Targets) == 0 {
			t.Errorf("edge %q must have non-empty sources/targets", e.ID)
		}
		for _, src := range e.Sources {
			if _, ok := idSet[src]; !ok {
				t.Errorf("edge %q references unknown source node %q", e.ID, src)
			}
		}
		for _, dst := range e.Targets {
			if _, ok := idSet[dst]; !ok {
				t.Errorf("edge %q references unknown target node %q", e.ID, dst)
			}
		}
	}

	edgeIDs := make([]string, len(root.Edges))
	for i, e := range root.Edges {
		edgeIDs[i] = e.ID
	}
	if dup := firstDuplicate(edgeIDs); dup != "" {
		t.Errorf("duplicate edge id: %q", dup)
	}
}

// collectNodeIDs は root 配下の全 elkNode から ID を再帰的に集める。
func collectNodeIDs(root *elkRoot) []string {
	out := make([]string, 0)
	var walk func(nodes []*elkNode)
	walk = func(nodes []*elkNode) {
		for _, n := range nodes {
			out = append(out, n.ID)
			walk(n.Children)
		}
	}
	walk(root.Children)
	return out
}

// firstDuplicate は重複する最初の要素を返す。重複がなければ空文字。
func firstDuplicate(values []string) string {
	seen := make(map[string]struct{}, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			return v
		}
		seen[v] = struct{}{}
	}
	return ""
}

func TestRender_SimpleUngrouped(t *testing.T) {
	// 要件 4.2（width/height）/ 4.3（親→子）/ 4.6（ungrouped はルート直下）
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
	renderGolden(t, s, "01_simple_ungrouped.json")
}

func TestRender_PrimaryGroupOnly(t *testing.T) {
	// 要件 4.4（primary group は groupNode の children に格納）
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
	renderGolden(t, s, "02_primary_group_only.json")
}

func TestRender_PrimaryAndSecondary(t *testing.T) {
	// 要件 4.5（secondary はテーブルノードの properties.secondaryGroups に保持）
	s := &model.Schema{
		Title:  "primary_and_secondary",
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
	renderGolden(t, s, "03_primary_and_secondary.json")
}

func TestRender_UngroupedMixed(t *testing.T) {
	// 要件 4.4 + 4.6（groupNode 配下と root 直下の混在）
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
	renderGolden(t, s, "04_ungrouped_mixed.json")
}

func TestRender_MultiFKToSameParent(t *testing.T) {
	// 要件 4.3（同一親子間の複数 FK は独立エッジ、ID 衝突なし）
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
	renderGolden(t, s, "05_multi_fk_to_same_parent.json")
}

func TestRender_WithoutErdExcluded(t *testing.T) {
	// 要件 1.8 を ELK へ派生適用（WithoutErd カラム由来のエッジを除外）
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
	renderGolden(t, s, "06_without_erd_excluded.json")
}

// TestRender_NilSchema は nil 入力をエラーで弾くことを確認する（Fail Fast）。
func TestRender_NilSchema(t *testing.T) {
	got, err := Render(nil)
	if err == nil {
		t.Fatalf("Render(nil) returned no error; got=%q", string(got))
	}
}

// TestRender_EdgeOrientation は親→子方向（要件 4.3）を機械的に検証する。
//
// posts.user_id → users.id の FK において、
//   - sources = [users]（親、参照される側）
//   - targets = [posts]（子、FK を持つ側）
//
// が出力に正しく反映されることを構造的に確認する。
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
	var root elkRoot
	if err := json.Unmarshal(got, &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(root.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(root.Edges))
	}
	e := root.Edges[0]
	if got, want := e.Sources, []string{"users"}; !equalSlices(got, want) {
		t.Errorf("sources = %v, want %v", got, want)
	}
	if got, want := e.Targets, []string{"posts"}; !equalSlices(got, want) {
		t.Errorf("targets = %v, want %v", got, want)
	}
}

// TestRender_RootHasLayoutAlgorithm は root に layoutOptions.algorithm が
// 付与されることを確認する。SPA 側のレイアウト計算が安定して動くために
// 必要な最低限の指示。
func TestRender_RootHasLayoutAlgorithm(t *testing.T) {
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
	var root elkRoot
	if err := json.Unmarshal(got, &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if root.LayoutOptions == nil || root.LayoutOptions.Algorithm == "" {
		t.Errorf("expected root.layoutOptions.algorithm to be set, got %+v", root.LayoutOptions)
	}
}

// equalSlices は文字列スライスの内容比較。テスト用の小さなヘルパ。
func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
