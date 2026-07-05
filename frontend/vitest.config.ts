// Vitest 設定（タスク 7.8 / 要件 5.10, 5.11, 6.1, 7.1〜7.10）。
//
// 設計判断:
//   - environment: jsdom — DOM API（fetch / localStorage / URL.createObjectURL）を
//     使用するためノード上で DOM をエミュレートする。
//   - globals: true — `describe` / `it` / `expect` / `vi` を import 不要にする。
//     既存の TS 厳格設定（noUnusedLocals）下で記述量を削減するため。
//   - setupFiles: jest-dom 拡張を毎テストファイルで有効化する。
//
// vite.config.ts と二重設定にしないため独立ファイルに分離（Vitest 推奨）。

import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test-setup.ts'],
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
  },
})
