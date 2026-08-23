import { useState, useCallback, useRef, useEffect, forwardRef, useImperativeHandle } from 'react'
import { useTranslation } from 'react-i18next'
import { MessageSquare, Loader2, History, Plus, ArrowDown } from 'lucide-react'
import { EventsOn } from '@/lib/wailsjs/runtime/runtime'
import { useApp } from '@/hooks/useApp'
import type { llm, app } from '@/hooks/useApp'
import type { AgentEvent, Turn, TurnSegment } from './types'
import { AgentEventType, emptySegment, rebuildTurns } from './types'
import ChatInput from './ChatInput'
import ChatControls from './ChatControls'
import MessageBubble from './MessageBubble'
import ThinkingBlock from './ThinkingBlock'
import ToolCallCard, { type ToolCallDetail } from './ToolCallCard'
import WebSearchCard from './WebSearchCard'
import WebFetchCard from './WebFetchCard'
import SubagentCard from './SubagentCard'
import CompressionBlock from './CompressionBlock'
import RetryNotification from './RetryNotification'
import type { UsageInfo } from './ContextRing'
import SettingsDialog from '@/components/settings/SettingsDialog'
import SessionHistory from './SessionHistory'

interface Props {
  novelId: number
  onApprove: (toolId: string, feedback: string) => Promise<void>
  onReject: (toolId: string, feedback: string) => Promise<void>
  onApprovalFileEdit?: (payload: {
    path: string; title: string; diff: string; original: string; modified: string
    changeType: string; reason: string; toolId: string
  }) => void
  chatPanelWidth: number
  onChatPanelResize: (w: number) => void
  onPhaseGate?: (s: import('./types').PhaseStatus) => void
  onUsage?: (u: UsageInfo | null) => void
  onModelChange?: (modelID: string) => void
  phaseMode?: string
}
const EVENT_REORDER_TIMEOUT = 120

interface EventQueue {
  nextSeq: number
  pending: Map<number, AgentEvent>
  flushTimer: ReturnType<typeof setTimeout> | null
}

interface ChatStartedEvent {
  session_id?: string
  turn_id: number
}

export interface ChatPanelHandle {
  compress: () => void
}

