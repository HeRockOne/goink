// tokenRate — 流式 token 速率追踪（纯前端估算，回合结束冻结平均速率）。
// 估算口径：CJK 字符 ≈0.6 token/字，其他 ≈0.25 token/字符（英文 ~4 字符/token）。
// 瞬时速率用 3s 滑动窗口；>2s 无 delta 视为停顿（工具执行等），窗口重开且速率归零隐藏；
// finish() 在回合结束时按活跃输出时间定格本轮平均速率。

const WINDOW_MS = 3000
const IDLE_MS = 2000
const EMIT_MS = 500

function estimateTokens(text: string): number {
  let n = 0
  for (let i = 0; i < text.length; i++) {
    const cp = text.codePointAt(i)!
    if (cp > 0xffff) i++
    if ((cp >= 0x4e00 && cp <= 0x9fff) || (cp >= 0x3400 && cp <= 0x4dbf) ||
        (cp >= 0xf900 && cp <= 0xfaff) || (cp >= 0x3000 && cp <= 0x303f) ||
        (cp >= 0xff00 && cp <= 0xffef)) {
      n += 0.6
    } else {
      n += 0.25
    }
  }
  return n
}

export interface TokenRateTracker {
  addDelta(text: string): void
  finish(): void
  cancel(): void
}

export function createTokenRateTracker(emit: (tps: number | null) => void): TokenRateTracker {
  let totalTokens = 0
  let activeMs = 0
  let firstDeltaAt = 0
  let lastDeltaAt = 0
  let lastEmitAt = 0
  let samples: Array<{ t: number; cum: number }> = []
  let emitTimer: ReturnType<typeof setTimeout> | null = null
  let idleTimer: ReturnType<typeof setTimeout> | null = null

  function clearTimers() {
    if (emitTimer) { clearTimeout(emitTimer); emitTimer = null }
    if (idleTimer) { clearTimeout(idleTimer); idleTimer = null }
  }

  function reset() {
    totalTokens = 0
    activeMs = 0
    firstDeltaAt = 0
    lastDeltaAt = 0
    samples = []
  }

  function liveRate(now: number): number | null {
    while (samples.length > 1 && now - samples[0].t > WINDOW_MS) samples.shift()
    if (samples.length < 2) return null
    const dt = (now - samples[0].t) / 1000
    if (dt <= 0) return null
    return (samples[samples.length - 1].cum - samples[0].cum) / dt
  }

  return {
    addDelta(text) {
      if (!text) return
      const now = Date.now()
      if (lastDeltaAt && now - lastDeltaAt >= IDLE_MS) {
        samples = [] // 停顿后重开窗口，不把停顿算进分母
      } else if (lastDeltaAt) {
        activeMs += now - lastDeltaAt
      }
      if (!firstDeltaAt) firstDeltaAt = now
      lastDeltaAt = now
      totalTokens += estimateTokens(text)
      samples.push({ t: now, cum: totalTokens })
      if (!emitTimer) {
        const wait = Math.max(0, EMIT_MS - (now - lastEmitAt))
        emitTimer = setTimeout(() => {
          emitTimer = null
          lastEmitAt = Date.now()
          const r = liveRate(Date.now())
          emit(r !== null ? Math.round(r * 10) / 10 : null)
        }, wait)
      }
      if (idleTimer) clearTimeout(idleTimer)
      idleTimer = setTimeout(() => emit(null), IDLE_MS)
    },
    finish() {
      clearTimers()
      const avg = activeMs > 500 && totalTokens > 0
        ? Math.round((totalTokens / (activeMs / 1000)) * 10) / 10
        : null
      reset()
      lastEmitAt = 0
      emit(avg)
    },
    cancel() {
      clearTimers()
      reset()
      lastEmitAt = 0
      emit(null)
    },
  }
}
