package elk

import (
	"strings"

	"github.com/unok/erdm/internal/model"
)

// 各ノードに付与する暫定サイズ。レイアウト計算に支障が出ない仮値で、
// SPA 側（タスク 7.4）でカラム数や表示密度に応じて再計算される想定
// （design.md §C6 では「仮値で開始」と整合）。
const (
	nodeWidth  = 200.0
	nodeHeight = 100.0
)

// buildRoot は Schema を elkjs 互換のルートツリーへ変換する。
//
// 仕様（design.md §C6 / 要件 4.2〜4.6）:
//   - 各 Table は elkNode { id, width, height } として表現する。
//   - primary グループは groupNode（親ノード）として表現し、所属テーブル
//     ノードを Children に格納する。
//   - secondary グループはテーブルノードの Properties.SecondaryGroups に
//     格納する（階層化はしない）。
//   - グループ未指定のテーブルは root.Children 直下に配置する。
//   - FK は親（FK.TargetTable）→ 子（FK 保有テーブル）方向で root.Edges に集約する。
//   - WithoutErd カラム由来のエッジは出力しない（要件 1.8 を ELK へ派生適用）。
func buildRoot(s *model.Schema) *elkRoot {
	root := &elkRoot{
		ID:    "root",
		Label: s.Title,
		LayoutOptions: &layoutOptions{
			Algorithm: "layered",
		},
	}
	root.Children = buildRootChildren(s)
	root.Edges = buildEdges(s)
	return root
}

// buildRootChildren は primary グループ単位の groupNode と、グループ未指定の
// テーブルノードを宣言順で集約する。
//
// Schema.Groups の登場順を維持し（要件 2.7）、各 Group に primary 所属する
// テーブルを Schema.Tables の宣言順で詰める。primary 所属テーブルが 0 件の
// グループ（secondary 専用グループ）は groupNode を出力しない（DOT cluster と
// 同方針、要件 2.11 の派生適用）。
func buildRootChildren(s *model.Schema) []*elkNode {
	children := make([]*elkNode, 0, len(s.Groups)+len(s.Tables))

	for _, name := range s.Groups {
		var members []*elkNode
		for ti := range s.Tables {
			t := &s.Tables[ti]
			primary, ok := t.PrimaryGroup()
			if !ok || primary != name {
				continue
			}
			members = append(members, buildTableNode(t))
		}
		if len(members) == 0 {
			continue
		}
		children = append(children, &elkNode{
			ID:       sanitizeID(name),
			Label:    name,
			Children: members,
		})
	}

	for ti := range s.Tables {
		t := &s.Tables[ti]
		if _, ok := t.PrimaryGroup(); ok {
			continue
		}
		children = append(children, buildTableNode(t))
	}

	if len(children) == 0 {
		return nil
	}
	return children
}

// buildTableNode は Table 1 件を elkjs 互換のノードへ変換する。
//
// Width/Height は仮値（nodeWidth × nodeHeight）。Label には論理名があれば
// それを優先し、無ければ物理名をそのまま用いる（SPA 表示用）。secondary
// グループが 1 つ以上あれば Properties.SecondaryGroups を設定する。
func buildTableNode(t *model.Table) *elkNode {
	node := &elkNode{
		ID:     sanitizeID(t.Name),
		Label:  tableLabel(t),
		Width:  nodeWidth,
		Height: nodeHeight,
	}
	if secondaries := t.SecondaryGroups(); len(secondaries) > 0 {
		node.Properties = &nodeProperties{SecondaryGroups: secondaries}
	}
	return node
}

// tableLabel は SPA 表示用ラベルを返す。論理名が空なら物理名で代用する。
func tableLabel(t *model.Table) string {
	if t.LogicalName != "" {
		return t.LogicalName
	}
	return t.Name
}

// buildEdges は全テーブルを走査して親 → 子方向の FK エッジ列を生成する。
//
// WithoutErd カラム由来のエッジは除外（要件 1.8 派生）。同一親子間の複数 FK
// は各カラムごとに独立 edge として連続して append する（要件 4.3、重複統合
// なし）。エッジ ID は `fk_<sanitized_child>_<sanitized_column>_<sanitized_parent>`
// 形式でユニーク化する（同一親子間でカラムが異なれば衝突しない）。
func buildEdges(s *model.Schema) []*elkEdge {
	var edges []*elkEdge
	for ti := range s.Tables {
		t := &s.Tables[ti]
		for ci := range t.Columns {
			c := &t.Columns[ci]
			if c.WithoutErd || c.FK == nil {
				continue
			}
			edges = append(edges, &elkEdge{
				ID:      edgeID(t.Name, c.Name, c.FK.TargetTable),
				Sources: []string{sanitizeID(c.FK.TargetTable)},
				Targets: []string{sanitizeID(t.Name)},
			})
		}
	}
	if len(edges) == 0 {
		return nil
	}
	return edges
}

// edgeID は FK エッジの ID を組み立てる。
//
// 同一親子間で複数 FK がある場合でもカラム名を含めることで衝突を避ける
// （要件 4.3 の独立エッジ要請、design.md §C6）。
func edgeID(child, column, parent string) string {
	return "fk_" + sanitizeID(child) + "_" + sanitizeID(column) + "_" + sanitizeID(parent)
}

// sanitizeID は識別子を `[A-Za-z0-9_]` 範囲に正規化する。
//
// elkjs のノード ID は文字列であれば良いが、後段（DOT への再変換 / SPA の
// HTML 属性化など）で扱いやすいよう、英数字とアンダースコア以外は `_` へ
// 置換する。元の名前は Label 側に保持されているため情報は失われない。
func sanitizeID(name string) string {
	if name == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}
