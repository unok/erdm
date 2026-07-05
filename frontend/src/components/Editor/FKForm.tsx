// FKForm: 単一カラムに紐づく外部キー（FK）の編集 UI（タスク 7.6 / 要件 7.3）。
//
// 役割:
//   - 参照先テーブルをドロップダウンで選択する（同スキーマ内のテーブル名一覧）。
//   - 多重度（cardinality）の source / destination をテキスト入力で編集する。
//
// 設計判断:
//   - TargetTable は `<select>` で選択肢から選ばせ、入力ミスを構造的に排除する。
//     現在値が `availableTables` に含まれない（例: 削除済みテーブルへの参照）
//     場合は、それを当該値の唯一の選択肢として併置し、誤ってドロップダウンが
//     0 件選択になる事故を避ける。
//   - cardinality は自由文字列（`0..*` / `1` 等）。固定リスト化はスコープ外。
//   - 入力検証は最小限（必須=空文字列の警告のみ HTML レベル）に留め、整合性
//     検証は保存時の Go 側 Parser に委譲する（design.md §C11、tasks.md 7.6）。

import type { JSX } from 'react'
import type { FK } from '../../model'

export interface FKFormProps {
  fk: FK
  availableTables: string[]
  onChange: (fk: FK) => void
}

export function FKForm({ fk, availableTables, onChange }: FKFormProps): JSX.Element {
  const includesCurrent = availableTables.includes(fk.TargetTable)

  return (
    <div style={{ marginTop: '8px', paddingLeft: '8px', borderLeft: '2px solid #888' }}>
      <div style={{ marginBottom: '4px' }}>
        <label style={{ display: 'block', fontSize: '12px' }}>Target table</label>
        <select
          value={fk.TargetTable}
          onChange={(e) => onChange({ ...fk, TargetTable: e.target.value })}
          style={{ width: '100%' }}
        >
          {fk.TargetTable === '' ? (
            <option value="" disabled>
              Select a table...
            </option>
          ) : null}
          {!includesCurrent && fk.TargetTable !== '' ? (
            <option value={fk.TargetTable}>{fk.TargetTable} (unknown)</option>
          ) : null}
          {availableTables.map((name) => (
            <option key={name} value={name}>
              {name}
            </option>
          ))}
        </select>
      </div>
      <div style={{ display: 'flex', gap: '4px', marginBottom: '4px' }}>
        <div style={{ flex: 1 }}>
          <label style={{ display: 'block', fontSize: '12px' }}>Cardinality (source)</label>
          <input
            type="text"
            value={fk.CardinalitySource}
            onChange={(e) => onChange({ ...fk, CardinalitySource: e.target.value })}
            style={{ width: '100%' }}
            placeholder="e.g. 1"
          />
        </div>
        <div style={{ flex: 1 }}>
          <label style={{ display: 'block', fontSize: '12px' }}>Cardinality (destination)</label>
          <input
            type="text"
            value={fk.CardinalityDestination}
            onChange={(e) => onChange({ ...fk, CardinalityDestination: e.target.value })}
            style={{ width: '100%' }}
            placeholder="e.g. 0..*"
          />
        </div>
      </div>
    </div>
  )
}
