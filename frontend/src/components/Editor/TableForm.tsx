// TableForm: 単一テーブルのメタデータ編集 UI（タスク 7.6 / 要件 7.1, 7.4）。
//
// 役割:
//   - 物理名・論理名を編集する。
//   - グループ宣言（`@groups[...]`）をカンマ区切りテキストで編集する。
//     先頭が primary、以降は secondary（design.md §C4 / 要件 2.4）。
//   - テーブル削除ボタンを提供する。
//
// 設計判断:
//   - グループ編集は専用 UI（タグ風）を使わずカンマ区切りで簡素化（tasks.md 7.6
//     の検討）。フォーム内部表現は引用符不要で、保存時に serializer が
//     `@groups["A", "B"]` 形式へ変換する。
//   - 入力検証は最小限。重複・空文字列のグループ名はパースで検出される。

import type { JSX } from 'react'
import type { Table } from '../../model'

export interface TableFormProps {
  table: Table
  onChange: (table: Table) => void
  onDelete: () => void
}

export function TableForm({ table, onChange, onDelete }: TableFormProps): JSX.Element {
  const handleGroupsChange = (value: string): void => {
    const next = value
      .split(',')
      .map((s) => s.trim())
      .filter((s) => s !== '')
    onChange({ ...table, Groups: next })
  }

  return (
    <div
      style={{
        border: '1px solid #888',
        padding: '8px',
        marginBottom: '12px',
        borderRadius: '4px',
        background: '#f4f4f4',
      }}
    >
      <div style={{ marginBottom: '4px' }}>
        <label style={{ display: 'block', fontSize: '12px' }}>Table name</label>
        <input
          type="text"
          value={table.Name}
          onChange={(e) => onChange({ ...table, Name: e.target.value })}
          style={{ width: '100%' }}
        />
      </div>
      <div style={{ marginBottom: '4px' }}>
        <label style={{ display: 'block', fontSize: '12px' }}>Logical name</label>
        <input
          type="text"
          value={table.LogicalName}
          onChange={(e) => onChange({ ...table, LogicalName: e.target.value })}
          style={{ width: '100%' }}
        />
      </div>
      <div style={{ marginBottom: '4px' }}>
        <label style={{ display: 'block', fontSize: '12px' }}>
          Groups (comma-separated; first = primary)
        </label>
        <input
          type="text"
          value={table.Groups.join(', ')}
          onChange={(e) => handleGroupsChange(e.target.value)}
          style={{ width: '100%' }}
          placeholder="e.g. orders, payments"
        />
      </div>
      <div style={{ textAlign: 'right' }}>
        <button type="button" onClick={onDelete} style={{ color: '#a00' }}>
          Delete table
        </button>
      </div>
    </div>
  )
}
