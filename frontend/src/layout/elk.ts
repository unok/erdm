// ELK 連携: スキーマから ELK 入力グラフを構築し、自動レイアウトを計算する
// （タスク 7.4 / 要件 6.4 / 6.5、design.md §C11、ADR-0001）。
//
// 設計判断:
//   - `elkjs` のバンドル版（`elk.bundled.js`）を採用。Web Worker を使わず、
//     呼び出し側の `useEffect` 内で `await` するだけで完結する。
//   - 公開関数は `computeLayout(schema)` のみ。マージは `merge.ts` の責務。
//     （`buildElkInput` / `extractPositions` はクロスチェック・単体テスト用に公開）
//   - primary グループは ELK の階層（親ノードの children）として表現し、所属
//     テーブルがまとまって配置されるようにする（ADR-0001、Go `internal/elk`
//     の groupNode と構造を一致させる）。secondary グループはレイアウトに影響
//     させない。
//   - ノード幅/高さは固定値（200x100）。カラム数連動は将来検討（design.md §C11）。
//   - エッジ方向は親（FK.TargetTable）→ 子（テーブル）。要件 1.6 と整合。
//   - `WithoutErd === true` のカラムは ERD に現れないため、対応する FK は
//     ELK 入力にも含めない（要件 1.8 と整合）。
//   - ELK のレイアウトオプションは要件 1.1（rankdir=LR 相当）と DOT レンダラ
//     既定値（`internal/dot`）に合わせる。階層エッジを跨いでレイアウトさせる
//     ため `elk.hierarchyHandling = INCLUDE_CHILDREN` を指定する。

import ELK, { type ElkNode, type ElkExtendedEdge } from 'elkjs/lib/elk.bundled.js'
import { primaryGroup, type Layout, type Schema } from '../model'

const NODE_WIDTH = 200
const NODE_HEIGHT = 100

const ROOT_LAYOUT_OPTIONS: Record<string, string> = {
  'elk.algorithm': 'layered',
  'elk.direction': 'RIGHT',
  'elk.hierarchyHandling': 'INCLUDE_CHILDREN',
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

// sanitizeId は識別子を `[A-Za-z0-9_]` 範囲へ正規化する。Go 側 `internal/elk`
// の `sanitizeID` と同一規則にして、クロスチェックでの構造一致を保つ。
// グループ名は任意文字列（空白・日本語可）を取り得るため groupNode の id 化に
// 使う。テーブル名は文法上すでに `[A-Za-z0-9_]+` のため実質恒等変換になる。
export function sanitizeId(name: string): string {
  let out = ''
  for (const ch of name) {
    out += /[A-Za-z0-9_]/.test(ch) ? ch : '_'
  }
  return out
}

// buildElkInput はスキーマから ELK 入力グラフ（root ノード）を構築する。
//
// primary グループは Schema.Groups の登場順で groupNode（children を持つ親
// ノード）として並べ、所属テーブルを Schema.Tables の登場順で格納する。primary
// 所属テーブルが 0 件のグループは groupNode を出力しない（Go `internal/elk`
// と同方針）。グループ未指定テーブルは root.children 直下に置く。
export function buildElkInput(schema: Schema): ElkNode {
  const children: ElkNode[] = []

  for (const name of schema.Groups) {
    const members = schema.Tables.filter((t) => primaryGroup(t) === name).map(
      buildTableNode,
    )
    if (members.length === 0) continue
    children.push({ id: sanitizeId(name), children: members })
  }

  for (const t of schema.Tables) {
    if (primaryGroup(t) !== null) continue
    children.push(buildTableNode(t))
  }

  return {
    id: 'root',
    layoutOptions: ROOT_LAYOUT_OPTIONS,
    children,
    edges: buildEdges(schema),
  }
}

// buildTableNode は Table 1 件を elkjs 互換のノードへ変換する。
function buildTableNode(t: Schema['Tables'][number]): ElkNode {
  return { id: sanitizeId(t.Name), width: NODE_WIDTH, height: NODE_HEIGHT }
}

// buildEdges は全テーブルを走査して親 → 子方向の FK エッジ列を生成する。
// WithoutErd カラム由来のエッジは除外（要件 1.8）。ID にカラム名を含めることで
// 同一親子間の複数 FK でも衝突しない（要件 4.3）。
function buildEdges(schema: Schema): ElkExtendedEdge[] {
  const edges: ElkExtendedEdge[] = []
  for (const t of schema.Tables) {
    for (const c of t.Columns) {
      if (c.WithoutErd) continue
      if (c.FK === null) continue
      edges.push({
        id: `fk_${sanitizeId(t.Name)}_${sanitizeId(c.Name)}_${sanitizeId(c.FK.TargetTable)}`,
        sources: [sanitizeId(c.FK.TargetTable)],
        targets: [sanitizeId(t.Name)],
      })
    }
  }
  return edges
}

// extractPositions は ELK レイアウト結果から各テーブルの絶対 (x, y) を抽出する。
//
// groupNode（children を持つノード）配下のテーブル座標は親グループからの相対
// 座標で返るため、親のオフセットを積算して絶対座標へ変換する。テーブルノード
// （children を持たないノード）を Layout のエントリとして採用する。
export function extractPositions(result: ElkNode): Layout {
  const layout: Layout = {}

  const walk = (nodes: readonly ElkNode[] | undefined, ox: number, oy: number): void => {
    for (const node of nodes ?? []) {
      if (typeof node.x !== 'number' || typeof node.y !== 'number') {
        // ELK が座標を返さなかった場合は Fail Fast。0,0 でサイレントに隠蔽しない。
        throw new Error(`ELK layout did not return coordinates for node "${node.id}"`)
      }
      if (node.children && node.children.length > 0) {
        walk(node.children, ox + node.x, oy + node.y)
      } else {
        layout[node.id] = { x: ox + node.x, y: oy + node.y }
      }
    }
  }

  walk(result.children, 0, 0)
  return layout
}
