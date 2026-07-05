// グループフィルタ: サイドバーで特定グループを選び、キャンバス上で該当グループの
// テーブルを強調（非該当を淡色化）する（#27）。
//
// 選択が空のときはフィルタ無効（全テーブル通常表示）。複数選択はいずれかに属する
// テーブルを強調する（OR）。primary / secondary を問わず、テーブルの所属グループ
// （Table.Groups）で判定する。

import type { JSX } from 'react'

export interface GroupFilterProps {
  // 選択肢となる全グループ名（初出順）。
  groups: string[]
  // 現在強調中のグループ集合。空なら強調なし。
  highlighted: Set<string>
  onChange: (next: Set<string>) => void
}

export function GroupFilter({ groups, highlighted, onChange }: GroupFilterProps): JSX.Element | null {
  if (groups.length === 0) {
    return null
  }

  const toggle = (name: string): void => {
    const next = new Set(highlighted)
    if (next.has(name)) {
      next.delete(name)
    } else {
      next.add(name)
    }
    onChange(next)
  }

  return (
    <section
      aria-label="Group filter"
      style={{ borderTop: '1px solid #ccc', marginTop: '8px', paddingTop: '8px' }}
    >
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <h3 style={{ fontSize: '13px', margin: '0 0 4px' }}>Groups</h3>
        {highlighted.size > 0 && (
          <button
            type="button"
            onClick={() => onChange(new Set())}
            style={{ fontSize: '11px' }}
          >
            Clear
          </button>
        )}
      </div>
      <ul style={{ listStyle: 'none', margin: 0, padding: 0 }}>
        {groups.map((g) => (
          <li key={g}>
            <label style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '13px' }}>
              <input type="checkbox" checked={highlighted.has(g)} onChange={() => toggle(g)} />
              {g}
            </label>
          </li>
        ))}
      </ul>
    </section>
  )
}
