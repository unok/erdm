// Canvas コンポーネントの単体テスト（タスク 7.8、要件 5.10 / 5.11 / 6.1）。
//
// React Flow の DOM 描画は jsdom 上でレイアウト計算を伴い扱いづらいため、
// `reactflow` をモジュール単位でモックし、ノード／エッジ／ハンドラ参照を
// テスト側で直接観測する。これによって以下を検証する:
//   - schema を渡すとテーブル数 / FK 数だけ Node / Edge が構築される
//   - WithoutErd カラム由来の FK は Edge から除外される
//   - onNodeClick が ReactFlow の onNodeClick から正しく呼ばれる
//   - onNodeDragStop が起きると debounce 経由で putLayout が呼ばれる
//
// Requirements: 5.10, 5.11, 6.1

import { render, screen } from '@testing-library/react'
import { useState } from 'react'
import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  vi,
  type Mock,
} from 'vitest'
import { putLayout } from '../../api'
import type { Layout, Schema } from '../../model'
import { Canvas } from './Canvas'

// reactflow を最小限モックする。受け取った props を data-* 属性で晒し、
// onNodeClick / onNodeDragStop をボタン経由で発火できるテスト用ダブル。
vi.mock('reactflow', () => {
  // Node[] / Edge[] を簡易な JSON 文字列として data 属性へ載せる。
  const ReactFlow = (props: {
    nodes: Array<{ id: string }>
    edges: Array<{ id: string }>
    onNodeClick?: (e: unknown, node: { id: string }) => void
    onNodeDragStop?: (e: unknown, node: { id: string }, all: Array<{ id: string; position: { x: number; y: number } }>) => void
  }) => {
    const triggerClick = (): void => {
      const first = props.nodes[0]
      if (first === undefined) return
      props.onNodeClick?.({}, first)
    }
    const triggerDragStop = (): void => {
      const all = props.nodes.map((n) => ({ ...n, position: { x: 1, y: 2 } }))
      const first = all[0]
      if (first === undefined) return
      props.onNodeDragStop?.({}, first, all)
    }
    return (
      <div data-testid="rf-mock">
        <div data-testid="rf-nodes">{JSON.stringify(props.nodes)}</div>
        <div data-testid="rf-edges">{JSON.stringify(props.edges)}</div>
        <button type="button" data-testid="rf-click" onClick={triggerClick}>
          click
        </button>
        <button type="button" data-testid="rf-dragstop" onClick={triggerDragStop}>
          dragstop
        </button>
      </div>
    )
  }
  const Background = (): null => null
  const Controls = (): null => null
  const MiniMap = (): null => null
  // 実環境同様、初期値のみを尊重して以降の prop 変化は setter 経由でしか
  // 反映しないように useState で「最初の initial だけ採用」するセマンティクス
  // を再現する。これにより Canvas 側の useEffect が setNodes/setEdges を呼ぶ
  // パスがないとノード・エッジが更新されない、本来の振る舞いを検査できる。
  const useNodesState = <T,>(initial: T): [T, (next: T) => void, () => void] => {
    const [state, setState] = useState<T>(initial)
    return [state, setState, () => undefined]
  }
  const useEdgesState = <T,>(initial: T): [T, (next: T) => void, () => void] => {
    const [state, setState] = useState<T>(initial)
    return [state, setState, () => undefined]
  }
  return {
    default: ReactFlow,
    Background,
    Controls,
    MiniMap,
    useNodesState,
    useEdgesState,
  }
})

vi.mock('../../api', () => ({
  putLayout: vi.fn().mockResolvedValue(undefined),
}))

const SCHEMA: Schema = {
  Title: 't',
  Groups: [],
  Tables: [
    {
      Name: 'users',
      LogicalName: '会員',
      Columns: [],
      PrimaryKeys: [],
      Indexes: [],
      Groups: [],
    },
    {
      Name: 'orders',
      LogicalName: '',
      Columns: [
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
        {
          Name: 'hidden_user_id',
          LogicalName: '',
          Type: 'bigint',
          AllowNull: true,
          IsUnique: false,
          IsPrimaryKey: false,
          Default: '',
          Comments: [],
          WithoutErd: true,
          FK: {
            TargetTable: 'users',
            CardinalitySource: '0..*',
            CardinalityDestination: '1',
          },
          IndexRefs: [],
        },
      ],
      PrimaryKeys: [],
      Indexes: [],
      Groups: [],
    },
  ],
}

const LAYOUT: Layout = {
  users: { x: 0, y: 0 },
  orders: { x: 100, y: 100 },
}

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
  vi.clearAllMocks()
})

describe('Canvas', () => {
  it('builds nodes from schema/layout and excludes WithoutErd FKs', () => {
    render(<Canvas schema={SCHEMA} initialLayout={LAYOUT} />)
    const nodes = JSON.parse(screen.getByTestId('rf-nodes').textContent ?? '[]') as Array<{
      id: string
      data: { label: string }
    }>
    expect(nodes.map((n) => n.id)).toEqual(['users', 'orders'])
    // ラベルは LogicalName / Name の両方を含む（"会員 / users"）。
    expect(nodes[0]?.data.label).toContain('users')

    const edges = JSON.parse(screen.getByTestId('rf-edges').textContent ?? '[]') as Array<{
      id: string
      source: string
      target: string
    }>
    expect(edges).toHaveLength(1)
    expect(edges[0]?.source).toBe('users')
    expect(edges[0]?.target).toBe('orders')
  })

  it('invokes onNodeClick with the table name', () => {
    const onNodeClick = vi.fn()
    render(<Canvas schema={SCHEMA} initialLayout={LAYOUT} onNodeClick={onNodeClick} />)
    screen.getByTestId('rf-click').click()
    expect(onNodeClick).toHaveBeenCalledWith('users')
  })

  it('updates nodes/edges when schema or layout props change (Copilot review #5)', () => {
    const SCHEMA_NEXT: Schema = {
      Title: 't',
      Groups: [],
      Tables: [
        {
          Name: 'products',
          LogicalName: '',
          Columns: [],
          PrimaryKeys: [],
          Indexes: [],
          Groups: [],
        },
      ],
    }
    const LAYOUT_NEXT: Layout = { products: { x: 50, y: 50 } }
    const { rerender } = render(<Canvas schema={SCHEMA} initialLayout={LAYOUT} />)
    expect(
      JSON.parse(screen.getByTestId('rf-nodes').textContent ?? '[]')
        .map((n: { id: string }) => n.id),
    ).toEqual(['users', 'orders'])

    rerender(<Canvas schema={SCHEMA_NEXT} initialLayout={LAYOUT_NEXT} />)
    expect(
      JSON.parse(screen.getByTestId('rf-nodes').textContent ?? '[]')
        .map((n: { id: string }) => n.id),
    ).toEqual(['products'])
    expect(
      JSON.parse(screen.getByTestId('rf-edges').textContent ?? '[]'),
    ).toEqual([])
  })

  it('debounces putLayout after onNodeDragStop', () => {
    const putLayoutMock = putLayout as unknown as Mock
    render(<Canvas schema={SCHEMA} initialLayout={LAYOUT} />)
    screen.getByTestId('rf-dragstop').click()
    // 500ms 経過するまでは保存されない（要件 6.1 の debounce）。
    vi.advanceTimersByTime(499)
    expect(putLayoutMock).not.toHaveBeenCalled()
    vi.advanceTimersByTime(2)
    expect(putLayoutMock).toHaveBeenCalledTimes(1)
    const arg = putLayoutMock.mock.calls[0]?.[0] as Layout
    expect(arg.users).toEqual({ x: 1, y: 2 })
  })
})
