import { describe, expect, it } from 'vitest'
import type { Column } from '../../model'
import { columnBadges } from './TableNode'

function col(over: Partial<Column>): Column {
  return {
    Name: 'c',
    LogicalName: '',
    Type: 'int',
    AllowNull: true,
    IsUnique: false,
    IsPrimaryKey: false,
    Default: '',
    Comments: [],
    WithoutErd: false,
    FK: null,
    IndexRefs: [],
    ...over,
  }
}

describe('columnBadges', () => {
  it('marks NOT NULL columns with NN', () => {
    expect(columnBadges(col({ AllowNull: false }))).toEqual(['NN'])
  })

  it('marks unique columns with U', () => {
    expect(columnBadges(col({ IsUnique: true }))).toEqual(['U'])
  })

  it('collapses NOT NULL + unique into UNN', () => {
    expect(columnBadges(col({ AllowNull: false, IsUnique: true }))).toEqual(['UNN'])
  })

  it('appends FK when the column has a foreign key', () => {
    const fk = col({ AllowNull: false, FK: { TargetTable: 'users', CardinalitySource: '0..*', CardinalityDestination: '1' } })
    expect(columnBadges(fk)).toEqual(['NN', 'FK'])
  })

  it('returns no badges for a plain nullable column', () => {
    expect(columnBadges(col({}))).toEqual([])
  })

  it('does not include PK (shown as an icon, not a badge)', () => {
    expect(columnBadges(col({ IsPrimaryKey: true }))).toEqual([])
  })
})
