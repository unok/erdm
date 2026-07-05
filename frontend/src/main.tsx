import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'

// Fail Fast: ルート要素が無い HTML が読み込まれた状況は構成バグなので例外で停止させる。
const rootElement = document.getElementById('root')
if (!rootElement) {
  throw new Error('Root element #root not found in index.html')
}

createRoot(rootElement).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
