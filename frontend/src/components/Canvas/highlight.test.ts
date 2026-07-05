import { describe, expect, it } from 'vitest'
import type { Schema } from '../../model'
import { dimmedTables } from './highlight'

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
