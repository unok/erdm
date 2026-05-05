// Storage draft helpers の単体テスト（タスク 7.8、要件 7.5）。
//
// 検証対象: localStorage を介した下書きの読み込み・保存・削除・存在確認。
// jsdom 標準の localStorage 実装を使い、各テストの先頭で `localStorage.clear()`
// により分離する。
//
// Requirements: 7.5

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Schema } from '../model'
import { clearDraft, hasDraft, loadDraft, saveDraft } from './draft'

const SAMPLE_SCHEMA: Schema = {
  Title: 'sample',
  Tables: [
    {
      Name: 'users',
      LogicalName: '',
      Columns: [
        {
          Name: 'id',
          LogicalName: '',
          Type: 'bigserial',
          AllowNull: false,
          IsUnique: true,
          IsPrimaryKey: true,
          Default: '',
          Comments: [],
          WithoutErd: false,
          FK: null,
          IndexRefs: [],
        },
      ],
      PrimaryKeys: [0],
      Indexes: [],
      Groups: [],
    },
  ],
  Groups: [],
}

beforeEach(() => {
  window.localStorage.clear()
})

afterEach(() => {
  window.localStorage.clear()
  vi.restoreAllMocks()
})

describe('loadDraft', () => {
  it('returns null when no draft is stored', () => {
    expect(loadDraft()).toBeNull()
  })

  it('returns the saved schema after saveDraft', () => {
    saveDraft(SAMPLE_SCHEMA)
    expect(loadDraft()).toEqual(SAMPLE_SCHEMA)
  })

  it('returns null when the stored draft is corrupted JSON', () => {
    window.localStorage.setItem('erdm-draft', '{not valid json')
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    expect(loadDraft()).toBeNull()
    expect(warn).toHaveBeenCalled()
  })
})

describe('saveDraft / clearDraft / hasDraft', () => {
  it('hasDraft is false initially and true after save', () => {
    expect(hasDraft()).toBe(false)
    saveDraft(SAMPLE_SCHEMA)
    expect(hasDraft()).toBe(true)
  })

  it('clearDraft removes the saved draft', () => {
    saveDraft(SAMPLE_SCHEMA)
    expect(hasDraft()).toBe(true)
    clearDraft()
    expect(hasDraft()).toBe(false)
    expect(loadDraft()).toBeNull()
  })

  it('saveDraft swallows quota errors and keeps the editor functional', () => {
    const original = window.localStorage.setItem
    const stub = vi
      .spyOn(Storage.prototype, 'setItem')
      .mockImplementation(() => {
        throw new Error('quota exceeded')
      })
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    expect(() => saveDraft(SAMPLE_SCHEMA)).not.toThrow()
    expect(warn).toHaveBeenCalled()
    stub.mockRestore()
    // sanity: original setItem still callable (jsdom restored)
    expect(typeof original).toBe('function')
  })
})