export default forwardRef<ChatPanelHandle, Props>(function ChatPanel({ novelId, onApprove, onReject, onApprovalFileEdit, chatPanelWidth, onChatPanelResize, onPhaseGate, onUsage, onModelChange, phaseMode }: Props, ref) {
  const { t } = useTranslation()
  const app = useApp()

  // 从 model key 中安全提取 provider 和 modelID
  // key 格式：providerName/modelID（如 "deepseek/deepseek-v4-pro"）
  // 注意：modelID 可能包含 "/"，必须只在第一个 "/" 处拆分
  const splitModelKey = (key: string): [string, string] => {
    const idx = key.indexOf('/')
    return idx >= 0 ? [key.substring(0, idx), key.substring(idx + 1)] : ['', key]
  }

  const [turns, setTurns] = useState<Turn[]>([])
  // turns 镜像：供事件回调在 updater 外读取当前值（updater 内禁止副作用）
  const turnsRef = useRef<Turn[]>([])
  turnsRef.current = turns
  const [sessionId, setSessionId] = useState('')
  const [isDragging, setIsDragging] = useState(false)
  const startXRef = useRef(0)
  const startWidthRef = useRef(chatPanelWidth)
  const [isLoading, setIsLoading] = useState(false)
  const [models, setModels] = useState<llm.AvailableModel[]>([])
  const [selectedKey, setSelectedKey] = useState('')
  const [reasoningEffort, setReasoningEffort] = useState('')
  const [approvalMode, setApprovalMode] = useState<'manual' | 'auto'>('manual')
  const [thinkingEnabled, setThinkingEnabled] = useState(false)
  const compressingRef = useRef(false)
  const activeCountRef = useRef(0)
  // 移动端（API 模式）对话同步：活跃状态 + 活跃会话（供停止按钮取消正确会话）
  const [apiStreaming, setApiStreaming] = useState(false)
  const apiActiveRef = useRef<{ turnId: number | null; sessionId: string | null }>({ turnId: null, sessionId: null })
  const [showSettings, setShowSettings] = useState(false)
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null)
  const [showHistoryPanel, setShowHistoryPanel] = useState(false)
  const [isLoadingHistory, setIsLoadingHistory] = useState(false)
  // 历史消息分页渲染：默认只渲染最近 N 轮，点"加载更早"递增
  const [visibleTurnCount, setVisibleTurnCount] = useState(30)
  const [sessionTitle, setSessionTitle] = useState('')
  const [initLoadError, setInitLoadError] = useState(false)
  const [initLoadRetry, setInitLoadRetry] = useState(0)
  const [historyLoadError, setHistoryLoadError] = useState(false)
  const [historyLoadRetry, setHistoryLoadRetry] = useState(0)
  const [slashCommands, setSlashCommands] = useState<app.SlashCommand[]>([])
  const [retryInfo, setRetryInfo] = useState<{ count: number; max: number; wait: number } | null>(null)
  const [showScrollBtn, setShowScrollBtn] = useState(false)
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const scrollContainerRef = useRef<HTMLDivElement>(null)
  const isNearBottomRef = useRef(true)
  const counterRef = useRef(0)
  const startedUnsubRef = useRef<(() => void) | null>(null)
  const agentUnsubRef = useRef<(() => void) | null>(null)
  const eventQueuesRef = useRef<Map<number, EventQueue>>(new Map())
  // 发送代际号：并发发送/切换会话时，旧发送的 finally 不得注销新发送的监听器、
  // 不得把积压事件 flush 进当前会话（旧实现共享 ref 被覆盖后误删，见审计 F1/F2）
  const sendGenRef = useRef(0)
  const onApprovalFileEditRef = useRef(onApprovalFileEdit)
  useEffect(() => { onApprovalFileEditRef.current = onApprovalFileEdit }, [onApprovalFileEdit])
  // 加载模型列表并恢复持久化设置
  useEffect(() => {
    setInitLoadError(false)
    Promise.all([
      app.GetModels(),
      app.GetSettings(),
    ]).then(([modelList, settings]) => {
      if (modelList && modelList.length > 0) {
        setModels(modelList)

        // 恢复模型选择（验证 key 仍存在）
        let key = settings?.selected_model_key || ''
        let model = modelList.find(m => m.Key === key)
        if (!model) {
          model = modelList[0]
          key = model.Key
        }
        setSelectedKey(key)

        // 恢复推理程度（验证级别仍合法）
        let effort = settings?.reasoning_effort || ''
        if (effort && !model.ReasoningLevels?.includes(effort)) {
          effort = model.ReasoningLevels?.[0] || ''
        }
        setReasoningEffort(effort)

        // 恢复思考模式（独立于 reasoning_effort，防止关掉思考后重启被重置）
        const thinkingOn = settings?.thinking_enabled !== false
        setThinkingEnabled(thinkingOn && (model?.SupportsThinking ?? false))
      }

      // 恢复审批模式
      const mode = settings?.approval_mode
      if (mode === 'manual' || mode === 'auto') {
        setApprovalMode(mode)
      }

      }).catch((err) => {
      console.error('Load models/settings failed', err)
      setInitLoadError(true)
    })
  }, [app, initLoadRetry])

  // 加载会话列表（不自动选中）
  useEffect(() => {
    if (!novelId) return
    // 切小说：拆除上一小说会话的动态监听与队列（同 handleSelectSession 的理由）
    startedUnsubRef.current?.()
    startedUnsubRef.current = null
    agentUnsubRef.current?.()
    agentUnsubRef.current = null
    eventQueuesRef.current.forEach(queue => {
      if (queue.flushTimer) clearTimeout(queue.flushTimer)
    })
    eventQueuesRef.current.clear()
    sendGenRef.current++
    setActiveSessionId(null)
    setTurns([])
    setSessionId('')
    setSessionTitle('')
  }, [app, novelId])

  // 启动时恢复上次活跃会话（方案 A：重启回到上次聊到一半的会话）。
  // 仅首次挂载且 last_session_id 属于当前小说时恢复；切小说保持清空行为。
  const restoredLastSessionRef = useRef(false)

  // 监听对话完成事件：当前打开的会话被更新时刷新消息
  useEffect(() => {
    const refreshOnDone = (data: { session_id: string }) => {
      if (!novelId) return
      if (activeSessionId === data.session_id) {
        app.GetSessionMessages(data.session_id).then(msgs => {
          if (msgs) {
            setTurns(prev => {
              // 桌面端自己流式构建的 turns（id 非 hist_ 前缀）已含完整工具结果；
              // 整体重建会丢失 result 与展开状态（DB 不落库结果、段 id 变化重挂载）——
              // 仅当本地无实时数据时才重建（移动端写入、桌面纯查看场景）
              const hasLiveTurns = prev.some(t => !t.id.startsWith('hist_'))
              return hasLiveTurns ? prev : rebuildTurns(msgs)
            })
          }
        }).catch(() => {})
      }
    }
    // 桌面端 Wails 模式对话完成发 "chat:done"；API/移动端模式发 "chat:api_done"
    const cleanup1 = EventsOn('chat:done', refreshOnDone)
    const cleanup2 = EventsOn('chat:api_done', refreshOnDone)
    return () => { cleanup1(); cleanup2() }
  }, [app, novelId, activeSessionId])

  // 新会话自动生成标题后刷新头部标题（app/chat.go generateTitle 完成时发出，
  // 旧实现无监听导致标题一直为空，需手动重选会话才刷新）
  useEffect(() => {
    const cleanup = EventsOn('chat:title_updated', (data: { session_id?: string; title?: string }) => {
      if (data.session_id && data.title && data.session_id === activeSessionId) {
        setSessionTitle(data.title)
      }
    })
    return () => { cleanup() }
  }, [activeSessionId])

  // 监听移动端对话实时流事件，实现双端同步
  useEffect(() => {
    type ApiEvent = {
      type: string
      turn_id: number
      data?: string
      error?: string
      tool_name?: string
      text?: string
      session_id?: string
      message?: string
    }
    // 跟踪当前正在构建的 streaming turn
    const apiStreamRef: {
      turnId: number | null
      sessionId: string | null
      content: string
      thinking: string
      toolName: string
    } = { turnId: null, sessionId: null, content: '', thinking: '', toolName: '' }

    const cleanup = EventsOn('chat:api_event', (ev: ApiEvent) => {
      if (!novelId) return
      if (ev.type === 'started') {
        apiStreamRef.turnId = ev.turn_id
        apiStreamRef.sessionId = ev.session_id || null
        apiActiveRef.current = { turnId: ev.turn_id, sessionId: ev.session_id || null }
        setApiStreaming(true)
        apiStreamRef.content = ''
        apiStreamRef.thinking = ''
        apiStreamRef.toolName = ''
        setTurns(prev => {
          const newTurn: Turn = {
            id: `api-${ev.session_id || apiStreamRef.sessionId || '?'}:${ev.turn_id}`,
            turnId: ev.turn_id,
            userMessage: ev.message || '',
            segments: [],
            status: 'streaming',
          }
          return [...prev, newTurn]
        })
        return
      }
      // 移动端对话完成：结束 turn 状态，恢复发送按钮
      if (ev.type === 'done') {
        const tid = ev.turn_id
        if (ev.text) apiStreamRef.content = ev.text
        if (tid === apiStreamRef.turnId) {
          apiStreamRef.toolName = ''
          apiActiveRef.current = { turnId: null, sessionId: null }
          setApiStreaming(false)
        }
        setTurns(prev => prev.map(t =>
          t.id === `api-${ev.session_id || apiStreamRef.sessionId || '?'}:${tid}`
            ? {
                ...t,
                status: 'done' as const,
                segments: t.segments.map(s =>
                  s.type === 'text'
                    ? { ...s, content: ev.text || s.content, thinkingDone: true, isStreaming: false }
                    : s
                ),
              }
            : t
        ))
        return
      }
      // 移动端对话出错：结束 turn 状态，恢复发送按钮
      if (ev.type === 'error') {
        const tid = ev.turn_id
        if (tid === apiStreamRef.turnId) {
          apiActiveRef.current = { turnId: null, sessionId: null }
          setApiStreaming(false)
        }
        setTurns(prev => prev.map(t =>
          t.id === `api-${ev.session_id || apiStreamRef.sessionId || '?'}:${tid}`
            ? {
                ...t,
                status: 'failed' as const,
                errorMessage: ev.error || '未知错误',
                segments: t.segments.map(s =>
                  s.type === 'text'
                    ? { ...s, thinkingDone: true, isStreaming: false }
                    : s
                ),
              }
            : t
        ))
        return
      }
      // 更新 streaming turn 的 segments
      setTurns(prev => {
        const idx = prev.findIndex(t => t.id === `api-${ev.session_id || apiStreamRef.sessionId || '?'}:${ev.turn_id}`)
        if (idx < 0) return prev
        const last = prev[idx]
        if (last.status !== 'streaming') return prev
        const segments: TurnSegment[] = []

        // 处理 thinking 事件
        if (ev.type === 'thinking') {
          apiStreamRef.thinking += ev.data || ''
        }
        // 处理 content 事件
        if (ev.type === 'content') {
          apiStreamRef.content += ev.data || ''
        }
        // 处理 tool_call 事件
        if (ev.type === 'tool_call') {
          apiStreamRef.toolName = ev.tool_name || ''
        }
        if (ev.type === 'tool_result') {
          apiStreamRef.toolName = ''
        }

        // 构建文本段（合并 thinking + content）
        // 固定 id：增量更新最后一条 text 段，避免每次事件新建 segment 导致 ThinkingBlock 重挂载、展开状态丢失
        if (apiStreamRef.content.length > 0 || apiStreamRef.thinking.length > 0) {
          const textSegId = 'api-text'
          const existing = segments.findIndex(s => s.id === textSegId)
          const textSeg = {
            id: textSegId,
            type: 'text' as const,
            content: apiStreamRef.content,
            thinkingContent: apiStreamRef.thinking,
            thinkingDone: ev.type === 'thinking_done',
            isStreaming: ev.type === 'content',
            toolName: '', toolId: '', toolStatus: 'completed' as const,
            displayText: '', activityKind: '', error: '',
          }
          if (existing >= 0) segments[existing] = textSeg
          else segments.push(textSeg)
        }

        // 构建工具调用段
        if (apiStreamRef.toolName.length > 0) {
          segments.push({
            id: `api-tool-${Date.now()}`,
            type: 'tool',
            content: apiStreamRef.toolName,
            thinkingContent: '', thinkingDone: false, isStreaming: false,
            toolName: apiStreamRef.toolName, toolId: `api-${apiStreamRef.turnId}`,
            toolStatus: 'executing', displayText: '', activityKind: '', error: '',
          })
        }

        return prev.map((t, i) => i === idx ? { ...t, segments } : t)
      })
      return
    })
    return () => { cleanup() }
  }, [app, novelId])

  // 监听模型变更事件（移动端切换模型时桌面端实时更新）
  useEffect(() => {
    const cleanup = EventsOn('model:changed', (data: { selected_model_key?: string; reasoning_effort?: string }) => {
      if (data.selected_model_key) {
        setSelectedKey(data.selected_model_key)
        // 验证新模型存在并恢复推理程度
        const model = models.find(m => m.Key === data.selected_model_key)
        if (model && data.reasoning_effort) {
          setReasoningEffort(data.reasoning_effort)
          setThinkingEnabled(data.reasoning_effort !== '' && (model.SupportsThinking ?? false))
        }
      }
    })
    return () => { cleanup() }
  }, [app, models])

  // 监听思考深度变更事件（移动端切换思考深度时桌面端实时更新）
  useEffect(() => {
    const cleanup = EventsOn('settings:reasoning_effort_changed', (data: { reasoning_effort?: string }) => {
      if (data.reasoning_effort !== undefined) {
        setReasoningEffort(data.reasoning_effort)
        const model = models.find(m => m.Key === selectedKey)
        setThinkingEnabled(data.reasoning_effort !== '' && (model?.SupportsThinking ?? false))
      }
    })
    return () => { cleanup() }
  }, [models, selectedKey])

  // 当前模型变化时上报纯 modelID（底部计费面板按模型切换统计）
  useEffect(() => {
    const [, modelID] = splitModelKey(selectedKey)
    if (modelID) onModelChange?.(modelID)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedKey])

  // 加载历史消息：仅在显式选择会话（handleSelectSession/恢复）时触发。
  // chat:started 也会改 activeSessionId，但不能重建——流式构建的 turns 含工具结果与
  // 展开状态，中途 rebuildTurns 会替换段（result 丢失、key 变化导致展开关闭）
  const loadOnSessionRef = useRef(false)
  useEffect(() => {
    if (!activeSessionId || !novelId) return
    if (!loadOnSessionRef.current) return
    loadOnSessionRef.current = false
    setSessionId(activeSessionId)
    setHistoryLoadError(false)
    setIsLoadingHistory(true)
    app.GetSessionMessages(activeSessionId).then(msgs => {
      if (msgs) {
        setTurns(rebuildTurns(msgs))
      }
    }).catch((err) => {
      console.error('Load messages failed', err)
      setHistoryLoadError(true)
    }).finally(() => setIsLoadingHistory(false))
  }, [app, activeSessionId, novelId, historyLoadRetry])

  // ── 聊天面板缩放（右边锁死，只拖左边） ─────────────
  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    setIsDragging(true)
    startXRef.current = e.clientX
    startWidthRef.current = chatPanelWidth
  }, [chatPanelWidth])

  useEffect(() => {
    if (!isDragging) return
    const handleMouseMove = (e: MouseEvent) => {
      let w = startWidthRef.current - (e.clientX - startXRef.current)
      w = Math.min(800, Math.max(180, Math.round(w)))
      onChatPanelResize(w)
    }
    const handleMouseUp = () => setIsDragging(false)
    document.addEventListener('mousemove', handleMouseMove)
    document.addEventListener('mouseup', handleMouseUp)
    return () => {
      document.removeEventListener('mousemove', handleMouseMove)
      document.removeEventListener('mouseup', handleMouseUp)
    }
  }, [isDragging, onChatPanelResize])

  // 清理事件监听器
  useEffect(() => {
    const eventQueues = eventQueuesRef.current
    return () => {
      startedUnsubRef.current?.()
      agentUnsubRef.current?.()
      eventQueues.forEach(queue => {
        if (queue.flushTimer) clearTimeout(queue.flushTimer)
      })
      eventQueues.clear()
    }
  }, [])

  // 流式输出时自动滚到底部，但仅在用户未主动上滚时
  // 用 requestAnimationFrame 确保 DOM 更新后再滚动
  useEffect(() => {
    if (isNearBottomRef.current) {
      requestAnimationFrame(() => {
        messagesEndRef.current?.scrollIntoView({ behavior: 'instant' })
      })
    }
  }, [turns])

  const handleMessagesScroll = useCallback(() => {
    const el = scrollContainerRef.current
    if (!el) return
    const near = el.scrollHeight - el.scrollTop - el.clientHeight < 60
    isNearBottomRef.current = near
    setShowScrollBtn(!near)
  }, [])

  const scrollToBottom = useCallback(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [])

  const handleSelectSession = useCallback((sid: string) => {
    // 切换会话：注销上一会话的动态监听并清空事件队列。
    // agent 事件通道按 session 隔离（agent:{sessionID}:{turnID}），不注销时旧会话
    // 迟到事件会按撞号的 turnId 写进新会话的 turn；代际号 +1 让在途发送的
    // finally 跳过 flush（旧队列数据属于已离开的会话）
    startedUnsubRef.current?.()
    startedUnsubRef.current = null
    agentUnsubRef.current?.()
    agentUnsubRef.current = null
    eventQueuesRef.current.forEach(queue => {
      if (queue.flushTimer) clearTimeout(queue.flushTimer)
    })
    eventQueuesRef.current.clear()
    sendGenRef.current++
    loadOnSessionRef.current = true // 显式选会话 → 消息加载 effect 执行重建
    setActiveSessionId(sid)
    setVisibleTurnCount(30)
    setIsLoading(false)
    setRetryInfo(null)
    setApiStreaming(false)
    app.SetLastSession(sid).catch(() => {})
    app.GetSession(sid).then(detail => {
      if (detail) {
        setSessionTitle(detail.title || '')
        // 恢复该会话持久化的模型（sessions.model 存纯 modelID，匹配 AvailableModel.ModelName）
        let restoredModel: llm.AvailableModel | undefined
        if (detail.model) {
          const match = models.find(m => splitModelKey(m.Key)[1] === detail.model)
          if (match) {
            setSelectedKey(match.Key)
            restoredModel = match
          }
        }
        // 恢复该会话持久化的推理程度（sessions 表按会话保存，切换会话必须还原，
        // 否则沿用全局设置/默认值，后续发送的 reasoning_effort 与历史不一致）。
        // 当前模型不支持该级别时不强行覆盖（如模型已更换）
        if (detail.reasoning_effort) {
          const model = restoredModel ?? models.find(m => m.Key === selectedKey)
          if (!model || !model.ReasoningLevels || model.ReasoningLevels.includes(detail.reasoning_effort)) {
            setReasoningEffort(detail.reasoning_effort)
            setThinkingEnabled(detail.reasoning_effort !== '' && (model?.SupportsThinking ?? false))
          }
        }
        if (detail.usage) {
          onUsage?.(detail.usage as unknown as UsageInfo)
        }
      }
    }).catch(() => {})
  }, [app, onUsage, models, selectedKey])

  // 启动时恢复上次活跃会话（方案 A：重启回到上次聊到一半的会话）。
  // 仅首次挂载且 last_session_id 属于当前小说时恢复；切小说保持清空行为。
  useEffect(() => {
    if (!novelId || restoredLastSessionRef.current) return
    restoredLastSessionRef.current = true
    app.GetSettings().then(s => {
      const last = s?.last_session_id
      if (!last) return
      app.GetSession(last).then(detail => {
        if (detail && detail.novel_id === novelId) {
          handleSelectSession(last)
        }
      }).catch(() => {})
    }).catch(() => {})
  }, [app, novelId, handleSelectSession])

  const handleNewChat = useCallback(() => {
    // 新建对话：同 handleSelectSession，拆除上一会话监听与队列
    startedUnsubRef.current?.()
    startedUnsubRef.current = null
    agentUnsubRef.current?.()
    agentUnsubRef.current = null
    eventQueuesRef.current.forEach(queue => {
      if (queue.flushTimer) clearTimeout(queue.flushTimer)
    })
    eventQueuesRef.current.clear()
    sendGenRef.current++
    setActiveSessionId(null)
    setTurns([])
    setSessionId('')
    setSessionTitle('')
    setIsLoading(false)
    setRetryInfo(null)
    setApiStreaming(false)
    onUsage?.(null)
  }, [])

  const handleOpenHistory = useCallback(() => {
    setShowHistoryPanel(true)
  }, [])

  // 稳定引用：RetryNotification 的 effect deps 含 onDone，内联箭头会在每次渲染时
  // 重置倒计时定时器（流式期间父组件高频重渲染）
  const clearRetryInfo = useCallback(() => setRetryInfo(null), [])

  const handleCloseHistory = useCallback(() => {
    setShowHistoryPanel(false)
  }, [])

  const loadSlash = useCallback(async () => {
    if (!novelId) { setSlashCommands([]); return }
    try {
      const list = await app.ListSlashCommands({ novel_id: novelId })
      setSlashCommands(list ?? [])
    } catch (err) {
      console.error('Load slash commands failed', err)
    }
  }, [app, novelId])

  useEffect(() => { loadSlash() }, [loadSlash])

  const applyAgentEvent = useCallback((turnId: number, event: AgentEvent) => {
    // 收到任何事件（包括 Content）→ 关闭重试悬浮通知
    if (event.type !== AgentEventType.Retry) {
      setRetryInfo(null)
    }
    switch (event.type) {
      case AgentEventType.Usage: {
        if (event.usage) {
          onUsage?.(event.usage as unknown as UsageInfo)
        }
        return
      }
      case AgentEventType.Error: {
        setTurns(prev => prev.map(turn =>
          turn.turnId === turnId
            ? { ...turn, status: 'failed' as const, errorMessage: event.error || t('chat.chatError') }
            : turn
        ))
        return
      }
      case AgentEventType.Retry: {
        setRetryInfo({
          count: event.retry_count || 0,
          max: event.retry_max ?? 3,
          wait: event.retry_wait || 5,
        })
        return
      }
      case AgentEventType.Compression: {
        const phase = (event.compression_phase || 'started') as 'compressing' | 'done'
        if (event.sub_task_id) {
          setTurns(prev => prev.map(turn => {
            if (turn.turnId !== turnId) return turn
            const subIdx = turn.segments.findIndex(s =>
              s.type === 'subagent' && s.taskId === event.sub_task_id
            )
            if (subIdx < 0) {
              return {
                ...turn,
                segments: [...turn.segments, {
                  ...emptySegment(`subagent_${event.sub_task_id}`),
                  type: 'subagent',
                  status: 'streaming',
                  agentType: 'review' as const,
                  taskId: event.sub_task_id,
                  segments: [{
                    ...emptySegment(`comp_${++counterRef.current}`),
                    type: 'compression',
                    compressionPhase: phase,
                  }],
                }],
              }
            }
            const subSeg = { ...turn.segments[subIdx] }
            if (!subSeg.segments) subSeg.segments = []
            const subSegs = [...subSeg.segments]
            const compIdx = subSegs.findIndex(s => s.type === 'compression')
            if (compIdx >= 0) {
              subSegs[compIdx] = { ...subSegs[compIdx], compressionPhase: phase }
            } else {
              subSegs.push({
                ...emptySegment(`comp_${++counterRef.current}`),
                type: 'compression',
                compressionPhase: phase,
              })
            }
            subSeg.segments = subSegs
            const newSegs = [...turn.segments]
            newSegs[subIdx] = subSeg
            return { ...turn, segments: newSegs }
          }))
          return
        }
        setTurns(prev => prev.map(turn => {
          if (turn.turnId !== turnId) return turn
          const compIdx = turn.segments.findIndex(s => s.type === 'compression')
          if (compIdx >= 0) {
            const segs = [...turn.segments]
            segs[compIdx] = { ...segs[compIdx], compressionPhase: phase }
            return { ...turn, segments: segs }
          }
          return {
            ...turn,
            segments: [...turn.segments, {
              ...emptySegment(`comp_${++counterRef.current}`),
              type: 'compression' as const,
              compressionPhase: phase,
            }],
          }
        }))
        return
      }
      case AgentEventType.PhaseGate: {
        if (event.phase_gate) {
          onPhaseGate?.(event.phase_gate)
        }
        return
      }
    }

    setTurns(prev => prev.map(turn => {
      if (turn.turnId !== turnId) return turn

      // 子 Agent 事件：按 sub_task_id 路由到对应 SubagentSegment
      if (event.sub_task_id) {
        let subIdx = turn.segments.findIndex(s =>
          s.type === 'subagent' && s.taskId === event.sub_task_id
        )
        let updatedSegments = turn.segments
        if (subIdx < 0) {
          // run_subagent 的 ToolCall 事件还没 apply，子 Agent 事件先到了——就地创建
          const newSeg = {
            ...emptySegment(`subagent_${event.sub_task_id}`),
            type: 'subagent' as const,
            status: 'streaming' as const,
            agentType: 'memory' as const,
            taskId: event.sub_task_id,
            segments: [],
            finalText: '',
            toolStatus: 'executing' as const,
          }
          updatedSegments = [...turn.segments, newSeg]
          subIdx = updatedSegments.length - 1
        }
        const subSeg = { ...updatedSegments[subIdx] }
        if (!subSeg.segments) subSeg.segments = []
        const subSegs = [...subSeg.segments]
        const subSegId = `subseg_${++counterRef.current}`

        switch (event.type) {
          case AgentEventType.Thinking: {
            const chunk = event.data || ''
            const last = subSegs[subSegs.length - 1]
            if (last && last.type === 'text' && last.isStreaming) {
              subSegs[subSegs.length - 1] = { ...last, thinkingContent: last.thinkingContent + chunk }
            } else {
              subSegs.push({ ...emptySegment(subSegId), thinkingContent: chunk, thinkingDone: false, isStreaming: true })
            }
            break
          }
          case AgentEventType.ThinkingDone: {
            for (let i = 0; i < subSegs.length; i++) {
              if (subSegs[i].type === 'text' && !subSegs[i].thinkingDone) {
                subSegs[i] = { ...subSegs[i], thinkingDone: true, isStreaming: false }
              }
            }
            break
          }
          case AgentEventType.Content: {
            const chunk = event.data || ''
            const last = subSegs[subSegs.length - 1]
            if (last && last.type === 'text' && last.isStreaming) {
              subSegs[subSegs.length - 1] = { ...last, content: last.content + chunk, thinkingDone: true }
            } else {
              subSegs.push({ ...emptySegment(subSegId), content: chunk, thinkingDone: true, isStreaming: true })
            }
            break
          }
          case AgentEventType.ToolCall: {
            const subToolStatus = event.phase === 'completed' ? 'completed' as const
              : event.phase === 'failed' ? 'failed' as const
              : 'executing' as const
            const stIdx = subSegs.findIndex(s =>
              s.type === 'tool' && s.toolId === event.tool_id
            )
            if (stIdx >= 0) {
              subSegs[stIdx] = {
                ...subSegs[stIdx],
                toolStatus: subToolStatus,
                displayText: event.display_text || subSegs[stIdx].displayText,
                activityKind: event.activity_kind || '',
                error: event.error || '',
              }
            } else {
              subSegs.push({
                ...emptySegment(subSegId),
                type: 'tool',
                toolName: event.tool_name || '',
                toolId: event.tool_id || '',
                toolStatus: subToolStatus,
                displayText: event.display_text || event.tool_name || '',
                activityKind: event.activity_kind || '',
                error: event.error || '',
              })
            }
            break
          }
          default:
            break
        }

        subSeg.segments = subSegs
        const newSegs = [...updatedSegments]
        newSegs[subIdx] = subSeg
        return { ...turn, segments: newSegs }
      }

      const segments = [...turn.segments]
      const segId = `seg_${++counterRef.current}`

      switch (event.type) {
        case AgentEventType.Thinking: {
          const chunk = event.data || ''
          const lastSeg = segments[segments.length - 1]
          if (lastSeg && lastSeg.type === 'text' && lastSeg.isStreaming) {
            segments[segments.length - 1] = {
              ...lastSeg,
              thinkingContent: lastSeg.thinkingContent + chunk,
            }
          } else {
            segments.push({
              ...emptySegment(segId),
              thinkingContent: chunk,
              thinkingDone: false,
              isStreaming: true,
            })
          }
          return { ...turn, segments }
        }

        case AgentEventType.ThinkingDone: {
          return {
            ...turn,
            segments: segments.map(seg =>
              seg.type === 'text' && !seg.thinkingDone
                ? { ...seg, thinkingDone: true, isStreaming: false }
                : seg
            ),
          }
        }

        case AgentEventType.Content: {
          const chunk = event.data || ''
          const lastSeg = segments[segments.length - 1]
          if (lastSeg && lastSeg.type === 'text' && lastSeg.isStreaming) {
            segments[segments.length - 1] = {
              ...lastSeg,
              content: lastSeg.content + chunk,
              thinkingDone: true,
            }
          } else {
            segments.push({
              ...emptySegment(segId),
              content: chunk,
              thinkingDone: true,
              isStreaming: true,
            })
          }
          return { ...turn, segments }
        }

        case AgentEventType.ToolCall: {
          // 门禁阶段推进（set_phase）不在工具调用详情中显示
          if (event.tool_name === 'set_phase') {
            return { ...turn, segments }
          }
          const isSubagent = event.tool_name === 'run_subagent'
          const toolStatus =
            event.phase === 'awaiting_approval' ? 'awaiting_approval' as const
            : event.phase === 'completed' ? 'completed' as const
            : event.phase === 'failed' ? 'failed' as const
            : 'executing' as const

          // run_subagent：维护对应的 subagent segment
          if (isSubagent) {
            const agentType = (event.metadata?.agent_type as 'memory' | 'review') || 'memory'
            const toolId = event.tool_id || ''
            const subIdx = segments.findIndex(seg =>
              seg.type === 'subagent' && seg.taskId === toolId
            )
            if (subIdx >= 0) {
              segments[subIdx] = {
                ...segments[subIdx],
                agentType,
                status: toolStatus === 'executing' ? 'streaming' : toolStatus === 'failed' ? 'failed' : 'done',
                toolStatus,
              }
            } else {
              segments.push({
                ...emptySegment(`subagent_${toolId || segId}`),
                type: 'subagent',
                status: 'streaming',
                agentType,
                taskId: toolId,
                segments: [],
                finalText: '',
                toolStatus: 'executing',
              })
            }
            // 移除同 toolId 的 tool segment（可能由空 toolName 的早期事件误创建）
            const cleanSegs = toolId
              ? segments.filter(seg => !(seg.type === 'tool' && seg.toolId === toolId))
              : segments
            return { ...turn, segments: cleanSegs }
          }

          const idx = segments.findIndex(seg =>
            seg.type === 'tool' && event.tool_id && seg.toolId === event.tool_id
          )

          const approvalType = toolStatus === 'awaiting_approval'
            ? (event.metadata?.approval_type as string | undefined)
            : undefined
          const approvalPayload = toolStatus === 'awaiting_approval'
            ? (event.metadata?.payload as Record<string, unknown> | undefined)
            : undefined

          if (idx >= 0) {
            segments[idx] = {
              ...segments[idx],
              toolName: event.tool_name || segments[idx].toolName,
              toolId: event.tool_id || segments[idx].toolId,
              toolStatus,
              displayText: event.display_text || segments[idx].displayText,
              activityKind: event.activity_kind || segments[idx].activityKind || '',
              error: event.error || '',
              approvalType: approvalType ?? segments[idx].approvalType,
              approvalPayload: approvalPayload ?? segments[idx].approvalPayload,
              // 工具返回结果（后端 EventToolCall completed/failed 推送 tool_result）
              result: (toolStatus === 'completed' || toolStatus === 'failed') ? (event.tool_result || segments[idx].result) : segments[idx].result,
            }
          } else {
            segments.push({
              ...emptySegment(segId),
              type: 'tool',
              toolName: event.tool_name || '',
              toolId: event.tool_id || '',
              toolStatus,
              displayText: event.display_text || event.tool_name || '',
              activityKind: event.activity_kind || '',
              error: event.error || '',
              approvalType,
              approvalPayload,
              result: (toolStatus === 'completed' || toolStatus === 'failed') ? event.tool_result : undefined,
            })
          }

          // 文件编辑审批 → 通知 ContentPanel 打开 diff 标签页
          if (toolStatus === 'awaiting_approval' && approvalType === 'file_edit' && approvalPayload) {
            const p = approvalPayload
            const path = (p.path as string) || ''
            let title = `diff: ${path}`
            if (path.startsWith('chapters/')) {
              const num = path.replace('chapters/', '').replace('.md', '')
              title = `diff: ${t('chat.diffChapter', { n: parseInt(num) })}`
            } else if (path === 'goink.md') {
              title = `diff: ${t('chat.diffStoryStatus')}`
            } else if (path.startsWith('outlines/')) {
              const num = path.replace('outlines/', '').replace('.md', '')
              title = `diff: ${t('chat.diffChapterOutline', { n: parseInt(num) })}`
            }
            onApprovalFileEditRef.current?.({
              path,
              title,
              diff: '',
              original: (p.original as string) || '',
              modified: (p.modified as string) || '',
              changeType: (p.change_type as string) || '',
              reason: (p.reason as string) || '',
              toolId: (event.tool_id as string) || '',
            })
          }

          return { ...turn, segments }
        }

        default:
          return turn
      }
    }))
  }, [t])

  const flushEventQueue = useCallback((turnId: number, force = false) => {
    const queue = eventQueuesRef.current.get(turnId)
    if (!queue) return

    let event = queue.pending.get(queue.nextSeq)
    while (event) {
      queue.pending.delete(queue.nextSeq)
      queue.nextSeq += 1
      applyAgentEvent(turnId, event)
      event = queue.pending.get(queue.nextSeq)
    }

    if (force && queue.pending.size > 0) {
      const orderedEvents = [...queue.pending.entries()].sort(([a], [b]) => a - b)
      queue.pending.clear()

      for (const [seq, queuedEvent] of orderedEvents) {
        if (seq >= queue.nextSeq) {
          queue.nextSeq = seq + 1
          applyAgentEvent(turnId, queuedEvent)
        }
      }
    }

    if (queue.pending.size === 0 && queue.flushTimer) {
      clearTimeout(queue.flushTimer)
      queue.flushTimer = null
    }
  }, [applyAgentEvent])

  const handleAgentEvent = useCallback((turnId: number) => (event: AgentEvent) => {
    if (!event.seq) {
      applyAgentEvent(turnId, event)
      return
    }

    let queue = eventQueuesRef.current.get(turnId)
    if (!queue) {
      queue = {
        nextSeq: 1,
        pending: new Map<number, AgentEvent>(),
        flushTimer: null,
      }
      eventQueuesRef.current.set(turnId, queue)
    }

    if (event.seq < queue.nextSeq) return

    queue.pending.set(event.seq, event)
    flushEventQueue(turnId)

    if (queue.pending.size > 0 && !queue.flushTimer) {
      queue.flushTimer = setTimeout(() => {
        queue.flushTimer = null
        flushEventQueue(turnId, true)
      }, EVENT_REORDER_TIMEOUT)
    }
  }, [applyAgentEvent, flushEventQueue])

  const handleConfigModel = useCallback(() => setShowSettings(true), [])

  const refreshModels = useCallback(() => {
    app.GetModels().then(list => {
      if (list && list.length > 0) setModels(list)
    }).catch(() => {})
  }, [app])

  const handleSelectModel = useCallback((key: string) => {
    setSelectedKey(key)
    const m = models.find(x => x.Key === key)
    let effort = ''
    if (m?.ReasoningLevels?.length) {
      effort = m.ReasoningLevels[0]
      setReasoningEffort(effort)
    }
    app.SetSelectedModel(key, effort).catch(() => {})
  }, [models, app])

  const handleToggleApproval = useCallback(() => {
    const next = approvalMode === 'manual' ? 'auto' : 'manual'
    setApprovalMode(next)
    app.SetApprovalMode(next).catch(() => {})
  }, [approvalMode, app])

  // 监听审批模式变更事件（移动端切换时桌面端实时更新）
  useEffect(() => {
    const cleanup = EventsOn('settings:approval_mode_changed', (data: { mode?: string }) => {
      if (data.mode === 'auto' || data.mode === 'manual') {
        setApprovalMode(data.mode)
      }
    })
    return () => { cleanup() }
  }, [])

  const handleSelectEffort = useCallback((effort: string) => {
    setReasoningEffort(effort)
    const enabled = effort !== ''
    setThinkingEnabled(enabled)
    app.SetReasoningEffort(effort).catch(() => {})
    app.SetThinkingEnabled(enabled).catch(() => {})
  }, [app])

  const handleCompress = useCallback(async () => {
    if (!sessionId || !selectedKey || compressingRef.current) return
    const [providerName, modelID] = splitModelKey(selectedKey)
    if (!providerName || !modelID) return

    compressingRef.current = true
    // 创建压缩中 turn（用于动画展示）
    const compTurnId = `comp_${++counterRef.current}`
    const compressingTurn: Turn = {
      id: compTurnId,
      turnId: 0,
      userMessage: '',
      segments: [{
        ...emptySegment(compTurnId),
        type: 'compression' as const,
        compressionPhase: 'compressing' as const,
      }],
      status: 'done' as const,
      compressionOnly: true,
    }
    setTurns(prev => [...prev, compressingTurn])

    try {
      const result = await app.CompressContext({
        session_id: sessionId,
        provider_name: providerName,
        model_id: modelID,
      })
      // 更新：回填真实 turnId + 完成状态
      setTurns(prev => prev.map(t => {
        if (t.id === compTurnId) {
          return {
            ...t,
            turnId: result.turn_id,
            segments: t.segments.map(s => s.type === 'compression' ? { ...s, compressionPhase: 'done' as const } : s),
          }
        }
        return t
      }))
    } catch {
      // 压缩失败，移除 compressing turn
      setTurns(prev => prev.filter(t => t.id !== compTurnId))
    } finally {
      compressingRef.current = false
    }
  }, [sessionId, selectedKey, app])

  useImperativeHandle(ref, () => ({
    compress: () => { void handleCompress() },
  }), [handleCompress])

  const handleSend = useCallback(async (content: string) => {
    if (!selectedKey) return
    const [p, m] = splitModelKey(selectedKey)
    activeCountRef.current++
    if (activeCountRef.current > 1) {
      app.CancelChat(sessionId)
      // 中途插入对话：立即把正在 streaming 的旧 turn 标记为 stopped，
      // 否则旧 turn 永远停在 streaming（finally 只处理本次 turnId），
      // 事件流混乱导致后续消息不渲染
      setTurns(prev => prev.map(t =>
        t.status === 'streaming' ? { ...t, status: 'stopped' as const } : t
      ))
    }
    setIsLoading(true)

    const turnId = `turn_${++counterRef.current}`
    const newTurn: Turn = {
      id: turnId,
      turnId: 0,
      userMessage: content,
      segments: [],
      status: 'streaming',
    }

    // 如果是新对话，清除历史标记
    if (activeSessionId === null || activeSessionId === undefined) {
      setActiveSessionId(null)
    }

    setTurns(prev => [...prev, newTurn])

    // 监听 chat:started，拿到 turnId 后订阅 agent 事件流。
    // myGen 代际守卫：并发发送时旧发送的 finally 不许注销新发送的监听器（F1）
    const myGen = ++sendGenRef.current
    let myBackendTurnId: number | null = null
    startedUnsubRef.current?.()
    const startedCleanup = EventsOn('chat:started', (data: ChatStartedEvent) => {
      if (data.session_id) {
        setSessionId(data.session_id)
        setActiveSessionId(data.session_id)
        app.SetLastSession(data.session_id).catch(() => {})
      }

      // 更新 turn 的 turnId 为后端分配的真实值
      myBackendTurnId = data.turn_id
      setTurns(prev => prev.map(t =>
        t.id === turnId ? { ...t, turnId: data.turn_id } : t
      ))

      agentUnsubRef.current?.()
      // 事件名带 session_id：turn_id 按会话独立递增，并发会话会碰撞，
      // 不带 session 前缀时两个会话的事件都发到 agent:{turnID} 互相串扰
      const agentCleanup = EventsOn(`agent:${data.session_id}:${data.turn_id}`, handleAgentEvent(data.turn_id))
      agentUnsubRef.current = agentCleanup
    })
    startedUnsubRef.current = startedCleanup

    // 门禁模式快捷入口：批量模式且无活跃会话时，自动拼"批量写N章"前缀触发后端 batch 检测
    let finalMsg = content
    if (phaseMode && phaseMode !== 'single' && (activeSessionId === null || activeSessionId === undefined)) {
      const match = phaseMode.match(/^batch(\d+)$/)
      if (match) finalMsg = `批量写${match[1]}章：${content}`
    }

    try {
      await app.Chat({
        session_id: sessionId,
        novel_id: novelId,
        message: finalMsg,
        provider_name: p,
        model_id: m,
        reasoning_effort: reasoningEffort,
      })
    } catch (err) {
      const errMsg = err instanceof Error ? err.message : String(err)
      setTurns(prev => prev.map(t => {
        if (t.id !== turnId) return t
        if (t.status === 'stopped') return t
        return { ...t, status: 'failed' as const, errorMessage: errMsg }
      }))
    } finally {
      // 代际守卫：flush 积压事件与注销监听器只由最新一次发送执行。
      // 被新发送取代（中途插话/取消）的旧发送只丢弃自己的队列，
      // 不动共享 ref——旧实现此处会误删新发送的监听器导致新 turn 收不到事件
      const isLatest = sendGenRef.current === myGen
      if (isLatest) {
        eventQueuesRef.current.forEach((queue, queuedTurnId) => {
          if (queue.flushTimer) clearTimeout(queue.flushTimer)
          const orderedEvents = [...queue.pending.entries()].sort(([a], [b]) => a - b)
          queue.pending.clear()
          for (const [seq, queuedEvent] of orderedEvents) {
            if (seq >= queue.nextSeq) {
              queue.nextSeq = seq + 1
              applyAgentEvent(queuedTurnId, queuedEvent)
            }
          }
        })
        eventQueuesRef.current.clear()
        startedUnsubRef.current?.()
        startedUnsubRef.current = null
        agentUnsubRef.current?.()
        agentUnsubRef.current = null
      } else if (myBackendTurnId !== null) {
        const staleQueue = eventQueuesRef.current.get(myBackendTurnId)
        if (staleQueue?.flushTimer) clearTimeout(staleQueue.flushTimer)
        eventQueuesRef.current.delete(myBackendTurnId)
      }
      setTurns(prev => prev.map(t =>
        t.id === turnId && t.status === 'streaming'
          ? { ...t, status: 'done' as const, segments: t.segments.map(seg =>
              seg.type === 'text' ? { ...seg, isStreaming: false } : seg
            )}
          : t
      ))
      activeCountRef.current--
      if (activeCountRef.current === 0) {
        setIsLoading(false)
      }
    }
  }, [sessionId, novelId, selectedKey, reasoningEffort, app, handleAgentEvent, applyAgentEvent, activeSessionId])

  const handleRetry = useCallback((turnId: string) => {
    // 从镜像 ref 读取（StrictMode 下 updater 双调用，updater 内调 handleSend 会重复发送）
    const turn = turnsRef.current.find(t => t.id === turnId)
    if (!turn || !turn.userMessage) return
    setTurns(prev => prev.filter(t => t.id !== turnId))
    handleSend(turn.userMessage)
  }, [handleSend])

  const hasNovel = novelId > 0
  const hasTurns = turns.length > 0
  const hasActiveSession = activeSessionId !== undefined && activeSessionId !== null
  const showRecent = !hasActiveSession && !hasTurns && !isLoading
  // 工具卡片合并计数（每次渲染重建：连续同名同状态 tool 段 → 一张卡 ×N）
  const toolCounts = new Map<string, number>()
  const toolMergeIds = new Set<string>()
  const toolDetails = new Map<string, ToolCallDetail[]>()


  const inputPlaceholder = !hasNovel
    ? t('chat.selectNovelFirst')
    : !selectedKey
      ? t('chat.configureModelFirst')
      : t('chat.inputPlaceholder')

  return (
    <aside className="chat-panel shrink-0 h-full flex flex-col bg-sidebar border-l relative overflow-hidden" style={{ width: chatPanelWidth }}>
      <div
        className="absolute left-0 top-0 bottom-0 w-1 cursor-col-resize hover:bg-primary/30 transition-colors z-10 select-none"
        style={{ marginLeft: -2 }}
        onMouseDown={handleMouseDown}
      />

      <div className="px-4 py-2.5 border-b shrink-0 flex items-center justify-between select-none">
        <div className="flex items-center gap-2 min-w-0">
          <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider shrink-0">{t('chat.aiChat')}</span>
          {sessionTitle && <span className="text-xs text-foreground/60 truncate max-w-[140px]">· {sessionTitle}</span>}
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={handleOpenHistory}
            className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
          >
            <History className="w-3.5 h-3.5" /> {t('chat.history')}
          </button>
          <button
            onClick={handleNewChat}
            className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
          >
            <Plus className="w-3.5 h-3.5" /> {t('chat.newChat')}
          </button>
        </div>
      </div>

      {initLoadError && (
        <div className="px-4 py-2 bg-danger-bg border-b border-danger-border text-xs text-red-600 flex items-center justify-between shrink-0">
          <span>{t('chat.loadSettingsFailed')}</span>
          <button
            onClick={() => setInitLoadRetry(n => n + 1)}
            className="underline hover:text-destructive cursor-pointer"
          >
            {t('chat.retry')}
          </button>
        </div>
      )}

      <div className="absolute left-0 right-0 top-[41px] bottom-0 pointer-events-none z-30">
        <SessionHistory
          open={showHistoryPanel}
          novelId={novelId}
          activeSessionId={activeSessionId}
          onClose={handleCloseHistory}
          onSelectSession={handleSelectSession}
          onDeleted={(deletedIds) => {
            setIsLoading(false)
            setRetryInfo(null)
            setApiStreaming(false)
            // 删的是当前活跃会话：清空状态（handleNewChat 会拆除监听与队列），
            // 否则 sessionId 残留脏值，下次发送带已删会话 ID
            if (activeSessionId && deletedIds.includes(activeSessionId)) {
              handleNewChat()
            }
          }}
        />
      </div>

      {retryInfo && (        <RetryNotification
          retryCount={retryInfo.count}
          retryMax={retryInfo.max}
          retryWait={retryInfo.wait}
          onDone={clearRetryInfo}
        />
      )}

      <div ref={scrollContainerRef} onScroll={handleMessagesScroll} className="flex-1 overflow-y-auto overscroll-contain px-3 py-3 relative">
        {!hasNovel ? (
          <div className="flex items-center justify-center h-full">
            <div className="text-center">
              <MessageSquare className="w-10 h-10 text-muted-foreground/20 mx-auto mb-3" />
              <p className="text-sm text-muted-foreground">{t('chat.selectNovel')}</p>
            </div>
          </div>
        ) : showRecent ? (
          <div className="flex flex-col items-center justify-center h-full px-8 text-center">
            <MessageSquare className="w-12 h-12 text-muted-foreground/20 mb-4" />
            <h3 className="text-base font-medium text-foreground mb-2">{t('chat.welcomeTitle')}</h3>
            <p className="text-sm text-muted-foreground mb-6 max-w-sm leading-relaxed">{t('chat.welcomeDesc')}</p>
            <div className="grid grid-cols-1 gap-3 w-full max-w-sm">
              <div className="rounded-lg border bg-card p-4 text-left">
                <p className="text-xs font-medium text-foreground mb-1">📝 {t('chat.hintWrite')}</p>
                <p className="text-[11px] text-muted-foreground">{t('chat.hintWriteDesc')}</p>
              </div>
              <div className="rounded-lg border bg-card p-4 text-left">
                <p className="text-xs font-medium text-foreground mb-1">📖 {t('chat.hintCreate')}</p>
                <p className="text-[11px] text-muted-foreground">{t('chat.hintCreateDesc')}</p>
              </div>
              <div className="rounded-lg border bg-card p-4 text-left">
                <p className="text-xs font-medium text-foreground mb-1">🌍 {t('chat.hintWorld')}</p>
                <p className="text-[11px] text-muted-foreground">{t('chat.hintWorldDesc')}</p>
              </div>
              <div className="rounded-lg border bg-card p-4 text-left">
                <p className="text-xs font-medium text-foreground mb-1">🔍 {t('chat.hintSearch')}</p>
                <p className="text-[11px] text-muted-foreground">{t('chat.hintSearchDesc')}</p>
              </div>
            </div>
          </div>
        ) : isLoadingHistory ? (
          <div className="flex items-center justify-center h-full">
            <Loader2 className="w-5 h-5 animate-spin text-muted-foreground" />
          </div>
        ) : (
          <>
            {/* 消息列表 */}
            {historyLoadError ? (
              <div className="flex items-center justify-center h-full">
                <div className="text-center">
                  <p className="text-sm text-red-500 mb-2">{t('chat.loadMessagesFailed')}</p>
                  <button
                    onClick={() => { loadOnSessionRef.current = true; setHistoryLoadRetry(n => n + 1) }}
                    className="text-xs text-primary underline cursor-pointer"
                  >
                    {t('chat.retry')}
                  </button>
                </div>
              </div>
            ) : !hasTurns && !isLoading ? (
              <div className="flex items-center justify-center h-full">
                <div className="text-center">
                  <MessageSquare className="w-10 h-10 text-muted-foreground/20 mx-auto mb-3" />
                  <p className="text-sm text-muted-foreground">{t('chat.startConversation')}</p>
                </div>
              </div>
            ) : (
              <div className="space-y-4">
                {turns.length > visibleTurnCount && (
                  <button
                    onClick={() => setVisibleTurnCount(c => c + 50)}
                    className="w-full py-1.5 text-xs text-muted-foreground hover:text-foreground border border-dashed border-border rounded-md transition-colors cursor-pointer"
                  >
                    加载更早消息（{turns.length - visibleTurnCount} 轮）
                  </button>
                )}
                {turns.slice(-visibleTurnCount).map(turn => (
                  <div key={turn.id} className="space-y-2">
                    {turn.userMessage && (
                      <MessageBubble role="user" content={turn.userMessage} />
                    )}

                    {/* 工具卡片合并计数：连续同名同状态的普通工具段合并（并行调用去噪） */}
                    {(() => {
                      toolCounts.clear()
                      toolMergeIds.clear()
                      toolDetails.clear()
                      // lastHeadId 追踪当前连续串的可见头部段：A,A,A 序列的第 3 个段
                      // 的 prev 是已隐藏的 A2，计数必须累加到头部 A0（旧实现写到
                      // 隐藏段 id 上导致 ≥3 连段少报）
                      let lastHeadId: string | null = null
                      for (let i = 0; i < turn.segments.length; i++) {
                        const s = turn.segments[i]
                        if (s.type !== 'tool' || s.toolName === 'run_subagent' || s.toolName === 'web_search' || s.toolName === 'web_fetch') continue
                        const prev = i > 0 ? turn.segments[i - 1] : null
                        const mergeable = prev !== null && prev.type === 'tool'
                          && prev.toolName === s.toolName && prev.toolStatus === s.toolStatus
                          && prev.toolName !== 'run_subagent' && prev.toolName !== 'web_search' && prev.toolName !== 'web_fetch'
                        if (mergeable && lastHeadId) {
                          toolMergeIds.add(s.id)
                          toolCounts.set(lastHeadId, (toolCounts.get(lastHeadId) ?? 1) + 1)
                          const list = toolDetails.get(lastHeadId) ?? []
                          list.push({ displayText: s.displayText, status: s.toolStatus, activityKind: s.activityKind, error: s.error })
                          toolDetails.set(lastHeadId, list)
                        } else {
                          lastHeadId = s.id
                          toolCounts.set(s.id, 1)
                        }
                      }
                      return null
                    })()}

                    {turn.segments.map(seg => {
                      if (seg.type === 'subagent' && seg.agentType) {
                        return (
                          <SubagentCard
                            key={seg.id}
                            agentType={seg.agentType}
                            segments={seg.segments || []}
                            status={seg.status || 'done'}
                          />
                        )
                      }

                      // 工具卡片合并：连续同名同状态的普通工具段合并为一张卡
                      // （同一轮并行调用如 get_lore ×4，避免 4 张重复卡片堆叠）
                      if (seg.type === 'tool') {
                        if (toolMergeIds.has(seg.id)) return null
                        const count = toolCounts.get(seg.id) ?? 1
                        if (seg.toolName === 'run_subagent') return null

                        if (seg.toolName === 'web_search' && seg.toolStatus === 'completed' && seg.result) {
                          return <WebSearchCard key={seg.id} result={seg.result} />
                        }
                        if (seg.toolName === 'web_fetch' && seg.toolStatus === 'completed' && seg.result) {
                          return <WebFetchCard key={seg.id} result={seg.result} displayText={seg.displayText} />
                        }

                        return (
                          <ToolCallCard
                            key={seg.id}
                            toolName={seg.toolName}
                            displayText={seg.displayText}
                            status={seg.toolStatus}
                            activityKind={seg.activityKind}
                            error={seg.error}
                            result={seg.result}
                            count={count}
                            details={toolDetails.get(seg.id)}
                            approvalType={seg.approvalType}
                            approvalPayload={seg.approvalPayload}
                            onApprove={
                              seg.toolStatus === 'awaiting_approval'
                                ? (feedback: string) => onApprove(seg.toolId, feedback)
                                : undefined
                            }
                            onReject={
                              seg.toolStatus === 'awaiting_approval'
                                ? (feedback: string) => onReject(seg.toolId, feedback)
                                : undefined
                            }
                          />
                        )
                      }

                      if (seg.type === 'compression') {
                        return (
                          <CompressionBlock
                            key={seg.id}
                            phase={seg.compressionPhase || 'compressing'}
                          />
                        )
                      }

                      return (
                        <div key={seg.id}>
                          {seg.thinkingContent && (
                            <div className="max-w-[85%]">
                              <ThinkingBlock
                                content={seg.thinkingContent}
                                isStreaming={!seg.thinkingDone && seg.isStreaming}
                              />
                            </div>
                          )}
                          {seg.content && (
                            <MessageBubble
                              role="assistant"
                              content={seg.content}
                              onRetry={turn.status === 'failed' ? () => handleRetry(turn.id) : undefined}
                            />
                          )}
                        </div>
                      )
                    })}

                    {turn.status === 'failed' && turn.errorMessage && (
                      <div className="flex justify-start">
                        <div className="bg-danger-bg border border-danger-border rounded-lg px-3 py-2 text-xs text-red-600 max-w-[80%] flex items-center gap-2">
                          <span className="flex-1">{turn.errorMessage}</span>
                          <button
                            onClick={() => handleRetry(turn.id)}
                            className="shrink-0 px-2 py-0.5 rounded bg-red-100 dark:bg-red-900/30 hover:bg-red-200 dark:hover:bg-red-900/50 text-red-700 dark:text-red-400 font-medium transition-colors cursor-pointer"
                          >
                            重试
                          </button>
                        </div>
                      </div>
                    )}
                    {turn.status === 'interrupted' && (
                      <div className="flex justify-center">
                        <div className="bg-danger-bg border border-danger-border rounded-lg px-3 py-2 text-xs text-red-500 max-w-[80%]">
                          {t('chat.chatInterrupted')}
                        </div>
                      </div>
                    )}
                    {turn.status === 'stopped' && (
                      <div className="flex justify-center">
                        <div className="bg-muted/50 border rounded-lg px-3 py-2 text-xs text-muted-foreground max-w-[80%]">
                          {t('chat.chatStopped')}
                        </div>
                      </div>
                    )}
                    {turn.status === 'streaming' && turn.segments.length === 0 && (
                      <div className="flex justify-start">
                        <div className="bg-muted rounded-lg rounded-bl-sm px-3 py-2">
                          <Loader2 className="w-4 h-4 animate-spin text-muted-foreground" />
                        </div>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}
          </>
        )}

        <div ref={messagesEndRef} />

        {/* 跳到底部按钮：贴右下角、不遮挡消息内容（sticky 容器占满宽，内容左对齐） */}
        {showScrollBtn && (
          <div className="sticky bottom-2 z-10 flex justify-end pointer-events-none">
            <button
              onClick={scrollToBottom}
              className="pointer-events-auto flex items-center gap-1 px-2.5 py-1 rounded-full bg-popover/80 border border-border/50 shadow-md text-xs text-muted-foreground hover:text-foreground transition-all cursor-pointer backdrop-blur-sm"
            >
              <ArrowDown className="w-3 h-3" />
              <span>底部</span>
            </button>
          </div>
        )}
      </div>

      <ChatInput
        disabled={!hasNovel || !selectedKey}
        isLoading={isLoading || apiStreaming}
        placeholder={inputPlaceholder}
        slashItems={slashCommands}
        onSend={handleSend}
        onListSlash={loadSlash}
        onStop={() => {
          setTurns(prev => prev.map(t =>
            t.status === 'streaming'
              ? { ...t, status: 'stopped' as const }
              : t
          ))
          setIsLoading(false)
          setRetryInfo(null)
          setApiStreaming(false)
          // 移动端对话进行中时，取消移动端的会话；否则取消桌面端当前会话
          app.CancelChat(apiActiveRef.current.sessionId || sessionId)
        }}
        controls={
          <ChatControls
            models={models}
            selectedKey={selectedKey}
            onSelectModel={handleSelectModel}
            onRefreshModels={refreshModels}
            reasoningEffort={reasoningEffort}
            onSelectEffort={handleSelectEffort}
            thinkingEnabled={thinkingEnabled}
            approvalMode={approvalMode}
            onToggleApproval={handleToggleApproval}
            onConfigModel={handleConfigModel}
            embedded
          />
        }
      />

      {isDragging && (
        <div className="fixed inset-0 z-50 cursor-col-resize select-none" />
      )}

      <SettingsDialog
        open={showSettings}
        onClose={() => setShowSettings(false)}
        onSaved={() => {
          app.GetModels().then(list => {
            if (list && list.length > 0) {
              setModels(list)
              if (!list.find(m => m.Key === selectedKey)) {
                setSelectedKey(list[0].Key)
                if (list[0].ReasoningLevels?.length) {
                  setReasoningEffort(list[0].ReasoningLevels[0])
                }
              }
            }
          }).catch(() => {})
        }}
        initialTab="model"
      />
    </aside>
  )
})
