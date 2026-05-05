// ColumnForm: 単一カラムの編集 UI（タスク 7.6 / 要件 7.2）。
//
// 役割:
//   - 物理名・論理名・型・主キー・NOT NULL・UNIQUE・Default・コメント・ERD 非表示
//     の各属性を編集する。
//   - FK の追加・編集・削除を行う（FKForm 委譲）。
//   - カラム削除ボタンを提供する。
//
// 設計判断:
//   - フラグ系は `checkbox`、文字列系は `<input type="text">` または
//     `<textarea>`（Comments は複数行）。
//   - `AllowNull` は内部表現のままだと意味が裏返る（true = NULL 許容）ため、
//     UI ラベルは「NOT NULL」、チェック ON で `AllowNull = false` を意味する。
//   - Comments は改行区切りで配列化する。空文字列入力は空配列に正規化し、
//     serialize 時の余分なコメント行を生まないようにする。
//   - FK の追加は空文字列の TargetTable で初期化する（ユーザに必ず選択させる）。
//   - イミュータブル更新で `onChange` に毎回新しい Column を渡す（React の
//     再レンダリング検出を確実にする）。

import type { JSX } from 'react'
import type { Column, FK } from '../../model'
import { FKForm } from './FKForm'

export interface ColumnFormProps {
  column: Column
  availableTables: string[]
  onChange: (column: Column) => void
  onDelete: () => void
}

const EMPTY_FK: FK = { TargetTable: '', CardinalitySource: '', CardinalityDestination: '' }

export function ColumnForm({
  column,
  availableTables,
  onChange,
  onDelete,
}: ColumnFormProps): JSX.Element {
  const handleCommentsChange = (value: string): void => {
    const next = value === '' ? [] : value.split('\n')
    onChange({ ...column, Comments: next })
  }

  return (
    <div
      style={{
        border: '1px solid #ccc',
        padding: '8px',
        marginBottom: '8px',
        borderRadius: '4px',
      }}
    >
      <div style={{ display: 'flex', gap: '4px', marginBottom: '4px' }}>
        <div style={{ flex: 1 }}>
          <label style={{ display: 'block', fontSize: '12px' }}>Name</label>
          <input
            type="text"
            value={column.Name}
            onChange={(e) => onChange({ ...column, Name: e.target.value })}
            style={{ width: '100%' }}
          />
        </div>
        <div style={{ flex: 1 }}>
          <label style={{ display: 'block', fontSize: '12px' }}>Logical name</label>
          <input
            type="text"
            value={column.LogicalName}
            onChange={(e) => onChange({ ...column, LogicalName: e.target.value })}
            style={{ width: '100%' }}
          />
        </div>
      </div>
      <div style={{ marginBottom: '4px' }}>
        <label style={{ display: 'block', fontSize: '12px' }}>Type</label>
        <input
          type="text"
          value={column.Type}
          onChange={(e) => onChange({ ...column, Type: e.target.value })}
          style={{ width: '100%' }}
          placeholder="e.g. integer, varchar(64)"
        />
      </div>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px', marginBottom: '4px' }}>
        <label style={{ fontSize: '12px' }}>
          <input
            type="checkbox"
            checked={column.IsPrimaryKey}
            onChange={(e) => onChange({ ...column, IsPrimaryKey: e.target.checked })}
          />
          PK
        </label>
        <label style={{ fontSize: '12px' }}>
          <input
            type="checkbox"
            checked={!column.AllowNull}
            onChange={(e) => onChange({ ...column, AllowNull: !e.target.checked })}
          />
          NOT NULL
        </label>
        <label style={{ fontSize: '12px' }}>
          <input
            type="checkbox"
            checked={column.IsUnique}
            onChange={(e) => onChange({ ...column, IsUnique: e.target.checked })}
          />
          UNIQUE
        </label>
        <label style={{ fontSize: '12px' }}>
          <input
            type="checkbox"
            checked={column.WithoutErd}
            onChange={(e) => onChange({ ...column, WithoutErd: e.target.checked })}
          />
          Hide from ERD
        </label>
      </div>
      <div style={{ marginBottom: '4px' }}>
        <label style={{ display: 'block', fontSize: '12px' }}>Default</label>
        <input
          type="text"
          value={column.Default}
          onChange={(e) => onChange({ ...column, Default: e.target.value })}
          style={{ width: '100%' }}
        />
      </div>
      <div style={{ marginBottom: '4px' }}>
        <label style={{ display: 'block', fontSize: '12px' }}>Comments (one per line)</label>
        <textarea
          value={column.Comments.join('\n')}
          onChange={(e) => handleCommentsChange(e.target.value)}
          style={{ width: '100%', minHeight: '40px' }}
        />
      </div>
      <div style={{ marginBottom: '4px' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
          <strong style={{ fontSize: '12px' }}>FK</strong>
          {column.FK === null ? (
            <button type="button" onClick={() => onChange({ ...column, FK: { ...EMPTY_FK } })}>
              Add FK
            </button>
          ) : (
            <button type="button" onClick={() => onChange({ ...column, FK: null })}>
              Remove FK
            </button>
          )}
        </div>
        {column.FK !== null ? (
          <FKForm
            fk={column.FK}
            availableTables={availableTables}
            onChange={(fk) => onChange({ ...column, FK: fk })}
          />
        ) : null}
      </div>
      <div style={{ textAlign: 'right' }}>
        <button type="button" onClick={onDelete} style={{ color: '#a00' }}>
          Delete column
        </button>
      </div>
    </div>
  )
}
