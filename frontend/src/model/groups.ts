// グループ判定のドメインヘルパ。Go 側 `internal/model` の
// `Table.PrimaryGroup()` / `Table.SecondaryGroups()` と同義に保つ。
//
// セマンティクス（design.md §C6 / 要件 2.x）:
//   - Table.Groups の先頭要素が primary グループ。
//   - 2 要素目以降が secondary グループ。
//   - primary グループのみが cluster / groupNode 描画の単位になる。

import type { Schema, Table } from './types'

// primaryGroup は Table の primary グループ名を返す。未所属なら null。
export function primaryGroup(t: Table): string | null {
  return t.Groups.length > 0 ? t.Groups[0]! : null
}

// secondaryGroups は secondary グループ名（Groups の 2 要素目以降）を返す。
export function secondaryGroups(t: Table): string[] {
  return t.Groups.length > 1 ? t.Groups.slice(1) : []
}

// allGroupNames はスキーマに現れる全グループ名を重複なく初出順で返す
// （Schema.Groups を先頭に、テーブルの Groups で発見した未登録分を末尾へ）。
// フィルタ UI の選択肢に用いる。
export function allGroupNames(schema: Schema): string[] {
  const seen = new Set<string>()
  const order: string[] = []
  const add = (name: string): void => {
    if (!seen.has(name)) {
      seen.add(name)
      order.push(name)
    }
  }
  for (const name of schema.Groups) add(name)
  for (const t of schema.Tables) {
    for (const g of t.Groups) add(g)
  }
  return order
}
