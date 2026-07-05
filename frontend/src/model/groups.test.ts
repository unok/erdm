import { describe, expect, it } from 'vitest'
import { allGroupNames, primaryGroup, secondaryGroups } from './groups'
import type { Schema, Table } from './types'

function table(name: string, groups: string[]): Table {
  return { Name: name, LogicalName: '', Columns: [], PrimaryKeys: [], Indexes: [], Groups: groups }
}

describe('primaryGroup / secondaryGroups', () => {
  it('splits Groups into primary (first) and secondary (rest)', () => {
    const t = table('t', ['core', 'audit', 'ops'])
    expect(primaryGroup(t)).toBe('core')
    expect(secondaryGroups(t)).toEqual(['audit', 'ops'])
  })

  it('returns null / empty for an ungrouped table', () => {
    const t = table('t', [])
    expect(primaryGroup(t)).toBeNull()
    expect(secondaryGroups(t)).toEqual([])
  })
})

describe('allGroupNames', () => {
  it('lists Schema.Groups first, then table-discovered groups, deduped in first-seen order', () => {
    const schema: Schema = {
      Title: 't',
      Groups: ['auth', 'shop'],
      Tables: [
        table('users', ['auth', 'audit']), // audit は Schema.Groups 未登録
        table('orders', ['shop']),
      ],
    }
    expect(allGroupNames(schema)).toEqual(['auth', 'shop', 'audit'])
  })

  it('returns an empty list when there are no groups', () => {
    expect(allGroupNames({ Title: 't', Groups: [], Tables: [table('a', [])] })).toEqual([])
  })
})
