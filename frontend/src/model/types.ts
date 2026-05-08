// Server JSON ↔ internal TS model types.
//
// Why PascalCase: Go 側 `internal/model` の構造体には `json:"..."` タグが無く、
// `encoding/json` はフィールド名 (PascalCase) をそのまま JSON キーとして書き出す。
// SPA 側でも同名で受け取り、変換層を介さずに型安全に取り扱う（design.md §C11、
// §テンプレートと新モデルのフィールド対応表）。
// 唯一 `Position` のみ、Go 側 `internal/layout/types.go` の `json:"x"` / `json:"y"`
// タグに合わせて小文字キーを採用する。

export interface Schema {
  Title: string
  Tables: Table[]
  Groups: string[]
}

export interface Table {
  Name: string
  LogicalName: string
  Columns: Column[]
  PrimaryKeys: number[]
  Indexes: Index[]
  Groups: string[]
}

export interface Column {
  Name: string
  LogicalName: string
  Type: string
  AllowNull: boolean
  IsUnique: boolean
  IsPrimaryKey: boolean
  Default: string
  Comments: string[]
  WithoutErd: boolean
  FK: FK | null
  IndexRefs: number[]
}

export interface FK {
  TargetTable: string
  CardinalitySource: string
  CardinalityDestination: string
}

export interface Index {
  Name: string
  Columns: string[]
  IsUnique: boolean
}

export interface Position {
  x: number
  y: number
}

export type Layout = Record<string, Position>

// DDL ダイアレクト（`/api/export/ddl?dialect=...` で受理される値）。
export type DdlDialect = 'pg' | 'sqlite3'
