import { useCallback, useEffect, useRef, useState, type JSX } from 'react'
import { ReactFlowProvider } from 'reactflow'
import { ApiError, getLayout, getSchema, putSchema } from './api'
import { Canvas } from './components/Canvas'
import { Editor, type SaveStatus } from './components/Editor'
import { ExportMenu } from './components/ExportMenu'
import { computeLayout, mergePositions } from './layout'
import type { Layout, Schema } from './model'
import { serialize } from './serializer'
import { clearDraft, loadDraft, saveDraft } from './storage'

// 編集中の Schema を localStorage に書き戻す debounce 間隔（要件 7.5）。
// Canvas のドラッグ保存（500ms）と同じ値に揃えて UX の体感を一致させる。
const DRAFT_DEBOUNCE_MS = 500

// 初回ロードのフェーズ。ready 後は schema/layout は必ず非 null で保持される。
type LoadStatus =
  | { status: 'loading' }
  | { status: 'error'; message: string }
  | { status: 'ready' }

// App は SPA のエントリポイント。`erdm serve` の `/api/schema` と `/api/layout`
// から取得した情報を React Flow + elkjs で可視化し、右サイドの Editor で編集
// 操作を受け付ける（design.md §C11、タスク 7.4 / 7.5 / 7.6）。
//
// 編集モードの主要フロー:
//   1. 起動時に getSchema + getLayout + ELK で配置をマージ
//   2. localStorage に下書きがあれば schema を上書き（要件 7.5）
//   3. Editor の onChange で schema 状態を更新
//   4. schema 変更検出 → 500ms debounce で saveDraft
//   5. Save 押下 → serialize → putSchema → clearDraft（要件 7.6）
//   6. Discard 押下 → clearDraft → サーバから再取得
export function App(): JSX.Element {
  const [loadStatus, setLoadStatus] = useState<LoadStatus>({ status: 'loading' })
  const [schema, setSchema] = useState<Schema | null>(null)
  const [layout, setLayout] = useState<Layout | null>(null)
  const [selectedTableName, setSelectedTableName] = useState<string | null>(null)
  const [saveStatus, setSaveStatus] = useState<SaveStatus>({ kind: 'idle' })

  const draftTimerRef = useRef<number | null>(null)
  // 「直近の schema 変更がユーザー編集由来か」のフラグ。下書き復元やサーバ
  // 再取得など、自動保存対象外の更新を区別する。
  const isUserEditRef = useRef<boolean>(false)

  useEffect(() => {
    let cancelled = false
    void (async () => {
      try {
        const [serverSchema, existing] = await Promise.all([getSchema(), getLayout()])
        const draft = loadDraft()
        const effective = draft ?? serverSchema
        const computed = await computeLayout(effective)
        const merged = mergePositions(existing, computed, effective)
        if (cancelled) return
        isUserEditRef.current = false
        setSchema(effective)
        setLayout(merged)
        setLoadStatus({ status: 'ready' })
      } catch (err) {
        if (cancelled) return
        setLoadStatus({ status: 'error', message: formatError(err) })
      }
    })()
    return () => {
      cancelled = true
    }
  }, [])

  // schema 変更を 500ms debounce して localStorage に保存（要件 7.5）。
  // 下書き復元・サーバ再取得など非ユーザー操作起点の更新では isUserEditRef が
  // false に設定されており、保存をスキップする（既存内容の再保存を防ぐ）。
  useEffect(() => {
    if (schema === null) return
    if (!isUserEditRef.current) return
    if (draftTimerRef.current !== null) {
      window.clearTimeout(draftTimerRef.current)
    }
    const timer = window.setTimeout(() => {
      draftTimerRef.current = null
      saveDraft(schema)
    }, DRAFT_DEBOUNCE_MS)
    draftTimerRef.current = timer
    return () => {
      window.clearTimeout(timer)
      if (draftTimerRef.current === timer) {
        draftTimerRef.current = null
      }
    }
  }, [schema])

  const handleSchemaChange = useCallback((next: Schema): void => {
    isUserEditRef.current = true
    setSchema(next)
    // ユーザーが新たに編集を始めた瞬間に直近の保存状態表示は無効化する。
    setSaveStatus((current) => (current.kind === 'success' ? { kind: 'idle' } : current))
  }, [])

  const handleSave = useCallback(async (): Promise<void> => {
    if (schema === null) return
    setSaveStatus({ kind: 'saving' })
    try {
      const text = serialize(schema)
      await putSchema(text)
      clearDraft()
      isUserEditRef.current = false
      setSaveStatus({ kind: 'success' })
    } catch (err) {
      setSaveStatus({ kind: 'error', message: formatError(err) })
    }
  }, [schema])

  const handleDiscard = useCallback(async (): Promise<void> => {
    setSaveStatus({ kind: 'saving' })
    try {
      clearDraft()
      const [serverSchema, existing] = await Promise.all([getSchema(), getLayout()])
      const computed = await computeLayout(serverSchema)
      const merged = mergePositions(existing, computed, serverSchema)
      isUserEditRef.current = false
      setSchema(serverSchema)
      setLayout(merged)
      setSelectedTableName(null)
      setSaveStatus({ kind: 'idle' })
    } catch (err) {
      setSaveStatus({ kind: 'error', message: formatError(err) })
    }
  }, [])

  const handleSelectedTableNameChange = useCallback((name: string | null): void => {
    setSelectedTableName(name)
  }, [])

  if (loadStatus.status === 'loading') {
    return <p>Loading...</p>
  }
  if (loadStatus.status === 'error') {
    return (
      <p style={{ color: 'red' }} role="alert">
        Error: {loadStatus.message}
      </p>
    )
  }
  if (schema === null || layout === null) {
    // ready 到達時は schema/layout がセット済みのはずなので Fail Fast で表面化させる。
    throw new Error('App reached ready state without schema/layout')
  }

  return (
    <div style={{ display: 'flex', width: '100vw', height: '100vh' }}>
      <div style={{ flex: 1, height: '100vh', minWidth: 0 }}>
        <ReactFlowProvider>
          <Canvas
            schema={schema}
            initialLayout={layout}
            onNodeClick={handleSelectedTableNameChange}
          />
        </ReactFlowProvider>
      </div>
      <Editor
        schema={schema}
        selectedTableName={selectedTableName}
        onChange={handleSchemaChange}
        onSelectedTableNameChange={handleSelectedTableNameChange}
        onSave={() => {
          void handleSave()
        }}
        onDiscard={() => {
          void handleDiscard()
        }}
        saveStatus={saveStatus}
      />
      <aside
        style={{
          width: '260px',
          height: '100vh',
          overflowY: 'auto',
          borderLeft: '1px solid #ccc',
          padding: '12px',
          boxSizing: 'border-box',
          background: '#fff',
          fontSize: '14px',
          flexShrink: 0,
        }}
      >
        <ExportMenu basename="schema" />
      </aside>
    </div>
  )
}

function formatError(err: unknown): string {
  if (err instanceof ApiError) return err.message
  if (err instanceof Error) return err.message
  return 'Unknown error'
}
