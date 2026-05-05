// React Flow キャンバス: テーブルノードと FK エッジを描画し、ズーム/パン/ドラッグ
// を提供する（タスク 7.5 / 要件 5.10 / 5.11 / 6.1、design.md §C11）。
//
// 設計判断:
//   - `useNodesState` / `useEdgesState` で React Flow 標準のノード・エッジ状態を
//     管理（research.md §3.3）。Redux 等の外部状態管理は導入しない。
//   - ドラッグ完了時の座標スナップショットは `onNodeDragStop` のコールバック
//     第 3 引数（`allNodes`）から取得する（React Flow 11 系 API）。
//   - 連続更新は 500ms の debounce で抑制（要件 6.1）。debounce はコンポーネント
//     スコープの useRef で実装。タイマー id は unmount で必ずキャンセル。
//   - 標準ノードを使用し、`data.label` にテーブルの論理名 / 物理名を表示する。
//     詳細なカラム表示・カスタムノードはタスク 7.6 で検討する。
//   - エッジ方向は parent → child（要件 1.6 と整合）。`source = FK.TargetTable`
//     （親）、`target = Table.Name`（子）。
//   - `WithoutErd` カラム由来の FK は描画しない（要件 1.8 と整合）。
//   - putLayout 失敗時はコンソール出力のみ（UI の閲覧/操作は継続）。閲覧モード
//     の主目的（描画）が損なわれない方針。

import { useCallback, useEffect, useRef, type JSX } from 'react'
import ReactFlow, {
  Background,
  Controls,
  MiniMap,
  useEdgesState,
  useNodesState,
  type Edge,
  type Node,
  type NodeDragHandler,
  type NodeMouseHandler,
} from 'reactflow'
import 'reactflow/dist/style.css'
import { putLayout } from '../../api'
import type { Layout, Schema } from '../../model'

const SAVE_DEBOUNCE_MS = 500

export interface CanvasProps {
  schema: Schema
  initialLayout: Layout
  // ノードクリックでテーブル名を親に通知する。タスク 7.6 の編集モード（Editor）が
  // 「いま編集対象に選んだテーブル」を確定するために使う。閲覧モードのみで
  // 利用する場合は省略可能（既存の 7.5 互換）。
  onNodeClick?: (tableName: string) => void
}

export function Canvas({ schema, initialLayout, onNodeClick }: CanvasProps): JSX.Element {
  const initialNodes = buildNodes(schema, initialLayout)
  const initialEdges = buildEdges(schema)
  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges)
  const debounceTimerRef = useRef<number | null>(null)

  // schema / initialLayout が更新されたら React Flow の state を同期する。
  // useNodesState は初期値しか拾わないため、Editor 側でテーブル追加/編集して
  // schema が差し替わっても Canvas が古いノード集合のままになってしまう
  // （Copilot レビュー指摘 #5）。propsを単一の正としてsetterで上書きする。
  useEffect(() => {
    setNodes(buildNodes(schema, initialLayout))
    setEdges(buildEdges(schema))
  }, [schema, initialLayout, setNodes, setEdges])

  const scheduleSave = useCallback((latestNodes: Node[]) => {
    if (debounceTimerRef.current !== null) {
      window.clearTimeout(debounceTimerRef.current)
    }
    debounceTimerRef.current = window.setTimeout(() => {
      debounceTimerRef.current = null
      const layout: Layout = {}
      for (const n of latestNodes) {
        layout[n.id] = { x: n.position.x, y: n.position.y }
      }
      void putLayout(layout).catch((err: unknown) => {
        // putLayout は ApiError or fetch 由来の Error を投げる想定。
        // 閲覧/操作を妨げないよう console に出すだけに留める。
        console.error('Failed to save layout:', err)
      })
    }, SAVE_DEBOUNCE_MS)
  }, [])

  const onNodeDragStop: NodeDragHandler = useCallback(
    (_event, _node, allNodes) => {
      scheduleSave(allNodes)
    },
    [scheduleSave],
  )

  const onNodeClickHandler: NodeMouseHandler = useCallback(
    (_event, node) => {
      onNodeClick?.(node.id)
    },
    [onNodeClick],
  )

  useEffect(() => {
    return () => {
      if (debounceTimerRef.current !== null) {
        window.clearTimeout(debounceTimerRef.current)
        debounceTimerRef.current = null
      }
    }
  }, [])

  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      onNodeDragStop={onNodeDragStop}
      onNodeClick={onNodeClickHandler}
      fitView
    >
      <Background />
      <Controls />
      <MiniMap />
    </ReactFlow>
  )
}

function buildNodes(schema: Schema, layout: Layout): Node[] {
  return schema.Tables.map((t) => {
    const pos = layout[t.Name]
    if (pos === undefined) {
      // 上位（App.tsx）で `mergePositions` を通している前提。万一抜けがあれば
      // Fail Fast でバグを早期に表面化させる。
      throw new Error(`Missing position for table "${t.Name}" in canvas input`)
    }
    const label = t.LogicalName !== '' ? `${t.LogicalName} / ${t.Name}` : t.Name
    return {
      id: t.Name,
      type: 'default',
      position: { x: pos.x, y: pos.y },
      data: { label },
    }
  })
}

function buildEdges(schema: Schema): Edge[] {
  const edges: Edge[] = []
  for (const t of schema.Tables) {
    for (const c of t.Columns) {
      if (c.WithoutErd) continue
      if (c.FK === null) continue
      edges.push({
        id: `${c.FK.TargetTable}__${t.Name}__${c.Name}`,
        source: c.FK.TargetTable,
        target: t.Name,
        label: `${c.FK.CardinalityDestination}--${c.FK.CardinalitySource}`,
      })
    }
  }
  return edges
}
