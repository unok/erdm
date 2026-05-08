package elk

// elkRoot は elkjs ElkNode 互換のルート要素。
//
// elkjs 仕様（research.md §3.2）の最小サブセットに合わせており、
// JSON 出力時のキー名はキャメルケースで `id`/`children`/`edges`/`label`/
// `layoutOptions` を使用する。空フィールドは `omitempty` で省略する。
type elkRoot struct {
	ID            string         `json:"id"`
	Label         string         `json:"label,omitempty"`
	Children      []*elkNode     `json:"children,omitempty"`
	Edges         []*elkEdge     `json:"edges,omitempty"`
	LayoutOptions *layoutOptions `json:"layoutOptions,omitempty"`
}

// elkNode は elkjs ElkNode 互換のノード要素。
//
// テーブルノード／groupNode（primary グループの親ノード）のいずれにも
// 用いられる。groupNode の場合は Width/Height は 0（`omitempty` で省略）、
// Children に所属テーブルを格納する。テーブルノードの場合は Width/Height を
// 仮値（200×100）で埋め、Children は空。
//
// Properties は elkjs が無視するカスタム属性置き場で、secondary グループ名
// 一覧の保持に使う（design.md §C6 / 要件 4.5）。
type elkNode struct {
	ID         string          `json:"id"`
	Label      string          `json:"label,omitempty"`
	Width      float64         `json:"width,omitempty"`
	Height     float64         `json:"height,omitempty"`
	Children   []*elkNode      `json:"children,omitempty"`
	Properties *nodeProperties `json:"properties,omitempty"`
}

// elkEdge は elkjs ElkEdge 互換のエッジ要素。
//
// elkjs 仕様で必須となる `id`/`sources`/`targets` を備える。
// `sources` は親（参照される側 = FK.TargetTable）、`targets` は子（FK を持つ側）
// に対応するノード ID 配列（要件 4.3）。
type elkEdge struct {
	ID      string   `json:"id"`
	Sources []string `json:"sources"`
	Targets []string `json:"targets"`
}

// nodeProperties はテーブルノードに付与するカスタム属性。
//
// elkjs はノードの `properties` を「文字列マップ的なメタ情報」として扱い
// レイアウト計算には使わない（research.md §3.2）。本パッケージでは
// secondary グループ名一覧を `secondaryGroups` キーで保持する（要件 4.5）。
type nodeProperties struct {
	SecondaryGroups []string `json:"secondaryGroups,omitempty"`
}

// layoutOptions は elkjs に渡すレイアウト指示の最小サブセット。
//
// 現状は algorithm のみ保持する。追加のキーが必要になった場合は SPA 側
// （タスク 7.4）で書き換える前提でフィールドを増やす。
type layoutOptions struct {
	Algorithm string `json:"algorithm,omitempty"`
}
