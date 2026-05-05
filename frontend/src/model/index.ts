// 公開境界: サーバ JSON ⇄ 内部 TS モデルの型定義をまとめて再エクスポートする
// （design.md §C11）。実体は `./types` に集約しており、本ファイルは
// `import { Schema } from '../model'` 形式の参照点として機能する。
export * from './types'
