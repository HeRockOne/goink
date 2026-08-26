// tokenRate — 流式 token 速率追踪。
// 实时速率：CJK 字符 ≈0.6 token/字、其他 ≈0.25 token/字符估算，3s 滑动窗口（网速式体验）。
// 回合结束定格值：改用后端真实 completion_tokens（acc_completion_tokens 回合内增量）÷
// 真实生成时长（首 token 时刻 → 末次 usage 时刻），数值精确；无 usage 时退回字符估算均值。

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
  addUsage(accCompletionTokens: number): void
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
  // 真实 token 计数（会话累计 completion_tokens），用于回合结束精确定格
  let realAccFirst: number | null = null
  let realAccLast: number | null = null
  let realLastAt = 0

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
    realAccFirst = null
    realAccLast = null
    realLastAt = 0
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
          emit(r !== null ? Math.round(r * 10) / 10 : 0)
        }, wait)
      }
      if (idleTimer) clearTimeout(idleTimer)
      idleTimer = setTimeout(() => emit(0), IDLE_MS)
    },
    addUsage(accCompletionTokens: number) {
      const now = Date.now()
      if (realAccFirst === null) realAccFirst = accCompletionTokens
      realAccLast = accCompletionTokens
      realLastAt = now
    },
    finish() {
      clearTimers()
      // 精确定格：真实 completion_tokens 回合内增量 ÷ 真实生成时长
      if (realAccFirst !== null && realAccLast !== null && realLastAt > 0 && firstDeltaAt > 0 && realLastAt > firstDeltaAt) {
        const realTokens = realAccLast - realAccFirst
        const realMs = realLastAt - firstDeltaAt
        const precise = realMs > 0 && realTokens > 0 ? realTokens / (realMs / 1000) : 0
        reset()
        lastEmitAt = 0
        emit(Math.round(precise * 10) / 10)
        return
      }
      // 兜底：无 usage 时退回字符估算均值
      const avg = activeMs > 500 && totalTokens > 0
        ? Math.round((totalTokens / (activeMs / 1000)) * 10) / 10
        : 0
      reset()
      lastEmitAt = 0
      emit(avg)
    },
    cancel() {
      clearTimers()
      reset()
      lastEmitAt = 0
      emit(0)
    },
  }
}
