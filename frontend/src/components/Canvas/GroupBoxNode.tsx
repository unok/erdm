// primary グループの背景枠を描く React Flow カスタムノード（#27）。
//
// 接続ハンドル（Handle）を持たない純粋な装飾ノード。左上にグループ名を表示し、
// 破線の枠と淡い背景でテーブル群の所属を示す。クリック・ドラッグはノード側の
// draggable/selectable=false（computeGroupBoxes が設定）で無効化される。

import type { JSX } from 'react'
import type { NodeProps } from 'reactflow'

export function GroupBoxNode({ data }: NodeProps<{ label: string }>): JSX.Element {
  return (
    <div
      style={{
        // ラベルの絶対配置がこの枠を基準になるよう positioned ancestor にする
        // （React Flow のラッパ DOM 構造に依存しない自己完結）。
        position: 'relative',
        width: '100%',
        height: '100%',
        boxSizing: 'border-box',
        border: '1px dashed #7a7a7a',
        borderRadius: 6,
        background: 'rgba(120, 120, 120, 0.06)',
      }}
    >
      <div
        style={{
          position: 'absolute',
          top: 4,
          left: 8,
          fontSize: 12,
          fontWeight: 600,
          color: '#555',
          pointerEvents: 'none',
          whiteSpace: 'nowrap',
        }}
      >
        {data.label}
      </div>
    </div>
  )
}
