// Editor: SPA の右サイドパネル。テーブル/カラム/FK/グループ宣言の編集と
// 「Save」「Discard」「Add table」アクションをまとめて提供する
// （タスク 7.6 / 要件 7.1, 7.2, 7.3, 7.4, 7.6）。
//
// 設計判断:
//   - 入力されたスキーマは props 経由で読み取り、編集は `onChange(newSchema)`
//     で逆伝播する一方向データフロー。状態の保持は呼び出し側 (App.tsx) に集約
//     する（過剰な状態の二重管理を避ける）。
//   - 「選択中のテーブル」はテーブル物理名で識別する。テーブル名が編集された
//     場合は `onSelectedTableNameChange` で呼び出し側に新名を通知し、編集中の
//     UI が消えないようにする。
//   - 派生フィールド（`Table.PrimaryKeys`, `Column.IndexRefs`）の整合は
//     serializer が使わないため意図的に同期しない。保存→再フェッチで Go 側
//     パーサが正しい値を再導出する（design.md §C4）。
//   - 子要素のキーは React の安定性のため index を使用する。並べ替え UI は
//     本バッチではスコープ外。

import type { JSX } from 'react'
import type { Column, Schema, Table } from '../../model'
import { ColumnForm } from './ColumnForm'
import { TableForm } from './TableForm'

export type SaveStatus =
  | { kind: 'idle' }
  | { kind: 'saving' }
  | { kind: 'success' }
  | { kind: 'error'; message: string }

export interface EditorProps {
  schema: Schema
  selectedTableName: string | null
  onChange: (schema: Schema) => void
  onSelectedTableNameChange: (name: string | null) => void
  onSave: () => void
  onDiscard: () => void
  saveStatus: SaveStatus
}

const NEW_COLUMN_TEMPLATE: Column = {
  Name: 'new_column',
  LogicalName: '',
  Type: 'integer',
  AllowNull: true,
  IsUnique: false,
  IsPrimaryKey: false,
  Default: '',
  Comments: [],
  WithoutErd: false,
  FK: null,
  IndexRefs: [],
}

export function Editor(props: EditorProps): JSX.Element {
  const {
    schema,
    selectedTableName,
    onChange,
    onSelectedTableNameChange,
    onSave,
    onDiscard,
    saveStatus,
  } = props

  const selectedTable =
    selectedTableName === null
      ? null
      : (schema.Tables.find((t) => t.Name === selectedTableName) ?? null)

  const updateTable = (next: Table): void => {
    if (selectedTable === null) return
    const oldName = selectedTable.Name
    const tables = schema.Tables.map((t) => (t.Name === oldName ? next : t))
    onChange({ ...schema, Tables: tables })
    if (next.Name !== oldName) {
      onSelectedTableNameChange(next.Name)
    }
  }

  const deleteTable = (): void => {
    if (selectedTable === null) return
    const tables = schema.Tables.filter((t) => t.Name !== selectedTable.Name)
    onChange({ ...schema, Tables: tables })
    onSelectedTableNameChange(null)
  }

  const addTable = (): void => {
    const name = uniqueName('new_table', schema.Tables.map((t) => t.Name))
    const newTable: Table = {
      Name: name,
      LogicalName: '',
      Columns: [],
      PrimaryKeys: [],
      Indexes: [],
      Groups: [],
    }
    onChange({ ...schema, Tables: [...schema.Tables, newTable] })
    onSelectedTableNameChange(name)
  }

  const addColumn = (): void => {
    if (selectedTable === null) return
    const name = uniqueName(
      NEW_COLUMN_TEMPLATE.Name,
      selectedTable.Columns.map((c) => c.Name),
    )
    const newColumn: Column = { ...NEW_COLUMN_TEMPLATE, Name: name }
    updateTable({ ...selectedTable, Columns: [...selectedTable.Columns, newColumn] })
  }

  const updateColumn = (index: number, next: Column): void => {
    if (selectedTable === null) return
    const columns = selectedTable.Columns.map((c, i) => (i === index ? next : c))
    updateTable({ ...selectedTable, Columns: columns })
  }

  const deleteColumn = (index: number): void => {
    if (selectedTable === null) return
    const columns = selectedTable.Columns.filter((_, i) => i !== index)
    updateTable({ ...selectedTable, Columns: columns })
  }

  const availableTables = schema.Tables.map((t) => t.Name)
  const isSaving = saveStatus.kind === 'saving'

  return (
    <aside
      style={{
        width: '400px',
        height: '100vh',
        overflowY: 'auto',
        borderLeft: '1px solid #ccc',
        padding: '12px',
        boxSizing: 'border-box',
        background: '#fff',
        fontSize: '14px',
        flexShrink: 0,
      }}
    >
      <div style={{ marginBottom: '8px', display: 'flex', gap: '4px' }}>
        <button type="button" onClick={onSave} disabled={isSaving}>
          {isSaving ? 'Saving...' : 'Save'}
        </button>
        <button type="button" onClick={onDiscard} disabled={isSaving}>
          Discard
        </button>
        <button type="button" onClick={addTable} disabled={isSaving}>
          Add table
        </button>
      </div>
      <SaveStatusBanner status={saveStatus} />
      {selectedTable === null ? (
        <p style={{ color: '#666' }}>
          Click a node on the canvas to edit, or press "Add table" to create a new table.
        </p>
      ) : (
        <div>
          <TableForm table={selectedTable} onChange={updateTable} onDelete={deleteTable} />
          <h3 style={{ fontSize: '13px', margin: '8px 0 4px' }}>Columns</h3>
          {selectedTable.Columns.map((c, i) => (
            <ColumnForm
              // 並べ替え UI なしのため index キーで安定（同番台の差し替えは prop 更新で反映）。
              key={i}
              column={c}
              availableTables={availableTables}
              onChange={(next) => updateColumn(i, next)}
              onDelete={() => deleteColumn(i)}
            />
          ))}
          <button type="button" onClick={addColumn}>
            Add column
          </button>
        </div>
      )}
    </aside>
  )
}

function SaveStatusBanner({ status }: { status: SaveStatus }): JSX.Element | null {
  if (status.kind === 'success') {
    return <p style={{ color: 'green', margin: '4px 0' }}>Saved.</p>
  }
  if (status.kind === 'error') {
    return (
      <p style={{ color: '#a00', margin: '4px 0' }} role="alert">
        Error: {status.message}
      </p>
    )
  }
  return null
}

// uniqueName は `existing` に含まれない名前を `<base>` / `<base>_1` / `<base>_2` ... の
// 順で探して返す。
function uniqueName(base: string, existing: string[]): string {
  const set = new Set(existing)
  if (!set.has(base)) return base
  let i = 1
  while (set.has(`${base}_${i}`)) {
    i += 1
  }
  return `${base}_${i}`
}
