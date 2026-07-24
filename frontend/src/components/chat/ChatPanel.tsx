import { useState, useCallback, useRef, useEffect } from 'react'
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
import ToolCallCard from './ToolCallCard'
import WebSearchCard from './WebSearchCard'
import WebFetchCard from './WebFetchCard'
import SubagentCard from './SubagentCard'
import CompressionBlock from './CompressionBlock'
import PhaseGateBar from './PhaseGateBar'
import RetryNotification from './RetryNotification'
import type { UsageInfo } from './ContextRing'
import SettingsDialog from '@/components/settings/SettingsDialog'
import RecentSessions from './RecentSessions'
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

export default function ChatPanel({ novelId, onApprove, onReject, onApprovalFileEdit, chatPanelWidth, onChatPanelResize }: Props) {
  const { t } = useTranslation()
  const app = useApp()

  // 从 model key 中安全提取 provider 和 modelID
  // key 格式：providerName/modelID（如 "deepseek/deepseek-v4-pro"）
  // 注意：modelID 可能包含 "/"，必须只在第一个 "/" 处拆分
  const splitModelKey = (key: string): [string, string] => {
    const idx = key.indexOf('/')
    return idx >= 0 ? [key.substring(0, idx), key.substring(idx + 1)] : ['', key]
  }

  const [isDragging, setIsDragging] = useState(false)
  const startXRef = useRef(0)
  const startWidthRef = useRef(chatPanelWidth)
  const [turns, setTurns] = useState<Turn[]>([])
  const [sessionId, setSessionId] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [models, setModels] = useState<llm.AvailableModel[]>([])
  const [selectedKey, setSelectedKey] = useState('')
  const [reasoningEffort, setReasoningEffort] = useState('')
  const [approvalMode, setApprovalMode] = useState<'manual' | 'auto'>('manual')
  const [thinkingEnabled, setThinkingEnabled] = useState(false)
  const [lastUsage, setLastUsage] = useState<UsageInfo | null>(null)
  const [isCompressing, setIsCompressing] = useState(false)
  const compressingRef = useRef(false)
  const activeCountRef = useRef(0)
  const [showSettings, setShowSettings] = useState(false)
  const [activeSessionId, setActiveSessionId] = useState<string | null | undefined>(undefined)
  const [sessions, setSessions] = useState<app.SessionMeta[]>([])
  const [sessionsTotal, setSessionsTotal] = useState(0)
  const [showHistoryPanel, setShowHistoryPanel] = useState(false)
  const [isLoadingHistory, setIsLoadingHistory] = useState(false)
  const [initLoadError, setInitLoadError] = useState(false)
  const [initLoadRetry, setInitLoadRetry] = useState(0)
  const [historyLoadError, setHistoryLoadError] = useState(false)
  const [historyLoadRetry, setHistoryLoadRetry] = useState(0)
  const [slashCommands, setSlashCommands] = useState<app.SlashCommand[]>([])
  const [phaseGateStatus, setPhaseGateStatus] = useState<import('./types').PhaseStatus | null>(null)
  const [phaseGateError, setPhaseGateError] = useState<string>('')
  const [retryInfo, setRetryInfo] = useState<{ count: number; max: number; wait: number } | null>(null)
  const [showScrollBtn, setShowScrollBtn] = useState(false)
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const scrollContainerRef = useRef<HTMLDivElement>(null)
  const isNearBottomRef = useRef(true)
  const counterRef = useRef(0)
  const startedUnsubRef = useRef<(() => void) | null>(null)
  const agentUnsubRef = useRef<(() => void) | null>(null)
  const eventQueuesRef = useRef<Map<number, EventQueue>>(new Map())
  const onApprovalFileEditRef = useRef(onApprovalFileEdit)
  useEffect(() => { onApprovalFileEditRef.current = onApprovalFileEdit }, [onApprovalFileEdit])
  const lastSessionIdRef = useRef('')

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
        if (!effort || !model.ReasoningLevels?.includes(effort)) {
          effort = model.ReasoningLevels?.[0] || ''
        }
        setReasoningEffort(effort)

        // 恢复思考模式
        setThinkingEnabled(effort !== '' && (model?.SupportsThinking ?? false))
      }

      // 恢复审批模式
      const mode = settings?.approval_mode
      if (mode === 'manual' || mode === 'auto') {
        setApprovalMode(mode)
      }

      // 暂存上次会话 ID，等 novelId 加载后恢复
      if (settings?.last_session_id) {
        lastSessionIdRef.current = settings.last_session_id
      }
    }).catch((err) => {
      console.error('Load models/settings failed', err)
      setInitLoadError(true)
    })
  }, [app, initLoadRetry])

  // 加载会话列表
  useEffect(() => {
    if (!novelId) return
    setActiveSessionId(undefined)
    setTurns([])
    setSessionId('')
    app.GetSessions({ novel_id: novelId, page: 1, size: 5, search: '' }).then(r => {
      if (r) {
        setSessions(r.items)
        setSessionsTotal(r.total)
      }
    }).catch((err) => {
      console.error('Load sessions failed', err)
    })

    // 尝试恢复上次活跃会话（仅恢复一次，通过 ref 标记）
    const sid = lastSessionIdRef.current
    if (sid && novelId) {
      lastSessionIdRef.current = ''
      app.GetSession(sid).then(detail => {
        if (detail && detail.novel_id === novelId) {
          setActiveSessionId(sid)
        }
      }).catch(() => {
        app.SetLastSession('').catch(() => {})
      })
    }
  }, [app, novelId])

  // 监听移动端对话完成事件，自动刷新会话列表
  useEffect(() => {
    const cleanup = EventsOn('chat:api_done', (data: { session_id: string }) => {
      if (!novelId) return
      // 刷新会话列表
      app.GetSessions({ novel_id: novelId, page: 1, size: 5, search: '' }).then(r => {
        if (r) setSessions(r.items)
      }).catch(() => {})
      // 如果当前打开的会话被更新，刷新消息
      if (activeSessionId === data.session_id) {
        app.GetSessionMessages(data.session_id).then(msgs => {
          if (msgs) setTurns(rebuildTurns(msgs))
        }).catch(() => {})
      }
    })
    return () => { cleanup() }
  }, [app, novelId, activeSessionId])

  // 监听移动端对话实时流事件，实现双端同步
  useEffect(() => {
    type ApiEvent = {
      type: string
      turn_id: number
      data?: string
      error?: string
      tool_name?: string
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
        apiStreamRef.content = ''
        apiStreamRef.thinking = ''
        apiStreamRef.toolName = ''
        setTurns(prev => {
          const newTurn: Turn = {
            id: `api-${ev.turn_id}`,
            turnId: ev.turn_id,
            userMessage: '',
            segments: [],
            status: 'streaming',
          }
          return [...prev, newTurn]
        })
        return
      }
      // 更新最后一个 streaming turn 的 segments
      setTurns(prev => {
        if (prev.length === 0) return prev
        const last = prev[prev.length - 1]
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
        if (apiStreamRef.content.length > 0 || apiStreamRef.thinking.length > 0) {
          segments.push({
            id: `api-text-${Date.now()}`,
            type: 'text',
            content: apiStreamRef.content,
            thinkingContent: apiStreamRef.thinking,
            thinkingDone: ev.type === 'done',
            isStreaming: ev.type === 'content',
            toolName: '', toolId: '', toolStatus: 'completed',
            displayText: '', activityKind: '', error: '',
          })
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

        // 错误段
        if (ev.type === 'error') {
          segments.push({
            id: `api-err-${Date.now()}`,
            type: 'text',
            content: '',
            thinkingContent: '', thinkingDone: false, isStreaming: false,
            toolName: '', toolId: '', toolStatus: 'completed',
            displayText: '', activityKind: '', error: ev.error || '未知错误',
          })
        }

        return prev.map((t, i) => i === prev.length - 1 ? { ...t, segments } : t)
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

  // 加载历史消息
  useEffect(() => {
    if (!activeSessionId || !novelId) return
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

  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    setIsDragging(true)
    startXRef.current = e.clientX
    startWidthRef.current = chatPanelWidth
  }, [chatPanelWidth])

  useEffect(() => {
    if (!isDragging) return
    const handleMouseMove = (e: MouseEvent) => {
      const delta = e.clientX - startXRef.current
      onChatPanelResize(startWidthRef.current - delta)
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
    setActiveSessionId(sid)
    app.SetLastSession(sid).catch(() => {})
    app.GetSession(sid).then(detail => {
      if (detail?.usage) {
        setLastUsage(detail.usage as unknown as UsageInfo)
      } else {
        setLastUsage(null)
      }
    }).catch(() => setLastUsage(null))
  }, [app])

  const handleNewChat = useCallback(() => {
    setActiveSessionId(null)
    setTurns([])
    setSessionId('')
    setLastUsage(null)
    app.GetSessions({ novel_id: novelId, page: 1, size: 5, search: '' }).then(r => {
      if (r) { setSessions(r.items); setSessionsTotal(r.total) }
    }).catch((err) => {
      console.error('Refresh sessions failed', err)
    })
  }, [novelId, app])

  const handleOpenHistory = useCallback(() => {
    setShowHistoryPanel(true)
  }, [])

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
          setLastUsage(event.usage as unknown as UsageInfo)
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
          max: event.retry_max || 3,
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
          setPhaseGateStatus(event.phase_gate)
          setPhaseGateError(event.error || '')
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
              result: toolStatus === 'completed' ? (event.metadata || segments[idx].result) : segments[idx].result,
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
              result: toolStatus === 'completed' ? event.metadata : undefined,
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

  const handleToggleThinking = useCallback(() => {
    const next = !thinkingEnabled
    setThinkingEnabled(next)
    if (!next) {
      setReasoningEffort('')
      app.SetReasoningEffort('').catch(() => {})
    } else {
      const m = models.find(x => x.Key === selectedKey)
      const defaultEffort = m?.ReasoningLevels?.[0] || 'high'
      setReasoningEffort(defaultEffort)
      app.SetReasoningEffort(defaultEffort).catch(() => {})
    }
  }, [thinkingEnabled, models, selectedKey, app])

  const handleSelectEffort = useCallback((effort: string) => {
    setReasoningEffort(effort)
    setThinkingEnabled(effort !== '')
    app.SetReasoningEffort(effort).catch(() => {})
  }, [app])

  const handleCompress = useCallback(async () => {
    if (!sessionId || !selectedKey || compressingRef.current) return
    const [providerName, modelID] = splitModelKey(selectedKey)
    if (!providerName || !modelID) return

    compressingRef.current = true
    setIsCompressing(true)
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
      setIsCompressing(false)
      compressingRef.current = false
    }
  }, [sessionId, selectedKey, app])

  const handleSend = useCallback(async (content: string) => {
    if (!selectedKey) return
    const [p, m] = splitModelKey(selectedKey)
    activeCountRef.current++
    if (activeCountRef.current > 1) {
      app.CancelChat(sessionId)
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

    // 监听 chat:started，拿到 turnId 后订阅 agent 事件流
    startedUnsubRef.current?.()
    const startedCleanup = EventsOn('chat:started', (data: ChatStartedEvent) => {
      if (data.session_id) {
        setSessionId(data.session_id)
        setActiveSessionId(data.session_id)
        app.SetLastSession(data.session_id).catch(() => {})
      }

      // 更新 turn 的 turnId 为后端分配的真实值
      setTurns(prev => prev.map(t =>
        t.id === turnId ? { ...t, turnId: data.turn_id } : t
      ))

      agentUnsubRef.current?.()
      const agentCleanup = EventsOn(`agent:${data.turn_id}`, handleAgentEvent(data.turn_id))
      agentUnsubRef.current = agentCleanup
    })
    startedUnsubRef.current = startedCleanup

    try {
      await app.Chat({
        session_id: sessionId,
        novel_id: novelId,
        message: content,
        provider_name: p,
        model_id: m,
        reasoning_effort: reasoningEffort,
      })
      // 刷新会话列表
      app.GetSessions({ novel_id: novelId, page: 1, size: 5, search: '' }).then(r => {
        if (r) { setSessions(r.items); setSessionsTotal(r.total) }
      }).catch((err) => {
        console.error('Post-send refresh sessions failed', err)
      })
    } catch (err) {
      const errMsg = err instanceof Error ? err.message : String(err)
      setTurns(prev => prev.map(t => {
        if (t.id !== turnId) return t
        if (t.status === 'stopped') return t
        return { ...t, status: 'failed' as const, errorMessage: errMsg }
      }))
    } finally {
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
      startedUnsubRef.current?.()
      startedUnsubRef.current = null
      agentUnsubRef.current?.()
      agentUnsubRef.current = null
    }
  }, [sessionId, novelId, selectedKey, reasoningEffort, app, handleAgentEvent, applyAgentEvent, activeSessionId])

  const handleRetry = useCallback((turnId: string) => {
    setTurns(prev => {
      const turn = prev.find(t => t.id === turnId)
      if (!turn || !turn.userMessage) return prev
      handleSend(turn.userMessage)
      return prev.filter(t => t.id !== turnId)
    })
  }, [handleSend])

  const hasNovel = novelId > 0
  const hasTurns = turns.length > 0
  const hasActiveSession = activeSessionId !== undefined && activeSessionId !== null
  const showRecent = !hasActiveSession && !hasTurns && !isLoading


  const inputPlaceholder = !hasNovel
    ? t('chat.selectNovelFirst')
    : !selectedKey
      ? t('chat.configureModelFirst')
      : t('chat.inputPlaceholder')

  return (
    <aside className="shrink-0 flex flex-col bg-sidebar border-l relative overflow-hidden" style={{ width: chatPanelWidth }}>
      <div
        className="absolute left-0 top-0 bottom-0 w-1 cursor-col-resize hover:bg-primary/30 transition-colors z-10 select-none"
        style={{ marginLeft: -2 }}
        onMouseDown={handleMouseDown}
      />

      <div className="px-4 py-2.5 border-b shrink-0 flex items-center justify-between select-none">
        <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">{t('chat.aiChat')}</span>
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
          onClose={handleCloseHistory}
          onSelectSession={handleSelectSession}
        />
      </div>

      <PhaseGateBar status={phaseGateStatus} error={phaseGateError} />

      {retryInfo && (
        <RetryNotification
          retryCount={retryInfo.count}
          retryMax={retryInfo.max}
          retryWait={retryInfo.wait}
          onDone={() => setRetryInfo(null)}
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
          <RecentSessions
            sessions={sessions}
            total={sessionsTotal}
            onSelectSession={handleSelectSession}
            onViewAll={handleOpenHistory}
          />
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
                    onClick={() => setHistoryLoadRetry(n => n + 1)}
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
                {turns.map(turn => (
                  <div key={turn.id} className="space-y-2">
                    {turn.userMessage && (
                      <MessageBubble role="user" content={turn.userMessage} timestamp={turn.id} />
                    )}

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

                      if (seg.type === 'tool') {
                        // run_subagent 已由 subagent 段渲染，跳过纯工具卡
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

        {/* 跳到底部按钮 */}
        {showScrollBtn && (
          <button
            onClick={scrollToBottom}
            className="sticky bottom-2 left-1/2 -translate-x-1/2 z-10 flex items-center gap-1 px-3 py-1.5 rounded-full bg-popover/90 border border-border/50 shadow-md text-xs text-muted-foreground hover:text-foreground hover:bg-popover transition-all cursor-pointer backdrop-blur-sm"
          >
            <ArrowDown className="w-3.5 h-3.5" />
            <span>底部</span>
          </button>
        )}
      </div>

      <ChatInput
        disabled={!hasNovel || !selectedKey}
        isLoading={isLoading}
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
          app.CancelChat(sessionId)
        }}
      />

      <div className="border-t mx-4" />

      <ChatControls
        models={models}
        selectedKey={selectedKey}
        onSelectModel={handleSelectModel}
        onRefreshModels={refreshModels}
        reasoningEffort={reasoningEffort}
        onSelectEffort={handleSelectEffort}
        thinkingEnabled={thinkingEnabled}
        onToggleThinking={handleToggleThinking}
        approvalMode={approvalMode}
        onToggleApproval={handleToggleApproval}
        onConfigModel={handleConfigModel}
        usage={lastUsage}
        onCompress={handleCompress}
        isTurnRunning={isLoading}
        isCompressing={isCompressing}
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
}
