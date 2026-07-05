// 座標マージ層の単体テスト（タスク 7.4、要件 6.4 / 6.5）。
//
// 検証対象 (`mergePositions`):
//   - 既存座標があるテーブルは既存座標を優先する（要件 6.4）。
//   - 既存座標が無いテーブルは ELK 自動配置結果を採用する（要件 6.5）。
//   - スキーマに含まれないテーブル（古い座標 JSON のエントリ）は捨てられる。
//   - `existing` / `computed` のいずれにも座標が無い場合は Fail Fast で例外。
//
// Requirements: 6.4, 6.5

import { describe, expect, it } from 'vitest'
import type { Layout, Schema } from '../model'
import { mergePositions } from './merge'

function tbl(name: string): Schema['Tables'][number] {
  return {
    Name: name,
    LogicalName: '',
    Columns: [],
    Indexes: [],
    PrimaryKeys: [],
    Groups: [],
  }
}

function schemaWith(...names: string[]): Schema {
  return { Title: 't', Tables: names.map(tbl), Groups: [] }
}

describe('mergePositions', () => {
  it('既存座標を ELK 自動配置より優先する（要件 6.4）', () => {
    const existing: Layout = { users: { x: 100, y: 200 } }
    const computed: Layout = { users: { x: 999, y: 999 } }
    const schema = schemaWith('users')
    expect(mergePositions(existing, computed, schema)).toEqual({
      users: { x: 100, y: 200 },
    })
  })

  it('既存座標が無いテーブルは ELK 自動配置にフォールバックする（要件 6.5）', () => {
    const existing: Layout = {}
    const computed: Layout = { users: { x: 10, y: 20 } }
    const schema = schemaWith('users')
    expect(mergePositions(existing, computed, schema)).toEqual({
      users: { x: 10, y: 20 },
    })
  })

  it('複数テーブルで「既存優先＋新規 ELK 採用」の混合動作を検証する（要件 6.4 / 6.5）', () => {
    const existing: Layout = { users: { x: 1, y: 2 } }
    const computed: Layout = {
      users: { x: 999, y: 999 },
      posts: { x: 50, y: 60 },
    }
    const schema = schemaWith('users', 'posts')
    expect(mergePositions(existing, computed, schema)).toEqual({
      users: { x: 1, y: 2 },
      posts: { x: 50, y: 60 },
    })
  })

  it('スキーマに含まれないテーブルの古い座標は捨てられる', () => {
    const existing: Layout = {
      users: { x: 1, y: 2 },
      orphan: { x: 9, y: 9 },
    }
    const computed: Layout = {}
    const schema = schemaWith('users')
    const merged = mergePositions(existing, computed, schema)
    expect(merged).toEqual({ users: { x: 1, y: 2 } })
    expect(merged).not.toHaveProperty('orphan')
  })

  it('既存にも computed にも無いテーブルは例外で Fail Fast する', () => {
    const existing: Layout = {}
    const computed: Layout = {}
    const schema = schemaWith('lonely')
    expect(() => mergePositions(existing, computed, schema)).toThrow(/lonely/)
  })
})
