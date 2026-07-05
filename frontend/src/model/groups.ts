// グループ判定のドメインヘルパ。Go 側 `internal/model` の
// `Table.PrimaryGroup()` / `Table.SecondaryGroups()` と同義に保つ。
//
// セマンティクス（design.md §C6 / 要件 2.x）:
//   - Table.Groups の先頭要素が primary グループ。
//   - 2 要素目以降が secondary グループ。
//   - primary グループのみが cluster / groupNode 描画の単位になる。

import type { Table } from './types'

// primaryGroup は Table の primary グループ名を返す。未所属なら null。
export function primaryGroup(t: Table): string | null {
  return t.Groups.length > 0 ? t.Groups[0]! : null
}

// secondaryGroups は secondary グループ名（Groups の 2 要素目以降）を返す。
export function secondaryGroups(t: Table): string[] {
  return t.Groups.length > 1 ? t.Groups.slice(1) : []
}
