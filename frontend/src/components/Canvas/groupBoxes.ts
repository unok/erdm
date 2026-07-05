// primary グループの可視化用「背景ボックス」ノードを計算する純粋関数
// （#27、ADR-0001）。
//
// 方針:
//   - テーブルノードは従来どおりトップレベルの絶対座標のまま（永続化=絶対座標を
//     維持）。グループ枠はメンバーテーブルの外接矩形（bbox）に余白を足した
//     非インタラクティブな背景ノードとして描画する。
//   - primary グループのみを枠にする（secondary は対象外、DOT cluster と同方針）。
//   - メンバーが 0 件のグループは枠を出さない。
//   - ノードの実寸はレンダリング後に React Flow が測って width/height に入れる。
//     未測定時は既定値で近似する（初回描画のちらつきは許容範囲）。

import type { Node } from 'reactflow'
import { primaryGroup, type Schema } from '../../model'

// グループ枠ノードの id 接頭辞。テーブルノード（= Table.Name）と区別し、
// 座標保存・クリック選択の対象外にするために用いる。
export const GROUP_BOX_PREFIX = 'group:'

export function isGroupBoxId(id: string): boolean {
  return id.startsWith(GROUP_BOX_PREFIX)
}

// React Flow のカスタムノード種別名（Canvas の nodeTypes に登録）。
export const GROUP_BOX_NODE_TYPE = 'groupBox'

// 実寸未測定時の近似値と余白。
const DEFAULT_NODE_WIDTH = 150
const DEFAULT_NODE_HEIGHT = 40
const PADDING = 24
const LABEL_HEIGHT = 22

// computeGroupBoxes は現在のテーブルノード配置から primary グループの背景枠
// ノード列を計算する。テーブルが動けば（再計算されれば）枠も追従する。
export function computeGroupBoxes(schema: Schema, tableNodes: Node[]): Node[] {
  const byName = new Map(tableNodes.map((n) => [n.id, n]))

  // テーブルを 1 パスで走査し、primary グループの初出順（Schema.Groups 非同期な
  // 下書きにも頑健）とメンバー Node を同時に収集する（グループ×テーブルの
  // 二重走査を避ける）。
  const order: string[] = []
  const membersByGroup = new Map<string, Node[]>()
  for (const t of schema.Tables) {
    const pg = primaryGroup(t)
    if (pg === null) continue
    let members = membersByGroup.get(pg)
    if (members === undefined) {
      members = []
      membersByGroup.set(pg, members)
      order.push(pg)
    }
    const node = byName.get(t.Name)
    if (node !== undefined) members.push(node)
  }

  const boxes: Node[] = []
  for (const name of order) {
    const members = membersByGroup.get(name) ?? []
    if (members.length === 0) continue

    let minX = Infinity
    let minY = Infinity
    let maxX = -Infinity
    let maxY = -Infinity
    for (const m of members) {
      const w = m.width ?? DEFAULT_NODE_WIDTH
      const h = m.height ?? DEFAULT_NODE_HEIGHT
      minX = Math.min(minX, m.position.x)
      minY = Math.min(minY, m.position.y)
      maxX = Math.max(maxX, m.position.x + w)
      maxY = Math.max(maxY, m.position.y + h)
    }

    boxes.push({
      id: GROUP_BOX_PREFIX + name,
      type: GROUP_BOX_NODE_TYPE,
      position: { x: minX - PADDING, y: minY - PADDING - LABEL_HEIGHT },
      data: { label: name },
      style: {
        width: maxX - minX + PADDING * 2,
        height: maxY - minY + PADDING * 2 + LABEL_HEIGHT,
      },
      draggable: false,
      selectable: false,
      connectable: false,
      deletable: false,
      // テーブルノードより背面へ。
      zIndex: -1,
    })
  }
  return boxes
}
