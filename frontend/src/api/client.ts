// HTTP fetch ラッパ（design.md §C11、§C10）。
//
// 役割: スキーマ・座標・エクスポートの各 API への薄いクライアントを提供し、
// サーバの JSON 形式（`internal/server`）と内部 TS モデル（`../model`）の
// 相互変換を一箇所に集約する。エラーレスポンスは `ApiError` に正規化し、
// UI 層から `instanceof ApiError` で扱えるようにする。
//
// 設計判断:
//   - エクスポート系 (`exportDDL` / `exportSVG` / `exportPNG`) は `Response` を
//     そのまま返す。UI 層 (タスク 7.7) で用途に応じて `blob()` / `text()` を
//     呼び分けるため。
//   - `PUT /api/schema` は `text/plain`（Go 側はバイト単位で受信ボディを保存、
//     要件 7.7）。
//   - `PUT /api/layout` は `application/json`（Go 側は `json.Decode`）。
//   - 認証ヘッダ・CORS 対応は対象外（`erdm serve` はローカル限定 = 127.0.0.1 既定）。

import type { DdlDialect, Layout, Schema } from '../model'

// API ベース URL。Vite の dev サーバではプロキシ経由（vite.config.ts）、
// 本番ビルドでは Go バイナリの SPA 配信と同一オリジンで動作する。
const API_BASE = '/api'

// ApiError は HTTP エラー応答を構造化した例外。Go 側 `internal/server/errors.go`
// が返す `{ "error": { "code": string, "message": string, "detail"?: object } }`
// に対応する。
export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly detail: unknown

  constructor(status: number, code: string, message: string, detail: unknown) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.detail = detail
  }
}

// errorEnvelope は parseApiError が期待する JSON 形状。フィールドの存在は
// 受信時に動的検証する（unknown 経由でアクセス）。
interface ErrorEnvelope {
  error?: {
    code?: unknown
    message?: unknown
    detail?: unknown
  }
}

// parseApiError は失敗 Response を ApiError に変換する。
//
// JSON 解釈に失敗した場合（HTTP 5xx でテキストエラー本文が返るケースなど）でも
// 必ず ApiError を返す（status コードを保持する）。
export async function parseApiError(res: Response): Promise<ApiError> {
  let envelope: ErrorEnvelope = {}
  try {
    envelope = (await res.json()) as ErrorEnvelope
  } catch {
    // JSON でない（または空ボディの）応答は status から既定メッセージを生成する。
    return new ApiError(
      res.status,
      'http_error',
      `HTTP ${res.status} ${res.statusText || ''}`.trim(),
      undefined,
    )
  }
  const err = envelope.error ?? {}
  const code = typeof err.code === 'string' ? err.code : 'http_error'
  const message =
    typeof err.message === 'string'
      ? err.message
      : `HTTP ${res.status} ${res.statusText || ''}`.trim()
  return new ApiError(res.status, code, message, err.detail)
}

// throwIfNotOk は失敗 Response を ApiError に変換して throw する小ヘルパ。
async function throwIfNotOk(res: Response): Promise<void> {
  if (!res.ok) {
    throw await parseApiError(res)
  }
}

// getSchema は GET /api/schema を呼び、サーバが json.Encode した
// `*model.Schema` を Schema 型として返す（要件 5.4）。
export async function getSchema(): Promise<Schema> {
  const res = await fetch(`${API_BASE}/schema`, { method: 'GET' })
  await throwIfNotOk(res)
  return (await res.json()) as Schema
}

// putSchema は PUT /api/schema にシリアライズ済み `.erdm` テキストを送る
// （要件 5.4 / 7.7 / 7.8）。Content-Type は `text/plain`（サーバはバイト単位
// で受信ボディを保存する）。
export async function putSchema(text: string): Promise<void> {
  const res = await fetch(`${API_BASE}/schema`, {
    method: 'PUT',
    headers: { 'Content-Type': 'text/plain; charset=utf-8' },
    body: text,
  })
  await throwIfNotOk(res)
}

// getLayout は GET /api/layout を呼び、座標ストアの内容を返す
// （要件 5.5 / 6.1 / 6.5）。ファイル不存在時は `{}`（空オブジェクト）が返る。
export async function getLayout(): Promise<Layout> {
  const res = await fetch(`${API_BASE}/layout`, { method: 'GET' })
  await throwIfNotOk(res)
  return (await res.json()) as Layout
}

// putLayout は PUT /api/layout に座標 JSON を送る（要件 5.5 / 6.1 / 6.2）。
// Content-Type は `application/json`（サーバは json.Decode で受信する）。
export async function putLayout(layout: Layout): Promise<void> {
  const res = await fetch(`${API_BASE}/layout`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json; charset=utf-8' },
    body: JSON.stringify(layout),
  })
  await throwIfNotOk(res)
}

// exportDDL は GET /api/export/ddl を呼び、生 Response を返す（要件 5.6 / 8.1 / 8.2）。
// 消費側 (タスク 7.7) が `text()` で本文を取得しダウンロードを行う。
export async function exportDDL(dialect: DdlDialect): Promise<Response> {
  const res = await fetch(
    `${API_BASE}/export/ddl?dialect=${encodeURIComponent(dialect)}`,
    { method: 'GET' },
  )
  await throwIfNotOk(res)
  return res
}

// exportSVG は GET /api/export/svg を呼び、生 Response を返す（要件 5.7 / 8.3）。
// 外部コマンド不在時は `ApiError(503, "graphviz_not_available", ...)` が throw される。
export async function exportSVG(): Promise<Response> {
  const res = await fetch(`${API_BASE}/export/svg`, { method: 'GET' })
  await throwIfNotOk(res)
  return res
}

// exportPNG は GET /api/export/png を呼び、生 Response を返す（要件 5.8 / 8.4）。
// 外部コマンド不在時は `ApiError(503, "graphviz_not_available", ...)` が throw される。
export async function exportPNG(): Promise<Response> {
  const res = await fetch(`${API_BASE}/export/png`, { method: 'GET' })
  await throwIfNotOk(res)
  return res
}
