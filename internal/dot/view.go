package dot

import (
	"strings"

	"github.com/unok/erdm/internal/model"
)

// dotView はテンプレートに渡す DOT 描画用ビューモデル。
//
// Schema を直接渡さない理由は、cluster 単位の整形（primary グループでの
// バケット分け、識別子サニタイズ）と FK エッジ列の事前整形を Go 側で済ませて
// テンプレートを単純なループに留めるため（design.md §C5）。
type dotView struct {
	Clusters        []dotCluster
	UngroupedTables []*model.Table
	Edges           []dotEdge
}

// dotCluster は primary グループ単位の subgraph 描画情報。
//
// Name は DOT 識別子としてサニタイズしたグループ名（`subgraph cluster_<Name>`
// に埋め込む）。Label は元のグループ名（`label="<Label>"` に埋め込む）。
type dotCluster struct {
	Name   string
	Label  string
	Tables []*model.Table
}

// dotEdge は親 → 子方向に正規化した FK エッジ。
//
// 矢尾（tail）= 親（参照される側 = FK.TargetTable）、
// 矢頭（head）= 子（FK カラムを持つ側 = カラム所属テーブル）。
// HeadLabel は子側 cardinality（FK.CardinalitySource）、
// TailLabel は親側 cardinality（FK.CardinalityDestination）。
//
// TailPort / HeadPort は HTML ラベル table 内の `<td port="...">` に対応する
// ポート名で、エッジを「テーブル枠」ではなく「該当カラムの行」から出すための
// 接続点を指定する（要件: ER 図でリレーションをカラム単位の繋がりとして
// 視認できるようにする）。
//   - HeadPort: 子テーブル側の FK 列名（必ず分かる）。
//   - TailPort: 親テーブル側の主キー先頭列名。親に PK が無い／FK 参照先が
//     スコープ外で解決できない場合は空文字列とし、ポート指定なし（テーブル
//     枠への接続）にフォールバックする。
type dotEdge struct {
	Parent    string
	Child     string
	HeadLabel string
	TailLabel string
	TailPort  string
	HeadPort  string
}

// buildView は Schema からビューモデルを派生する。
func buildView(s *model.Schema) dotView {
	return dotView{
		Clusters:        buildClusters(s),
		UngroupedTables: collectUngroupedTables(s),
		Edges:           buildEdges(s),
	}
}

// buildClusters は primary グループごとに所属テーブルを集約する。
//
// Schema.Groups の登場順を維持し、各 Group に primary 所属するテーブルを
// Schema.Tables の宣言順で詰める。secondary 所属は DOT 出力に出さないため
// 無視する（要件 2.11）。primary 所属テーブルが 0 件のグループは cluster を
// 出力しない（要件 2.11：secondary でしか参照されないグループは DOT に
// 現れない）。
func buildClusters(s *model.Schema) []dotCluster {
	if len(s.Groups) == 0 {
		return nil
	}
	clusters := make([]dotCluster, 0, len(s.Groups))
	for _, name := range s.Groups {
		var tables []*model.Table
		for ti := range s.Tables {
			t := &s.Tables[ti]
			primary, ok := t.PrimaryGroup()
			if !ok || primary != name {
				continue
			}
			tables = append(tables, t)
		}
		if len(tables) == 0 {
			continue
		}
		clusters = append(clusters, dotCluster{
			Name:   sanitizeIdentifier(name),
			Label:  name,
			Tables: tables,
		})
	}
	return clusters
}

// collectUngroupedTables はグループ未指定テーブルを宣言順で集約する。
func collectUngroupedTables(s *model.Schema) []*model.Table {
	var out []*model.Table
	for ti := range s.Tables {
		t := &s.Tables[ti]
		if _, ok := t.PrimaryGroup(); ok {
			continue
		}
		out = append(out, t)
	}
	return out
}

// buildEdges は全テーブルを走査して親 → 子方向の FK エッジ列を生成する。
//
// WithoutErd カラム由来のエッジは除外（要件 1.8）。同一親子間の複数 FK は
// 各カラムごとに独立 edge として連続して append する（要件 1.7、重複統合なし）。
//
// 接続点（ポート）の解決:
//   - HeadPort: 子側 FK 列名をそのまま採用する。
//   - TailPort: 親テーブルの先頭 PK 列名を採用する。親が見つからない／PK
//     未定義の場合は空文字列としてテンプレ側でポート指定をスキップする。
func buildEdges(s *model.Schema) []dotEdge {
	pkPortByTable := buildPKPortIndex(s.Tables)
	var out []dotEdge
	for ti := range s.Tables {
		t := &s.Tables[ti]
		for ci := range t.Columns {
			c := &t.Columns[ci]
			if c.WithoutErd || c.FK == nil {
				continue
			}
			out = append(out, dotEdge{
				Parent:    c.FK.TargetTable,
				Child:     t.Name,
				HeadLabel: c.FK.CardinalitySource,
				TailLabel: c.FK.CardinalityDestination,
				TailPort:  pkPortByTable[c.FK.TargetTable],
				HeadPort:  c.Name,
			})
		}
	}
	return out
}

// buildPKPortIndex は (テーブル名 → 先頭 PK 列名) の索引を作る。PK が無い
// テーブルはエントリを持たず、呼び出し側は空文字列扱いになる。
func buildPKPortIndex(tables []model.Table) map[string]string {
	index := make(map[string]string, len(tables))
	for ti := range tables {
		t := &tables[ti]
		if len(t.PrimaryKeys) == 0 {
			continue
		}
		idx := t.PrimaryKeys[0]
		if idx < 0 || idx >= len(t.Columns) {
			continue
		}
		index[t.Name] = t.Columns[idx].Name
	}
	return index
}

// sanitizeIdentifier はグループ名を DOT 識別子として安全な形に整える。
//
// DOT 識別子は `[A-Za-z_][A-Za-z0-9_]*` を満たす必要があるため、それ以外の
// 文字は `_` に置換する。`subgraph cluster_<Name>` の前置詞があるため先頭が
// 数字でも問題ない（先頭が `cluster_` で固定される）。
func sanitizeIdentifier(name string) string {
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
