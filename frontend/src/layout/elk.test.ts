// elk.ts の純粋関数（buildElkInput の階層構造 / extractPositions の座標平坦化）の
// 単体テスト。elkjs 実行を伴うレイアウトそのものはブラウザ実行のためここでは扱わず、
// グループ入れ子由来の絶対座標算出ロジックを重点的に検証する。

import type { ElkNode } from 'elkjs/lib/elk.bundled.js'
import { describe, expect, it } from 'vitest'
import type { Schema } from '../model'
import { buildElkInput, computeLayout, extractPositions, sanitizeId } from './elk'

const schema: Schema = {
  Title: 't',
  Groups: ['auth'],
  Tables: [
    {
      Name: 'users',
      LogicalName: '',
      Columns: [],
      PrimaryKeys: [],
      Indexes: [],
      Groups: ['auth'],
    },
    {
      Name: 'guests',
      LogicalName: '',
      Columns: [],
      PrimaryKeys: [],
      Indexes: [],
      Groups: [],
    },
  ],
}

describe('sanitizeId', () => {
  it('keeps ASCII identifiers unchanged', () => {
    expect(sanitizeId('users_1')).toBe('users_1')
  })
  it('replaces non-identifier chars with underscore', () => {
    expect(sanitizeId('Order Mgmt')).toBe('Order_Mgmt')
    expect(sanitizeId('売上')).toBe('__')
  })
})

describe('buildElkInput', () => {
  it('nests primary-group members under a group node and keeps ungrouped at root', () => {
    const root = buildElkInput(schema)
    const groupNode = root.children?.find((c) => c.id === 'auth')
    expect(groupNode?.children?.map((m) => m.id)).toEqual(['users'])
    expect(root.children?.some((c) => c.id === 'guests' && !c.children)).toBe(true)
  })
})

describe('extractPositions', () => {
  it('converts group-relative child coordinates to absolute positions', () => {
    // auth グループは (100,50) に配置され、その中の users は親からの相対 (10,20)。
    // guests はルート直下 (300,0)。絶対座標は users=(110,70), guests=(300,0)。
    const laidOut: ElkNode = {
      id: 'root',
      children: [
        {
          id: 'auth',
          x: 100,
          y: 50,
          children: [{ id: 'users', x: 10, y: 20, width: 200, height: 100 }],
        },
        { id: 'guests', x: 300, y: 0, width: 200, height: 100 },
      ],
    }
    expect(extractPositions(laidOut)).toEqual({
      users: { x: 110, y: 70 },
      guests: { x: 300, y: 0 },
    })
  })

  it('throws when ELK omits coordinates (fail fast)', () => {
    const bad: ElkNode = { id: 'root', children: [{ id: 'x', width: 200, height: 100 }] }
    expect(() => extractPositions(bad)).toThrow(/did not return coordinates/)
  })
})

describe('computeLayout (real elkjs)', () => {
  it('returns absolute coordinates for grouped and ungrouped tables alike', async () => {
    const layout = await computeLayout(schema)
    // グループ内(users)・グループ外(guests)いずれも絶対座標が得られること。
    expect(typeof layout.users?.x).toBe('number')
    expect(typeof layout.users?.y).toBe('number')
    expect(typeof layout.guests?.x).toBe('number')
    expect(typeof layout.guests?.y).toBe('number')
  })
})
