// ExportMenu の単体テスト（タスク 7.8、要件 8.1 / 8.2 / 8.3 / 8.4 / 9.4）。
//
// 検証対象:
//   - dialect 切替で `exportDDL` の引数が `pg` / `sqlite3` の両方で正しい
//   - DDL / SVG / PNG ボタンクリックで対応 API が呼び出される
//   - ダウンロードファイル名が `<basename>.<dialect>.sql` / `<basename>.svg` /
//     `<basename>.png` の規則に従う
//   - ApiError(503, "graphviz_not_available") を捕捉してアラート通知する
//
// Requirements: 8.3, 8.4, 9.4

import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import { ApiError, exportDDL, exportPNG, exportSVG } from '../../api'
import { ExportMenu } from './ExportMenu'

vi.mock('../../api', async () => {
  const actual = await vi.importActual<typeof import('../../api')>('../../api')
  return {
    ...actual,
    exportDDL: vi.fn(),
    exportSVG: vi.fn(),
    exportPNG: vi.fn(),
  }
})

const exportDDLMock = exportDDL as unknown as Mock
const exportSVGMock = exportSVG as unknown as Mock
const exportPNGMock = exportPNG as unknown as Mock

let alertSpy: ReturnType<typeof vi.spyOn>
let clickSpy: ReturnType<typeof vi.spyOn>
const downloadAttempts: Array<{ href: string; download: string }> = []
const originalCreateObjectURL = (URL as { createObjectURL?: unknown }).createObjectURL
const originalRevokeObjectURL = (URL as { revokeObjectURL?: unknown }).revokeObjectURL

beforeEach(() => {
  downloadAttempts.length = 0
  exportDDLMock.mockReset()
  exportSVGMock.mockReset()
  exportPNGMock.mockReset()
  // jsdom は URL.createObjectURL / revokeObjectURL を実装していないため、
  // Object.defineProperty で必要な間だけ差し込む。
  Object.defineProperty(URL, 'createObjectURL', {
    configurable: true,
    writable: true,
    value: vi.fn(() => 'blob:mock-url'),
  })
  Object.defineProperty(URL, 'revokeObjectURL', {
    configurable: true,
    writable: true,
    value: vi.fn(),
  })
  alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => undefined)
  // jsdom は <a>.click() でナビゲーションを試みると例外を投げる場合がある。
  // 代わりに href / download を記録してクリック動作を握りつぶす。
  clickSpy = vi
    .spyOn(HTMLAnchorElement.prototype, 'click')
    .mockImplementation(function (this: HTMLAnchorElement) {
      downloadAttempts.push({ href: this.href, download: this.download })
    })
})

afterEach(() => {
  alertSpy.mockRestore()
  clickSpy.mockRestore()
  if (originalCreateObjectURL === undefined) {
    delete (URL as { createObjectURL?: unknown }).createObjectURL
  } else {
    Object.defineProperty(URL, 'createObjectURL', {
      configurable: true,
      writable: true,
      value: originalCreateObjectURL,
    })
  }
  if (originalRevokeObjectURL === undefined) {
    delete (URL as { revokeObjectURL?: unknown }).revokeObjectURL
  } else {
    Object.defineProperty(URL, 'revokeObjectURL', {
      configurable: true,
      writable: true,
      value: originalRevokeObjectURL,
    })
  }
})

describe('ExportMenu', () => {
  it('downloads DDL with default pg dialect', async () => {
    exportDDLMock.mockResolvedValueOnce(new Response('CREATE TABLE u();', { status: 200 }))
    render(<ExportMenu basename="schema" />)
    fireEvent.click(screen.getByRole('button', { name: 'Download DDL' }))
    await waitFor(() => expect(downloadAttempts).toHaveLength(1))
    expect(exportDDLMock).toHaveBeenCalledWith('pg')
    expect(downloadAttempts[0]).toEqual({ href: 'blob:mock-url', download: 'schema.pg.sql' })
  })

  it('downloads DDL with sqlite3 dialect when changed', async () => {
    exportDDLMock.mockResolvedValueOnce(new Response('CREATE TABLE u();', { status: 200 }))
    render(<ExportMenu basename="schema" />)
    fireEvent.change(screen.getByLabelText('DDL dialect:'), {
      target: { value: 'sqlite3' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Download DDL' }))
    await waitFor(() => expect(downloadAttempts).toHaveLength(1))
    expect(exportDDLMock).toHaveBeenCalledWith('sqlite3')
    expect(downloadAttempts[0]).toEqual({ href: 'blob:mock-url', download: 'schema.sqlite3.sql' })
  })

  it('downloads SVG and PNG with the corresponding filenames', async () => {
    exportSVGMock.mockResolvedValueOnce(new Response('<svg/>', { status: 200 }))
    exportPNGMock.mockResolvedValueOnce(new Response(new Uint8Array([0x89, 0x50]), { status: 200 }))
    render(<ExportMenu basename="erd" />)
    fireEvent.click(screen.getByRole('button', { name: 'Download SVG' }))
    await waitFor(() => expect(downloadAttempts).toHaveLength(1))
    fireEvent.click(screen.getByRole('button', { name: 'Download PNG' }))
    await waitFor(() => expect(downloadAttempts).toHaveLength(2))
    expect(exportSVGMock).toHaveBeenCalledTimes(1)
    expect(exportPNGMock).toHaveBeenCalledTimes(1)
    expect(downloadAttempts.map((d) => d.download)).toEqual(['erd.svg', 'erd.png'])
  })

  it('alerts a graphviz-specific message when SVG export returns 503', async () => {
    exportSVGMock.mockRejectedValueOnce(
      new ApiError(503, 'graphviz_not_available', 'dot binary missing', undefined),
    )
    render(<ExportMenu basename="schema" />)
    fireEvent.click(screen.getByRole('button', { name: 'Download SVG' }))
    await waitFor(() => expect(alertSpy).toHaveBeenCalledTimes(1))
    expect(alertSpy.mock.calls[0]?.[0]).toContain('Graphviz is not available')
    expect(downloadAttempts).toHaveLength(0)
  })

  it('alerts a generic error message for non-503 ApiError', async () => {
    exportDDLMock.mockRejectedValueOnce(
      new ApiError(500, 'internal_error', 'boom', undefined),
    )
    render(<ExportMenu basename="schema" />)
    fireEvent.click(screen.getByRole('button', { name: 'Download DDL' }))
    await waitFor(() => expect(alertSpy).toHaveBeenCalledTimes(1))
    expect(alertSpy.mock.calls[0]?.[0]).toContain('Export failed')
    expect(alertSpy.mock.calls[0]?.[0]).toContain('500')
  })
})
