// Vitest テストセットアップ（タスク 7.8）。
//
// 役割: 全テストで `@testing-library/jest-dom` のカスタムマッチャ
// （`toBeInTheDocument` 等）を有効化する。Vitest の `setupFiles` に登録する
// （`vitest.config.ts`）。

import '@testing-library/jest-dom/vitest'
