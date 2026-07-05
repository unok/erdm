import { describe, expect, it } from 'vitest'
import type { Schema } from '../../model'
import { dimmedTables, isGroupBoxDimmed } from './highlight'

function table(name: string, groups: string[]): Schema['Tables'][number] {
  return { Name: name, LogicalName: '', Columns: [], PrimaryKeys: [], Indexes: [], Groups: groups }
}

const schema: Schema = {
  Title: 't',
  Groups: ['auth', 'shop'],
  Tables: [
    table('users', ['auth']),
    table('orders', ['shop', 'audit']), // shop=primary, audit=secondary
    table('guests', []),
  ],
}

describe('dimmedTables', () => {
  it('returns empty set (no dimming) when the highlight set is empty', () => {
    expect(dimmedTables(schema, new Set())).toEqual(new Set())
  })

  it('dims tables not belonging to any highlighted group', () => {
    // auth を強調 → users は残り、orders / guests が淡色化。
    expect(dimmedTables(schema, new Set(['auth']))).toEqual(new Set(['orders', 'guests']))
  })

  it('matches secondary group membership too', () => {
    // audit は orders の secondary グループ。強調で orders は残る。
    expect(dimmedTables(schema, new Set(['audit']))).toEqual(new Set(['users', 'guests']))
  })

  it('supports OR across multiple highlighted groups', () => {
    expect(dimmedTables(schema, new Set(['auth', 'shop']))).toEqual(new Set(['guests']))
  })
})

describe('isGroupBoxDimmed', () => {
  it('is never dimmed when the filter is inactive', () => {
    expect(isGroupBoxDimmed(schema, 'shop', new Set())).toBe(false)
  })

  it('dims a primary box only when all its members are dimmed', () => {
    // auth 強調時: shop の唯一の primary メンバー orders は audit 経由でなく shop
    // なので淡色化される → shop 枠も淡色化。
    const dimmed = dimmedTables(schema, new Set(['auth']))
    expect(isGroupBoxDimmed(schema, 'auth', dimmed)).toBe(false)
    expect(isGroupBoxDimmed(schema, 'shop', dimmed)).toBe(true)
  })

  it('keeps a primary box lit when a member is kept via a secondary group', () => {
    // audit（orders の secondary）強調時: orders は残る → その primary グループ
    // shop の枠も残す（テーブル表示と整合）。
    const dimmed = dimmedTables(schema, new Set(['audit']))
    expect(isGroupBoxDimmed(schema, 'shop', dimmed)).toBe(false)
  })
})
