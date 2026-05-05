// 座標マージ: 既存座標（永続化済）と ELK 自動配置結果を統合する純粋関数
// （タスク 7.4 / 要件 6.4 / 6.5、design.md §C11）。
//
// 規約:
//   - 既存座標が存在するテーブルは既存座標を優先（要件 6.4）。
//   - 既存座標が無いテーブルは ELK 自動配置結果を採用（要件 6.5）。
//   - スキーマに含まれないテーブル（古い座標 JSON のエントリ）は捨てる。
//   - `existing` / `computed` のいずれにも座標が無い場合は Fail Fast で例外。
//     これは ELK が 1 ノードでも返さなかった場合にしか発生せず、`computeLayout`
//     側で既に検知される想定。merge 段階でサイレントに 0,0 を埋めない。
//   - 副作用なしの純粋関数（テスト容易性）。

import type { Layout, Schema } from '../model'

export function mergePositions(
  existing: Layout,
  computed: Layout,
  schema: Schema,
): Layout {
  const result: Layout = {}
  for (const table of schema.Tables) {
    const existingPos = existing[table.Name]
    if (existingPos !== undefined) {
      result[table.Name] = existingPos
      continue
    }
    const computedPos = computed[table.Name]
    if (computedPos === undefined) {
      throw new Error(
        `Cannot resolve position for table "${table.Name}": missing in both stored layout and ELK result`,
      )
    }
    result[table.Name] = computedPos
  }
  return result
}
