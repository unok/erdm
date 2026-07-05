// グループ強調フィルタの淡色化判定（#27）。
//
// highlight が空ならフィルタ無効（淡色化なし）。それ以外は、選択グループのいずれ
// にも属さないテーブルを淡色化対象とする（primary / secondary を問わず Table.Groups
// で判定、OR）。

import { primaryGroup, type Schema } from '../../model'

export function dimmedTables(schema: Schema, highlight: Set<string>): Set<string> {
  const dimmed = new Set<string>()
  if (highlight.size === 0) return dimmed
  for (const t of schema.Tables) {
    const belongs = t.Groups.some((g) => highlight.has(g))
    if (!belongs) dimmed.add(t.Name)
  }
  return dimmed
}

// isGroupBoxDimmed は primary グループ枠を淡色化すべきか判定する。
// テーブルの表示と整合させるため、その primary グループの可視メンバーが 1 つも
// 無い（＝全員淡色化）ときのみ枠を淡色化する。secondary のみ選択でメンバーが
// 残る場合は枠も残す。dimmed が空（フィルタ無効）なら淡色化しない。
export function isGroupBoxDimmed(schema: Schema, groupName: string, dimmed: Set<string>): boolean {
  if (dimmed.size === 0) return false
  const members = schema.Tables.filter((t) => primaryGroup(t) === groupName)
  return members.length > 0 && members.every((t) => dimmed.has(t.Name))
}
