// API client の単体テスト（タスク 7.8）。
//
// 検証対象: `getSchema` / `putSchema` / `getLayout` / `putLayout` /
// `exportDDL` / `exportSVG` / `exportPNG` の正常系・異常系。
// `global.fetch` を `vi.fn()` でモックし、Response オブジェクトを直接構築する。
//
// Requirements: 5.4, 5.5, 5.6, 5.7, 5.8

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Layout, Schema } from '../model'
import {
  ApiError,
  exportDDL,
  exportPNG,
  exportSVG,
  getLayout,
  getSchema,
  putLayout,
  putSchema,
} from './client'

const SAMPLE_SCHEMA: Schema = {
  Title: 't',
  Tables: [],
  Groups: [],
}

const SAMPLE_LAYOUT: Layout = {
  users: { x: 10, y: 20 },
}

let fetchMock: ReturnType<typeof vi.fn>

beforeEach(() => {
  fetchMock = vi.fn()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('getSchema', () => {
  it('returns parsed Schema on 200', async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify(SAMPLE_SCHEMA), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    const got = await getSchema()
    expect(got).toEqual(SAMPLE_SCHEMA)
    expect(fetchMock).toHaveBeenCalledWith('/api/schema', { method: 'GET' })
  })

  it('throws ApiError when server returns error envelope', async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ error: { code: 'parse_error', message: 'bad input' } }),
        { status: 500, headers: { 'Content-Type': 'application/json' } },
      ),
    )
    const err = await getSchema().catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).status).toBe(500)
    expect((err as ApiError).code).toBe('parse_error')
    expect((err as ApiError).message).toBe('bad input')
  })

  it('throws ApiError with http_error code for non-JSON error responses', async () => {
    fetchMock.mockResolvedValueOnce(new Response('plain text body', { status: 502 }))
    const err = await getSchema().catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).status).toBe(502)
    expect((err as ApiError).code).toBe('http_error')
  })
})

describe('putSchema', () => {
  it('sends text/plain body', async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }))
    await putSchema('# Title: t\n')
    const [url, init] = fetchMock.mock.calls[0] ?? []
    expect(url).toBe('/api/schema')
    expect(init).toMatchObject({
      method: 'PUT',
      headers: { 'Content-Type': 'text/plain; charset=utf-8' },
      body: '# Title: t\n',
    })
  })

  it('throws ApiError on 403 (write disabled)', async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: { code: 'write_disabled', message: 'no' } }), {
        status: 403,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    const err = await putSchema('').catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).status).toBe(403)
    expect((err as ApiError).code).toBe('write_disabled')
  })
})

describe('getLayout / putLayout', () => {
  it('getLayout returns parsed layout', async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify(SAMPLE_LAYOUT), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    const got = await getLayout()
    expect(got).toEqual(SAMPLE_LAYOUT)
  })

  it('putLayout sends JSON body', async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }))
    await putLayout(SAMPLE_LAYOUT)
    const [url, init] = fetchMock.mock.calls[0] ?? []
    expect(url).toBe('/api/layout')
    expect(init).toMatchObject({
      method: 'PUT',
      headers: { 'Content-Type': 'application/json; charset=utf-8' },
    })
    expect(JSON.parse((init as RequestInit).body as string)).toEqual(SAMPLE_LAYOUT)
  })
})

describe('exportDDL / exportSVG / exportPNG', () => {
  it('exportDDL passes dialect via query string and returns Response', async () => {
    const body = 'CREATE TABLE users();'
    fetchMock.mockResolvedValueOnce(new Response(body, { status: 200 }))
    const res = await exportDDL('pg')
    expect(await res.text()).toBe(body)
    expect(fetchMock).toHaveBeenCalledWith('/api/export/ddl?dialect=pg', { method: 'GET' })
  })

  it('exportDDL with sqlite3 dialect', async () => {
    fetchMock.mockResolvedValueOnce(new Response('', { status: 200 }))
    await exportDDL('sqlite3')
    expect(fetchMock).toHaveBeenCalledWith('/api/export/ddl?dialect=sqlite3', {
      method: 'GET',
    })
  })

  it('exportSVG returns Response on success', async () => {
    fetchMock.mockResolvedValueOnce(new Response('<svg/>', { status: 200 }))
    const res = await exportSVG()
    expect(await res.text()).toBe('<svg/>')
  })

  it('exportSVG throws ApiError(503) when graphviz is missing', async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          error: { code: 'graphviz_not_available', message: 'dot binary missing' },
        }),
        { status: 503, headers: { 'Content-Type': 'application/json' } },
      ),
    )
    const err = await exportSVG().catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).status).toBe(503)
    expect((err as ApiError).code).toBe('graphviz_not_available')
  })

  it('exportPNG throws ApiError(503) when graphviz is missing', async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          error: { code: 'graphviz_not_available', message: 'dot binary missing' },
        }),
        { status: 503, headers: { 'Content-Type': 'application/json' } },
      ),
    )
    const err = await exportPNG().catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).status).toBe(503)
  })
})
