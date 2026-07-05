// ExportMenu: DDL/SVG/PNG ダウンロード UI（タスク 7.7、要件 8.1, 8.2, 8.3, 8.4, 9.4）。
//
// 役割:
//   - サーバの `/api/export/{ddl|svg|png}` を呼び、レスポンス本体を `<a download>`
//     経由でユーザにダウンロードさせる。DDL は dialect クエリ（pg / sqlite3）で
//     PostgreSQL / SQLite3 を切り替える。
//   - Graphviz 不在（503 graphviz_not_available）はユーザに通知する。
//
// 設計判断:
//   - api/client.ts は失敗 Response を ApiError に変換して throw するため、ここでは
//     ApiError を捕捉して 503 専用メッセージとそれ以外（HTTP ステータス + message）の
//     2 系統で表示する。`alert()` を使う（design.md §C11 のフロント UI は最小構成）。
//   - basename は呼び出し側から受け取り、ダウンロードファイル名のステム部分に使う
//     （例: `schema` → `schema.pg.sql` / `schema.svg` / `schema.png`）。
//   - ダウンロード処理は `Response.blob()` → `URL.createObjectURL` → 動的 `<a>`
//     クリック → `URL.revokeObjectURL` の標準パターン。

import { useState, type JSX } from 'react'
import { ApiError, exportDDL, exportPNG, exportSVG } from '../../api'
import type { DdlDialect } from '../../model'

export interface ExportMenuProps {
  basename: string
}

export function ExportMenu({ basename }: ExportMenuProps): JSX.Element {
  const [dialect, setDialect] = useState<DdlDialect>('pg')
  const [busy, setBusy] = useState<boolean>(false)

  const runExport = async (
    fetcher: () => Promise<Response>,
    filename: string,
  ): Promise<void> => {
    setBusy(true)
    try {
      const res = await fetcher()
      const blob = await res.blob()
      triggerBlobDownload(blob, filename)
    } catch (err) {
      notifyExportError(err)
    } finally {
      setBusy(false)
    }
  }

  const handleDownloadDDL = (): void => {
    void runExport(() => exportDDL(dialect), `${basename}.${dialect}.sql`)
  }
  const handleDownloadSVG = (): void => {
    void runExport(() => exportSVG(), `${basename}.svg`)
  }
  const handleDownloadPNG = (): void => {
    void runExport(() => exportPNG(), `${basename}.png`)
  }

  return (
    <section
      aria-label="Export"
      style={{ borderTop: '1px solid #ccc', marginTop: '8px', paddingTop: '8px' }}
    >
      <h3 style={{ fontSize: '13px', margin: '0 0 4px' }}>Export</h3>
      <div style={{ display: 'flex', gap: '4px', alignItems: 'center', marginBottom: '4px' }}>
        <label htmlFor="export-ddl-dialect" style={{ fontSize: '12px' }}>
          DDL dialect:
        </label>
        <select
          id="export-ddl-dialect"
          value={dialect}
          onChange={(e) => setDialect(e.target.value as DdlDialect)}
          disabled={busy}
        >
          <option value="pg">PostgreSQL</option>
          <option value="sqlite3">SQLite3</option>
        </select>
      </div>
      <div style={{ display: 'flex', gap: '4px', flexWrap: 'wrap' }}>
        <button type="button" onClick={handleDownloadDDL} disabled={busy}>
          Download DDL
        </button>
        <button type="button" onClick={handleDownloadSVG} disabled={busy}>
          Download SVG
        </button>
        <button type="button" onClick={handleDownloadPNG} disabled={busy}>
          Download PNG
        </button>
      </div>
    </section>
  )
}

// triggerBlobDownload は blob を `<a download>` 経由でダウンロードさせる。
function triggerBlobDownload(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob)
  try {
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
  } finally {
    URL.revokeObjectURL(url)
  }
}

// notifyExportError は ApiError を 503（Graphviz 不在）と通常エラーに分けて通知する。
function notifyExportError(err: unknown): void {
  if (err instanceof ApiError) {
    if (err.status === 503) {
      window.alert(
        'Graphviz is not available on the server. SVG/PNG export is disabled. ' +
          'Install dot (graphviz) and restart the server.',
      )
      return
    }
    window.alert(`Export failed: ${err.message} (HTTP ${err.status})`)
    return
  }
  const message = err instanceof Error ? err.message : 'Unknown error'
  window.alert(`Export failed: ${message}`)
}
