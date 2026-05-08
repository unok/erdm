import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Vite ビルド設定。
// - `outDir: 'dist'` でビルド成果物を `frontend/dist/` に集約する（Makefile の verify-frontend と Go 側の埋め込み配信が前提とする経路）。
// - 開発時に `npm run dev` でフロント単体（localhost:5173）から Go バックエンド（localhost:8080）の `/api/*` を叩けるよう
//   プロキシを設定する。タスク 7.2 以降の API クライアントが localhost 直叩きを意識せず動作するための前提。
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
