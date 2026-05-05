// Package dot は *model.Schema を Graphviz DOT 形式へ描画する。
//
// 公開境界は Render 関数のみ。テンプレートは embed.FS で同梱し、外部から
// 参照不可な状態でパッケージ内に閉じる（design.md §C5「テンプレート所有」）。
//
// 出力仕様（要件 1.1〜1.8 / 2.10〜2.12）:
//   - グラフ既定属性: rankdir=LR / splines=ortho / nodesep=0.8 / ranksep=1.2 /
//     concentrate=false
//   - エッジ方向: 親（参照される側）→ 子（FK を持つ側）
//   - 同一親子間の複数 FK は独立エッジとして列挙する（重複統合なし）
//   - WithoutErd（ERD 非表示）カラムから派生するエッジ・ノード行を除外
//   - primary グループは subgraph cluster_<name> 配下に配置
//   - secondary グループは DOT 出力に現れない
//   - グループ未指定テーブルはサブグラフに属さない
package dot

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"text/template"

	"github.com/unok/erdm/internal/model"
)

//go:embed templates/*.tmpl
var templatesFS embed.FS

// Render は Schema を DOT テキストへ描画する。
//
// s が nil の場合はエラーを返す。テンプレート解析・実行で失敗した場合も
// その原因をラップしたエラーを返す（フォールバックは行わない）。
func Render(s *model.Schema) (string, error) {
	if s == nil {
		return "", errors.New("dot: schema is nil")
	}
	tmpl, err := template.ParseFS(templatesFS, "templates/*.tmpl")
	if err != nil {
		return "", fmt.Errorf("dot: parse templates: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "dot", buildView(s)); err != nil {
		return "", fmt.Errorf("dot: execute template: %w", err)
	}
	return buf.String(), nil
}
