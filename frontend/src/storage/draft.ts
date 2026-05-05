// localStorage 下書き保存ヘルパ（タスク 7.6 / 要件 7.5）。
//
// 役割: 編集中の `Schema` をブラウザの `localStorage` に純粋関数で読み書きする。
// 連続更新の debounce や保存タイミングは呼び出し側 (App.tsx) の責務とし、本
// モジュールは「いま受け取った Schema を 1 度書く / 1 度読む / 削除する / 存在を
// 確認する」だけに徹する（Simple > Easy）。
//
// 単一ドキュメント前提: `erdm serve` は 1 プロセス 1 ファイルなので、キーは
// 固定文字列で十分（design.md §C11、tasks.md 7.6 の検討）。複数ファイル対応の
// キー動的化は要件外。
//
// 例外方針: 容量超過 (`QuotaExceededError`) や `localStorage` 不可用環境
// （プライベートブラウジング等）でも編集自体は継続できるよう、`saveDraft`
// は内部で `console.warn` に降格してから戻る。`loadDraft` は JSON parse 失敗
// と未保存（null）を `null` で同一視する（呼び出し側はサーバ取得値にフォール
// バックすればよい）。

import type { Schema } from '../model'

// localStorage キー。SPA 内で唯一の下書きを表す。
const DRAFT_KEY = 'erdm-draft'

// loadDraft は localStorage から下書きを取り出して `Schema` として返す。
//
// 取り出し時に JSON parse が失敗した場合は破損下書きとして `null` を返す
// （呼び出し側はサーバ取得値を採用する）。`localStorage` 自体が利用できない
// 環境（古い iOS Safari のプライベートモード等）でも `null` を返すよう、
// アクセス自体を try で囲う。
export function loadDraft(): Schema | null {
  let raw: string | null
  try {
    raw = window.localStorage.getItem(DRAFT_KEY)
  } catch (err) {
    console.warn('Failed to read draft from localStorage:', err)
    return null
  }
  if (raw === null) {
    return null
  }
  try {
    return JSON.parse(raw) as Schema
  } catch (err) {
    console.warn('Failed to parse draft from localStorage:', err)
    return null
  }
}

// saveDraft は `Schema` を localStorage に保存する。
//
// 容量超過や利用不可は `console.warn` に降格して握り潰す。編集体験を維持する
// ことを優先し、保存失敗による例外を UI に伝播させない（要件 7.5 「自動保存」
// は best-effort）。
export function saveDraft(schema: Schema): void {
  try {
    window.localStorage.setItem(DRAFT_KEY, JSON.stringify(schema))
  } catch (err) {
    console.warn('Failed to save draft to localStorage:', err)
  }
}

// clearDraft は下書きを削除する。保存成功時または「Discard」操作時に呼ぶ。
export function clearDraft(): void {
  try {
    window.localStorage.removeItem(DRAFT_KEY)
  } catch (err) {
    console.warn('Failed to clear draft from localStorage:', err)
  }
}

// hasDraft は下書きが存在するか（= キーが localStorage にあるか）を返す。
export function hasDraft(): boolean {
  try {
    return window.localStorage.getItem(DRAFT_KEY) !== null
  } catch (err) {
    console.warn('Failed to access localStorage:', err)
    return false
  }
}
