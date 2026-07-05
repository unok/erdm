// テーブルを「ヘッダ＋カラム行」で描く React Flow カスタムノード（#28）。
//
// 表示規約は DOT レンダラ（internal/dot）に合わせる:
//   - ヘッダ: 論理名があれば `論理名 / 物理名`、無ければ物理名。
//   - カラム行: `名前  型  (制約)`。PK は鍵アイコン、制約は NN / U / UNN、
//     FK は FK バッジで示す。
//   - WithoutErd カラムは ERD 非表示なので行に出さない（DOT と同方針）。
//
// エッジは従来どおりテーブル単位（左=target / 右=source ハンドル）。カラム行
// 単位へのアンカーは後続対応（#28 の残作業）。

import type { JSX } from 'react'
import { Handle, Position, type NodeProps } from 'reactflow'
import { type Column, type Table, secondaryGroups } from '../../model'

export interface TableNodeData {
  table: Table
}

// カラム行のハンドル id。子側 FK 列は target（左）、親側アンカー列は source（右）。
// source / target で型が異なるため接頭辞を分けて衝突を避ける。
export const columnTargetHandleId = (columnName: string): string => `t:${columnName}`
export const columnSourceHandleId = (columnName: string): string => `s:${columnName}`

// parentAnchorColumn は親テーブル側でエッジを出す列名を返す。FK モデルは参照先
// 列を持たないため、慣習に従い可視 PK の先頭列を採用する。PK が無ければ可視列の
// 先頭、可視列が無ければ null（アンカー不能）。
export function parentAnchorColumn(t: Table): string | null {
  const visible = t.Columns.filter((c) => !c.WithoutErd)
  const pk = visible.find((c) => c.IsPrimaryKey)
  if (pk !== undefined) return pk.Name
  return visible.length > 0 ? visible[0]!.Name : null
}

const HANDLE_STYLE = { width: 7, height: 7, background: '#7a8aa0', border: '1px solid #fff' }

// columnBadges は PK 以外の制約バッジ（NN / U / UNN / FK）を DOT と同じ規則で返す。
// PK はアイコンで別途示すためここには含めない。
export function columnBadges(c: Column): string[] {
  const badges: string[] = []
  if (!c.AllowNull && c.IsUnique) {
    badges.push('UNN')
  } else if (!c.AllowNull) {
    badges.push('NN')
  } else if (c.IsUnique) {
    badges.push('U')
  }
  if (c.FK !== null) {
    badges.push('FK')
  }
  return badges
}

function columnLabel(c: Column): string {
  return c.LogicalName !== '' ? `${c.LogicalName} / ${c.Name}` : c.Name
}

function tableLabel(t: Table): string {
  return t.LogicalName !== '' ? `${t.LogicalName} / ${t.Name}` : t.Name
}

export function TableNode({ data }: NodeProps<TableNodeData>): JSX.Element {
  const { table } = data
  const columns = table.Columns.filter((c) => !c.WithoutErd)

  return (
    <div
      style={{
        border: '1px solid #555',
        borderRadius: 6,
        background: '#fff',
        fontSize: 12,
        color: '#222',
        minWidth: 160,
      }}
    >
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 6,
          padding: '4px 8px',
          fontWeight: 700,
          background: '#f2f2f2',
          borderTopLeftRadius: 6,
          borderTopRightRadius: 6,
          borderBottom: '1px solid #ccc',
          whiteSpace: 'nowrap',
        }}
      >
        <span style={{ flex: 1 }}>{tableLabel(table)}</span>
        {/* secondary グループはレイアウトに影響しない（primary のみ枠になる）ため、
            所属をノード上のバッジで示す。 */}
        {secondaryGroups(table).map((g) => (
          <span
            key={g}
            title={`group: ${g}`}
            style={{
              fontSize: 10,
              fontWeight: 600,
              color: '#3a5',
              background: '#e6f4ea',
              border: '1px solid #bfe3ca',
              borderRadius: 3,
              padding: '0 4px',
            }}
          >
            {g}
          </span>
        ))}
      </div>
      {/* 可視カラムが無い縮退テーブルでもエッジが接続できるようフォールバック
          ハンドルを用意する（通常は各カラム行のハンドルを使う）。 */}
      {columns.length === 0 && (
        <>
          <Handle type="target" position={Position.Left} style={HANDLE_STYLE} />
          <Handle type="source" position={Position.Right} style={HANDLE_STYLE} />
        </>
      )}
      <div>
        {columns.map((c) => {
          const badges = columnBadges(c)
          return (
            <div
              key={c.Name}
              style={{
                position: 'relative',
                display: 'flex',
                alignItems: 'center',
                gap: 6,
                padding: '2px 8px',
                borderTop: '1px solid #eee',
                whiteSpace: 'nowrap',
              }}
            >
              <Handle
                type="target"
                position={Position.Left}
                id={columnTargetHandleId(c.Name)}
                style={HANDLE_STYLE}
              />
              {c.IsPrimaryKey ? (
                <span role="img" aria-label="primary key" style={{ width: 12, textAlign: 'center' }}>
                  🔑
                </span>
              ) : (
                <span aria-hidden="true" style={{ width: 12 }} />
              )}
              <span style={{ flex: 1 }}>{columnLabel(c)}</span>
              <span style={{ color: '#888' }}>{c.Type}</span>
              {badges.map((b) => (
                <span
                  key={b}
                  style={{
                    fontSize: 10,
                    fontWeight: 600,
                    color: '#555',
                    background: '#eee',
                    borderRadius: 3,
                    padding: '0 4px',
                  }}
                >
                  {b}
                </span>
              ))}
              <Handle
                type="source"
                position={Position.Right}
                id={columnSourceHandleId(c.Name)}
                style={HANDLE_STYLE}
              />
            </div>
          )
        })}
      </div>
    </div>
  )
}
