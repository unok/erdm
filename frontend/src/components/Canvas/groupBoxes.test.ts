import type { Node } from 'reactflow'
import { describe, expect, it } from 'vitest'
import type { Schema } from '../../model'
import { GROUP_BOX_PREFIX, computeGroupBoxes, isGroupBoxId } from './groupBoxes'

function table(name: string, groups: string[]): Schema['Tables'][number] {
  return { Name: name, LogicalName: '', Columns: [], PrimaryKeys: [], Indexes: [], Groups: groups }
}

const schema: Schema = {
  Title: 't',
  Groups: ['auth'],
  Tables: [table('users', ['auth']), table('sessions', ['auth']), table('guests', [])],
}

const tableNodes: Node[] = [
  { id: 'users', position: { x: 0, y: 0 }, width: 150, height: 40, data: {} },
  { id: 'sessions', position: { x: 200, y: 100 }, width: 150, height: 40, data: {} },
  { id: 'guests', position: { x: 500, y: 0 }, width: 150, height: 40, data: {} },
]

describe('isGroupBoxId', () => {
  it('matches the group box prefix only', () => {
    expect(isGroupBoxId(`${GROUP_BOX_PREFIX}auth`)).toBe(true)
    expect(isGroupBoxId('users')).toBe(false)
  })
})

describe('computeGroupBoxes', () => {
  it('emits one non-interactive box wrapping the primary-group members', () => {
    const boxes = computeGroupBoxes(schema, tableNodes)
    expect(boxes.map((b) => b.id)).toEqual(['group:auth'])
    const box = boxes[0]!
    // auth メンバー bbox = (0,0)-(350,140)。余白 24、ラベル高 22。
    expect(box.position).toEqual({ x: -24, y: -46 })
    expect(box.style?.width).toBe(398)
    expect(box.style?.height).toBe(210)
    expect(box.data).toEqual({ label: 'auth' })
    expect(box.draggable).toBe(false)
    expect(box.selectable).toBe(false)
  })

  it('omits groups whose members are not present in the current nodes', () => {
    // auth メンバーのノードが 1 つも無ければ枠を出さない。
    expect(computeGroupBoxes(schema, [tableNodes[2]!])).toEqual([])
  })

  it('returns no boxes for an ungrouped schema', () => {
    const ungrouped: Schema = { Title: 't', Groups: [], Tables: [table('a', []), table('b', [])] }
    const nodes: Node[] = [
      { id: 'a', position: { x: 0, y: 0 }, data: {} },
      { id: 'b', position: { x: 10, y: 0 }, data: {} },
    ]
    expect(computeGroupBoxes(ungrouped, nodes)).toEqual([])
  })

  it('falls back to default node size when width/height are unmeasured', () => {
    const nodes: Node[] = [{ id: 'users', position: { x: 0, y: 0 }, data: {} }]
    const single: Schema = { Title: 't', Groups: ['auth'], Tables: [table('users', ['auth'])] }
    const box = computeGroupBoxes(single, nodes)[0]!
    // 既定 150x40 + 余白。width = 150 + 48 = 198, height = 40 + 48 + 22 = 110。
    expect(box.style?.width).toBe(198)
    expect(box.style?.height).toBe(110)
  })
})
