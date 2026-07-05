// グループ強調フィルタの淡色化判定（#27）。
//
// highlight が空ならフィルタ無効（淡色化なし）。それ以外は、選択グループのいずれ
// にも属さないテーブルを淡色化対象とする（primary / secondary を問わず Table.Groups
// で判定、OR）。

import type { Schema } from '../../model'

export function dimmedTables(schema: Schema, highlight: Set<string>): Set<string> {
  const dimmed = new Set<string>()
  if (highlight.size === 0) return dimmed
  for (const t of schema.Tables) {
    const belongs = t.Groups.some((g) => highlight.has(g))
    if (!belongs) dimmed.add(t.Name)
  }
  return dimmed
}
