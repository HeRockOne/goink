import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import './i18n'
import App from './App.tsx'
import ErrorBoundary from '@/components/shared/ErrorBoundary'
import { installGlobalErrorHandlers } from '@/lib/errorLog'

// 全局错误钩子必须在应用挂载前安装：渲染期崩溃/异步异常都会被记录到 goink.log
installGlobalErrorHandlers()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ErrorBoundary label="root">
      <App />
    </ErrorBoundary>
  </StrictMode>,
)
