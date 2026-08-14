import { LogFrontendError } from '@/lib/wailsjs/go/app/App'

// logFrontendError 上报前端错误到后端 goink.log（slog ERROR 级）。
// 前端崩溃时后端收不到任何请求，必须显式上报才能定位（白屏黑盒问题）。
export function logFrontendError(message: string, stack?: string, filename?: string, line?: number, col?: number) {
  console.error('[goink-frontend]', message, stack ?? '')
  const detail = [
    stack ?? '',
    filename ? `at ${filename}${line ? ':' + line : ''}${col ? ':' + col : ''}` : '',
  ].filter(Boolean).join('\n')
  try {
    LogFrontendError(message, detail).catch(() => { /* 绑定未就绪 */ })
  } catch { /* window 未就绪 */ }
}

// installGlobalErrorHandlers 安装全局错误钩子：未捕获异常 + 未处理 Promise 拒绝。
// 在应用挂载前调用，保证任何渲染期/异步错误都被记录。
export function installGlobalErrorHandlers() {
  if (typeof window === 'undefined') return
  window.addEventListener('error', (e) => {
    logFrontendError(
      `uncaught: ${e.message || 'unknown error'}`,
      e.error instanceof Error ? (e.error.stack ?? '') : '',
      e.filename, e.lineno, e.colno,
    )
  })
  window.addEventListener('unhandledrejection', (e) => {
    const reason = e.reason
    const msg = reason instanceof Error ? reason.message : String(reason)
    const stack = reason instanceof Error && reason.stack ? reason.stack : ''
    logFrontendError(`unhandledrejection: ${msg}`, stack)
  })
}
