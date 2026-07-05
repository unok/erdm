// Package elk は *model.Schema を elkjs 互換の ELK JSON へ描画する。
//
// 公開境界は Render 関数のみ（design.md §C6「`internal/elk` の公開 API は
// Render(*model.Schema) ([]byte, error) のみ」）。ELK 互換構造体・ビルダ関数は
// パッケージ内部に閉じる。
//
// 出力仕様（要件 4.2〜4.7）:
//   - 各 Table は `{ id, label, width, height }` の elkjs ElkNode として出力する。
//   - 各 FK は `{ id, sources, targets }` の elkjs ElkEdge として親（参照される側）→
//     子（FK を持つ側）方向で出力する。
//   - primary グループは `children` を持つ groupNode（親ノード）として表現し、
//     対応するテーブルノードを当該 groupNode の `children` に格納する。
//   - secondary グループはテーブルノードの `properties.secondaryGroups` 配列
//     として保持し、階層化はしない。
//   - グループ未指定のテーブルは root.children 直下に配置する。
//   - 出力 JSON は elkjs 標準入力フォーマットを満たす（ElkNode 仕様）。
//
// 利用箇所:
//   - CLI の `render --format=elk`（タスク 4.3 で配線）
//   - SPA 起動時の自動配置（タスク 7.4）
package elk

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/unok/erdm/internal/model"
)

// Render は Schema を elkjs 互換の ELK JSON バイト列へ描画する。
//
// s が nil の場合はエラーを返す（Fail Fast、ポリシー「フォールバック禁止」）。
// 出力は 2 スペースインデント整形＋末尾改行付きで、ゴールデンファイル比較を
// 容易にする。
func Render(s *model.Schema) ([]byte, error) {
	if s == nil {
		return nil, errors.New("elk: schema is nil")
	}
	root := buildRoot(s)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(root); err != nil {
		return nil, fmt.Errorf("elk: encode json: %w", err)
	}
	return buf.Bytes(), nil
}
