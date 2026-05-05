// ELK 連携: スキーマから ELK 入力グラフを構築し、自動レイアウトを計算する
// （タスク 7.4 / 要件 6.4 / 6.5、design.md §C11）。
//
// 設計判断:
//   - `elkjs` のバンドル版（`elk.bundled.js`）を採用。Web Worker を使わず、
//     呼び出し側の `useEffect` 内で `await` するだけで完結する。
//   - 公開関数は `computeLayout(schema)` のみ。マージは `merge.ts` の責務。
//   - ノード幅/高さは固定値（200x100）。カラム数連動は将来検討（design.md
//     §C11、tasks.md 7.4 のスコープ外）。
//   - エッジ方向は親（FK.TargetTable）→ 子（テーブル）。要件 1.6 と整合。
//   - `WithoutErd === true` のカラムは ERD に現れないため、対応する FK は
//     ELK 入力にも含めない（要件 1.8 と整合）。
//   - ELK のレイアウトオプションは要件 1.1（rankdir=LR 相当）と DOT レンダラ
//     既定値（`internal/dot`）に合わせる:
//       * `elk.algorithm = layered`
//       * `elk.direction = RIGHT`
//       * `elk.spacing.nodeNode = 50`
//       * `elk.layered.spacing.nodeNodeBetweenLayers = 80`

import ELK, { type ElkNode, type ElkExtendedEdge } from 'elkjs/lib/elk.bundled.js'
import type { Layout, Schema } from '../model'

const NODE_WIDTH = 200
const NODE_HEIGHT = 100

const ROOT_LAYOUT_OPTIONS: Record<string, string> = {
  'elk.algorithm': 'layered',
  'elk.direction': 'RIGHT',
  'elk.spacing.nodeNode': '50',
  'elk.layered.spacing.nodeNodeBetweenLayers': '80',
}

// computeLayout はスキーマ全体に対して ELK 自動配置を計算し、テーブル名 →
// 座標 の `Layout` を返す。既存座標とのマージは行わない（merge.ts に委譲）。
export async function computeLayout(schema: Schema): Promise<Layout> {
  const elk = new ELK()
  const input = buildElkInput(schema)
  const result = await elk.layout(input)
  return extractPositions(result)
}

// buildElkInput はスキーマから ELK 入力グラフ（root ノード）を構築する。
function buildElkInput(schema: Schema): ElkNode {
  const children: ElkNode[] = schema.Tables.map((t) => ({
    id: t.Name,
    width: NODE_WIDTH,
    height: NODE_HEIGHT,
  }))

  const edges: ElkExtendedEdge[] = []
  for (const t of schema.Tables) {
    for (const c of t.Columns) {
      if (c.WithoutErd) continue
      if (c.FK === null) continue
      edges.push({
        id: `${c.FK.TargetTable}__${t.Name}__${c.Name}`,
        sources: [c.FK.TargetTable],
        targets: [t.Name],
      })
    }
  }

  return {
    id: 'root',
    layoutOptions: ROOT_LAYOUT_OPTIONS,
    children,
    edges,
  }
}

// extractPositions は ELK レイアウト結果から各テーブルの (x, y) を抽出する。
// ELK は通常 root の children に各ノードの x/y を埋めて返す。
function extractPositions(result: ElkNode): Layout {
  const layout: Layout = {}
  const children = result.children ?? []
  for (const node of children) {
    if (typeof node.x !== 'number' || typeof node.y !== 'number') {
      // ELK が座標を返さなかった場合は Fail Fast。レイアウト計算の異常を
      // サイレントに 0,0 で隠蔽しない。
      throw new Error(
        `ELK layout did not return coordinates for node "${node.id}"`,
      )
    }
    layout[node.id] = { x: node.x, y: node.y }
  }
  return layout
}
