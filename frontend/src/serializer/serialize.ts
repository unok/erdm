// 内部 TS モデルから `.erdm` テキストへ変換するシリアライザ。
//
// 役割: 要件 7.6 のクライアント側保存セマンティクスを実現する主体
// （research.md §4.5.1）。SPA 内で編集中の `Schema` をテキスト化し、
// `PUT /api/schema` へ送信する。サーバはこのバイト列をそのまま `.erdm`
// として保存する（design.md §C4 / §6.2）。
//
// バイト一致の責任: 同一 `Schema` 入力に対して Go 側
// `internal/serializer.Serialize` と完全に同じバイト列を返す（要件 7.10）。
// 関数構成・処理順序・固定書式（フラグ順 `[NN][U][=default][-erd]`、
// `@groups["A", "B"]`、4 スペース / 5 スペース インデント、テーブル間空行 1 行、
// `<src>--<dst> <target>` 形式の FK）はすべて Go 側 `internal/serializer/format.go`
// と同期する。
//
// コメント保持はスコープ外（research.md §3.5）。入力 TS モデルに格納された
// `Column.Comments` は出力するが、`.erdm` 内の独立コメント行（`//` 始まり）
// 等は SPA 側のモデルに到達した時点で失われる。

import type { Column, FK, Index, Schema, Table } from '../model'

// インデント定数（Go 側 `internal/serializer/format.go` と一致）。
const COLUMN_INDENT = '    ' // カラム宣言行: 4 スペース
const COMMENT_INDENT = '     # ' // カラムコメント行: 5 スペース + `# `
const INDEX_INDENT = '    index' // インデックス宣言行の先頭: 4 スペース + `index`

// serialize は `Schema` を `.erdm` テキストへ変換する。
//
// 出力形式:
//   - 1 行目: `# Title: <Schema.Title>\n`
//   - テーブル毎に空行 1 行を前置してテーブルブロックを出力
//   - 末尾改行 1 つ（最後のカラム/インデックス行の改行）
export function serialize(schema: Schema): string {
  const parts: string[] = []
  parts.push(formatTitle(schema.Title))
  for (const table of schema.Tables) {
    parts.push('\n')
    parts.push(formatTable(table))
  }
  return parts.join('')
}

// formatTitle は `# Title: <title>\n` を返す。タイトルが空文字列でも形式は維持する。
function formatTitle(title: string): string {
  return `# Title: ${title}\n`
}

// formatTable は 1 テーブルを「宣言行 + カラム行 + インデックス行」の順で返す。
// テーブル間の空行は呼び出し側 (serialize) の責任で挿入する。
function formatTable(table: Table): string {
  const lines: string[] = []
  lines.push(formatTableHeader(table))
  for (const column of table.Columns) {
    lines.push(formatColumn(column))
  }
  for (const index of table.Indexes) {
    lines.push(formatIndex(index))
  }
  return lines.join('')
}

// formatTableHeader は `<Name>[/<LogicalName>][ @groups[...]]\n` を返す。
function formatTableHeader(table: Table): string {
  let header = table.Name
  if (table.LogicalName !== '') {
    header += `/${formatNameLiteral(table.LogicalName)}`
  }
  if (table.Groups.length > 0) {
    header += ` ${formatGroupsDecl(table.Groups)}`
  }
  header += '\n'
  return header
}

// formatGroupsDecl は `@groups["A", "B", ...]` 形式を返す。要素は二重引用符で
// 囲み、カンマ + 半角スペース 1 個で連結する（要件 2.4）。
function formatGroupsDecl(groups: string[]): string {
  const quoted = groups.map((g) => `"${g}"`).join(', ')
  return `@groups[${quoted}]`
}

// formatColumn は 1 カラムの宣言行と付随コメント行を返す。
//
//   <indent>[+]<name>[/<logical>] [<type>][NN][U][=<default>][-erd][ <fk>]\n
//   <commentIndent><comment>\n  (Comments の各要素について)
function formatColumn(column: Column): string {
  let line = COLUMN_INDENT
  if (column.IsPrimaryKey) {
    line += '+'
  }
  line += column.Name
  if (column.LogicalName !== '') {
    line += `/${formatNameLiteral(column.LogicalName)}`
  }
  line += ` [${column.Type}]`
  line += formatColumnFlags(column)
  if (column.FK !== null) {
    line += ` ${formatRelation(column.FK)}`
  }
  line += '\n'
  for (const comment of column.Comments) {
    line += `${COMMENT_INDENT}${comment}\n`
  }
  return line
}

// formatColumnFlags は固定順 `[NN] → [U] → [=<default>] → [-erd]` で属性を返す。
function formatColumnFlags(column: Column): string {
  let flags = ''
  if (!column.AllowNull) {
    flags += '[NN]'
  }
  if (column.IsUnique) {
    flags += '[U]'
  }
  if (column.Default !== '') {
    flags += `[=${escapeDefaultExpr(column.Default)}]`
  }
  if (column.WithoutErd) {
    flags += '[-erd]'
  }
  return flags
}

// escapeDefaultExpr は `[=...]` の値部分に現れる `]` を `\]` にエスケープする。
//
// パーサ側 (`internal/parser/parser.peg` の `default` 規則 `'\\]' / (![\r\n\]] .)`)
// が `\]` を `]` のエスケープと解釈する仕様に合わせ、Serialize 側では対称に
// `]` を `\]` に再エスケープする。Go 側 `internal/serializer/format.go` の
// `escapeDefaultExpr` と同じ規則（PostgreSQL の `'{}'::integer[]` 等を含む
// default を round-trip させるための前提処理）。
function escapeDefaultExpr(v: string): string {
  return v.replace(/\]/g, '\\]')
}

// formatRelation は FK を `<src>--<dst> <target>` 形式で返す。
// CardinalitySource / CardinalityDestination が空文字列でも形式は維持する。
function formatRelation(fk: FK): string {
  return `${fk.CardinalitySource}--${fk.CardinalityDestination} ${fk.TargetTable}`
}

// formatIndex は 1 インデックスの宣言行を返す。
//
//   <indent>index <Name> (<col1>, <col2>, ...)[ unique]\n
function formatIndex(index: Index): string {
  let line = `${INDEX_INDENT} ${index.Name} (${index.Columns.join(', ')})`
  if (index.IsUnique) {
    line += ' unique'
  }
  line += '\n'
  return line
}

// formatNameLiteral は論理名を `.erdm` の table_name / column_name 規則で表現する。
// PEG 文法に従い、空白・タブ・改行・`/` を含む場合は二重引用符で囲み、
// それ以外は無引用で出力する。
function formatNameLiteral(name: string): string {
  return needsQuoted(name) ? `"${name}"` : name
}

// needsQuoted は table_name / column_name の無引用形が許されないかを返す。
// Go 側 `strings.ContainsAny(name, " \t\r\n/")` と同義。
function needsQuoted(name: string): boolean {
  for (const ch of name) {
    if (ch === ' ' || ch === '\t' || ch === '\r' || ch === '\n' || ch === '/') {
      return true
    }
  }
  return false
}
