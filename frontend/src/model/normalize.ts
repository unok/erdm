// Go の encoding/json が nil スライスを JSON null と書き出す場合や、古い下書き JSON
// により配列フィールドが null になる場合に、UI・シリアライザが安全に動くよう
// 正規化する（TableForm / ColumnForm の .join など）。

import type { Column, Index, Schema, Table } from './types'

function stringArray(v: unknown): string[] {
  return Array.isArray(v) ? (v as string[]) : []
}

function intArray(v: unknown): number[] {
  return Array.isArray(v) ? (v as number[]) : []
}

function normalizeIndex(i: Index): Index {
  return {
    ...i,
    Columns: stringArray(i.Columns),
  }
}

function normalizeColumn(c: Column): Column {
  return {
    ...c,
    Comments: stringArray(c.Comments),
    IndexRefs: intArray(c.IndexRefs),
  }
}

function normalizeTable(t: Table): Table {
  const cols = Array.isArray(t.Columns) ? t.Columns : []
  return {
    ...t,
    Columns: cols.map(normalizeColumn),
    PrimaryKeys: intArray(t.PrimaryKeys),
    Indexes: Array.isArray(t.Indexes) ? t.Indexes.map(normalizeIndex) : [],
    Groups: stringArray(t.Groups),
  }
}

/** API / localStorage から受け取ったスキーマを内部不変条件に合わせて整形する。 */
export function normalizeSchema(s: Schema): Schema {
  const tables = Array.isArray(s.Tables) ? s.Tables : []
  return {
    ...s,
    Tables: tables.map(normalizeTable),
    Groups: stringArray(s.Groups),
  }
}
