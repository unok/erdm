// Client-side serializer の単体テスト（タスク 7.8、要件 7.6 / 7.10）。
//
// `internal/serializer/serializer_test.go` と同等の正規化規則カバレッジを
// 提供する。クロスチェック（Go 出力との完全一致）は cross-check.test.ts で扱う。
//
// Requirements: 7.6, 7.10

import { describe, expect, it } from 'vitest'
import type { Column, Schema } from '../model'
import { serialize } from './serialize'

function makePkColumn(name: string): Column {
  return {
    Name: name,
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
  }
}

describe('serialize', () => {
  it('writes only the title for an empty schema', () => {
    const s: Schema = { Title: 'x', Tables: [], Groups: [] }
    expect(serialize(s)).toBe('# Title: x\n')
  })

  it('writes a single table with a single PK column', () => {
    const s: Schema = {
      Title: 't',
      Groups: [],
      Tables: [
        {
          Name: 'users',
          LogicalName: '',
          Columns: [makePkColumn('id')],
          PrimaryKeys: [0],
          Indexes: [],
          Groups: [],
        },
      ],
    }
    const want = '# Title: t\n' + '\n' + 'users\n' + '    +id [bigserial][NN][U]\n'
    expect(serialize(s)).toBe(want)
  })

  it('quotes logical names containing whitespace and slashes', () => {
    const s: Schema = {
      Title: 't',
      Groups: [],
      Tables: [
        {
          Name: 'users',
          LogicalName: 'site user master',
          Columns: [{ ...makePkColumn('id'), LogicalName: 'member id' }],
          PrimaryKeys: [0],
          Indexes: [],
          Groups: [],
        },
      ],
    }
    const got = serialize(s)
    expect(got).toContain('users/"site user master"')
    expect(got).toContain('+id/"member id"')
  })

  it('does not quote logical names without whitespace or slashes', () => {
    const s: Schema = {
      Title: 't',
      Groups: [],
      Tables: [
        {
          Name: 'users',
          LogicalName: '会員',
          Columns: [{ ...makePkColumn('id'), Type: 'bigint', LogicalName: '会員ID' }],
          PrimaryKeys: [0],
          Indexes: [],
          Groups: [],
        },
      ],
    }
    const got = serialize(s)
    expect(got).toContain('users/会員\n')
    expect(got).toContain('+id/会員ID ')
  })

  it('emits @groups[...] for primary and secondary groups', () => {
    const s: Schema = {
      Title: 't',
      Groups: ['core', 'audit'],
      Tables: [
        {
          Name: 'users',
          LogicalName: '',
          Columns: [makePkColumn('id')],
          PrimaryKeys: [0],
          Indexes: [],
          Groups: ['core', 'audit'],
        },
      ],
    }
    expect(serialize(s)).toContain(' @groups["core", "audit"]')
  })

  it('orders column flags as [NN][U][=default][-erd]', () => {
    const s: Schema = {
      Title: 't',
      Groups: [],
      Tables: [
        {
          Name: 'users',
          LogicalName: '',
          Columns: [
            {
              Name: 'id',
              LogicalName: '',
              Type: 'int',
              AllowNull: false,
              IsUnique: true,
              IsPrimaryKey: true,
              Default: '0',
              Comments: [],
              WithoutErd: true,
              FK: null,
              IndexRefs: [],
            },
          ],
          PrimaryKeys: [0],
          Indexes: [],
          Groups: [],
        },
      ],
    }
    expect(serialize(s)).toContain('+id [int][NN][U][=0][-erd]')
  })

  it('formats foreign keys as <src>--<dst> <target>', () => {
    const s: Schema = {
      Title: 't',
      Groups: [],
      Tables: [
        {
          Name: 'orders',
          LogicalName: '',
          Columns: [
            makePkColumn('id'),
            {
              Name: 'user_id',
              LogicalName: '',
              Type: 'bigint',
              AllowNull: false,
              IsUnique: false,
              IsPrimaryKey: false,
              Default: '',
              Comments: [],
              WithoutErd: false,
              FK: {
                TargetTable: 'users',
                CardinalitySource: '0..*',
                CardinalityDestination: '1',
              },
              IndexRefs: [],
            },
          ],
          PrimaryKeys: [0],
          Indexes: [],
          Groups: [],
        },
        {
          Name: 'users',
          LogicalName: '',
          Columns: [makePkColumn('id')],
          PrimaryKeys: [0],
          Indexes: [],
          Groups: [],
        },
      ],
    }
    expect(serialize(s)).toContain('    user_id [bigint][NN] 0..*--1 users\n')
  })

  it('emits indexes with optional unique modifier', () => {
    const s: Schema = {
      Title: 't',
      Groups: [],
      Tables: [
        {
          Name: 'users',
          LogicalName: '',
          Columns: [
            makePkColumn('id'),
            {
              Name: 'email',
              LogicalName: '',
              Type: 'text',
              AllowNull: false,
              IsUnique: false,
              IsPrimaryKey: false,
              Default: '',
              Comments: [],
              WithoutErd: false,
              FK: null,
              IndexRefs: [],
            },
          ],
          PrimaryKeys: [0],
          Indexes: [
            { Name: 'idx_email', Columns: ['email'], IsUnique: false },
            { Name: 'idx_email_u', Columns: ['email'], IsUnique: true },
          ],
          Groups: [],
        },
      ],
    }
    const got = serialize(s)
    expect(got).toContain('    index idx_email (email)\n')
    expect(got).toContain('    index idx_email_u (email) unique\n')
  })

  it('separates tables with a blank line', () => {
    const s: Schema = {
      Title: 't',
      Groups: [],
      Tables: [
        {
          Name: 'a',
          LogicalName: '',
          Columns: [makePkColumn('id')],
          PrimaryKeys: [0],
          Indexes: [],
          Groups: [],
        },
        {
          Name: 'b',
          LogicalName: '',
          Columns: [makePkColumn('id')],
          PrimaryKeys: [0],
          Indexes: [],
          Groups: [],
        },
      ],
    }
    const want =
      '# Title: t\n' +
      '\n' +
      'a\n' +
      '    +id [bigserial][NN][U]\n' +
      '\n' +
      'b\n' +
      '    +id [bigserial][NN][U]\n'
    expect(serialize(s)).toBe(want)
  })

  it('emits column comments with 5-space + "# " indent', () => {
    const s: Schema = {
      Title: 't',
      Groups: [],
      Tables: [
        {
          Name: 'users',
          LogicalName: '',
          Columns: [
            {
              Name: 'password',
              LogicalName: '',
              Type: 'varchar(128)',
              AllowNull: false,
              IsUnique: false,
              IsPrimaryKey: false,
              Default: "'********'",
              Comments: ['sha1 でハッシュ化して登録', '二行目'],
              WithoutErd: false,
              FK: null,
              IndexRefs: [],
            },
          ],
          PrimaryKeys: [],
          Indexes: [],
          Groups: [],
        },
      ],
    }
    const got = serialize(s)
    expect(got).toContain('     # sha1 でハッシュ化して登録\n')
    expect(got).toContain('     # 二行目\n')
  })
})
