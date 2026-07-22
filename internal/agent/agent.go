package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync/atomic"
	"time"

	wails "github.com/wailsapp/wails/v2/pkg/runtime"

	"gorm.io/gorm"

	"novel/internal/agentcfg"
	"novel/internal/approval"
	"novel/internal/llm"
	"novel/internal/mcp_tools"
	"novel/internal/search"
	"novel/internal/session"
	"novel/internal/skill"
	"novel/internal/storage"
)

// Agent 是对话编排核心，持有运行所需的所有基础设施。
type Agent struct {
	llm           *llm.Client
	registry      *mcp_tools.Registry
	session       *session.Store
	db            *gorm.DB
	approver      approval.Approver
	logger        *slog.Logger
	skillStore    *skill.Store
	searchService atomic.Pointer[search.Service]
	cancelMgr     *CancelManager
	phaseGate     *PhaseGate // 从 always-mode skill 解析的阶段门禁配置
}

// RunOptions 是单次 Run() 的参数。
type RunOptions struct {
	TurnID          int
	SessionID       string
	NovelID         int64
	Messages        []map[string]any
	AllowedTools    map[string]bool
	ActiveVersion   int
	SubAgentVersion int // 子 Agent 内存版本计数器，不持久化
	Model           *llm.ModelInfo
	ProviderName    string
	AgentType       string
	SubTaskID       string // 子 Agent 事件路由 ID
	EventSeq        *int   // 共享事件序号，nil 时自建（主Agent）；子Agent传入父的指针
	EventCallback   func(AgentEvent) // API 模式的事件回调（非 nil 时替代 wails.EventsEmit）
	Broadcast       func(eventType string, data map[string]any) // 双端同步广播（桌面端对话时推送到移动端 WebSocket）
	MaxTurns        int
	ReasoningEffort string  // 用户选择的推理等级
	CompressionThreshold float64 // 压缩触发阈值（0.0-1.0）
	PhaseConfig     *PhaseGate // 从 always-mode skill 解析的阶段门禁配置
	PhaseCurrent    string     // 从 session 恢复的当前阶段
	PhaseCalledJSON string     // 从 session 恢复的已调用工具 JSON
	PhaseMode       string     // 门禁模式："single" | "batch"
	PhaseGateEnabled bool      // 门禁总开关，false 时跳过所有门禁检查
}

// New 创建 Agent 实例。
func New(llmClient *llm.Client, registry *mcp_tools.Registry, session *session.Store, db *gorm.DB, approver approval.Approver, logger *slog.Logger, skillStore *skill.Store, cancelMgr *CancelManager) *Agent {
	return &Agent{
		llm:        llmClient,
		registry:   registry,
		session:    session,
		db:         db,
		approver:   approver,
		logger:     logger,
		skillStore: skillStore,
		cancelMgr:  cancelMgr,
	}
}

// SetSearchService 设置搜索服务，在搜索服务初始化完成后由 App 调用。
func (a *Agent) SetSearchService(s *search.Service) { a.searchService.Store(s) }

// RegisterCancel 注册一个可取消的对话。
func (a *Agent) RegisterCancel(sessionID string, cancel context.CancelFunc) {
	a.cancelMgr.Register(CancelPrefixChat+sessionID, cancel)
}

// UnregisterCancel 对话结束后清理，只删不 cancel。
func (a *Agent) UnregisterCancel(sessionID string) {
	a.cancelMgr.Unregister(CancelPrefixChat + sessionID)
}

// Cancel 取消一个正在进行的对话。
func (a *Agent) Cancel(sessionID string) {
	a.cancelMgr.Cancel(CancelPrefixChat + sessionID)
}

// RunSubAgent 启动子 Agent 并返回最终报告文本。
func (a *Agent) RunSubAgent(ctx context.Context, parentOpts RunOptions, req mcp_tools.SubAgentRequest) (string, error) {
	at := agentTypeFromString(req.AgentType)
	sysPrompt := agentcfg.AgentIdentity(at)
	allowed := agentcfg.Allowlist(at)

	msgs := []map[string]any{
		{"role": "system", "content": sysPrompt},
	}
	if novelState, err := agentcfg.NovelState(a.db, req.NovelID); err == nil && novelState != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": novelState})
	}
	msgs = append(msgs, map[string]any{"role": "user", "content": req.Instruction})

	subOpts := RunOptions{
		TurnID:          parentOpts.TurnID,
		SessionID:       parentOpts.SessionID,
		NovelID:         req.NovelID,
		Messages:        msgs,
		AllowedTools:    allowed,
		ActiveVersion:   parentOpts.ActiveVersion,
		AgentType:       req.AgentType,
		SubTaskID:       req.ToolID,
		EventSeq:        parentOpts.EventSeq,
		MaxTurns:        50,
		Model:           parentOpts.Model,
		ProviderName:    parentOpts.ProviderName,
		ReasoningEffort: parentOpts.ReasoningEffort,
	}
	result, err := a.Run(ctx, subOpts)
	return result.FinalText, err
}

// agentTypeFromString 将字符串转为 AgentType。
func agentTypeFromString(s string) agentcfg.AgentType {
	switch s {
	case "review":
		return agentcfg.ReviewAgent
	case "memory":
		return agentcfg.MemoryAgent
	default:
		return agentcfg.MainAgent
	}
}

// Run 执行 Agent 循环，返回最终文本和轮数。
func (a *Agent) Run(ctx context.Context, opts RunOptions) (AgentLoopResult, error) {
	if opts.MaxTurns <= 0 {
		opts.MaxTurns = 50
	}
	if opts.Model == nil {
		return AgentLoopResult{}, errors.New("agent: Model is required in RunOptions")
	}

	ctx = storage.WithTurn(ctx, opts.SessionID, opts.TurnID)

	// 门禁开关关闭时，强制清空上次残留的门禁状态
	if !opts.PhaseGateEnabled {
		a.phaseGate = nil
	}

	// 初始化阶段门禁：如果 RunOptions 没有提供，从系统消息中解析
	// PhaseGateEnabled=false 时跳过所有门禁逻辑
	phaseGate := opts.PhaseConfig
	if opts.PhaseGateEnabled && phaseGate == nil && opts.AgentType == "main" {
		mode := opts.PhaseMode
		if mode == "" {
			mode = "single" // 默认单章模式
		}
		phaseGate = parsePhaseGateFromMessages(opts.Messages, mode)
	}
	if opts.PhaseGateEnabled && phaseGate != nil {
		// 从 session 恢复门禁状态（跨 turn 持久化）
		if opts.PhaseCurrent != "" || opts.PhaseCalledJSON != "" {
			phaseGate.LoadState(opts.PhaseCurrent, opts.PhaseCalledJSON)
		}
		a.phaseGate = phaseGate
		a.logger.Info("阶段门禁已启用", "phase", phaseGate.CurrentPhase(), "mode", phaseGate.mode)
		// Run 结束时保存门禁状态到 session
		defer func() {
			if a.phaseGate != nil {
				phase, toolsJSON := a.phaseGate.SaveState()
				a.session.SavePhaseGateState(opts.SessionID, phase, toolsJSON)
			}
		}()
	}

	loopCount := 0
	fullResponse := ""
	responseBuffer := ""
	thinkingBuffer := ""
	isThinking := false
	recentPatterns := make([]string, 0, 6)
	failCnt := make(map[string]int)
	retryCount := 0
	runningTokens := a.InitRunningTokens(opts.Messages)
	tools := a.registry.OpenAI(opts.AllowedTools)
	agentEventName := "agent:" + strconv.Itoa(opts.TurnID)
	eventSeq := opts.EventSeq
	if eventSeq == nil {
		seq := 0
		eventSeq = &seq
		opts.EventSeq = eventSeq //回写 子agent才能共享这个值
	}
	emit := func(event AgentEvent) {
		*eventSeq++
		event.Seq = *eventSeq
		if event.Timestamp.IsZero() {
			event.Timestamp = time.Now()
		}
		event.SubTaskID = opts.SubTaskID
		if opts.EventCallback != nil {
			opts.EventCallback(event)
		} else {
			wails.EventsEmit(ctx, agentEventName, event)
		}
		// 双端同步：桌面端对话时通过 WebSocket 广播到移动端
		if opts.Broadcast != nil {
			evData := map[string]any{
				"turn_id": opts.TurnID,
				"type":    event.Type.String(),
				"data":    event.Data,
			}
			if event.ErrMsg != "" {
				evData["error"] = event.ErrMsg
			}
			if event.ToolName != "" {
				evData["tool_name"] = event.ToolName
			}
			opts.Broadcast("chat:api_event", evData)
		}
	}

	interrupted := false

	// 发送阶段门禁初始状态到前端
	if a.phaseGate != nil {
		ps := a.phaseGate.Status()
		emit(AgentEvent{
			TurnID:    opts.TurnID,
			Type:      EventPhaseGate,
			PhaseGate: &ps,
			Timestamp: time.Now(),
		})
	}

	for loopCount < opts.MaxTurns {
		toolOutputs := make([]toolOutput, 0)
		pendingInjects := make(map[string][]mcp_tools.InjectMessage)
		// token 预算检查：每轮开始时，超限触发压缩
		threshold := opts.CompressionThreshold
		if threshold <= 0 || threshold >= 1 {
			threshold = 0.7
		}
		if opts.Model.ContextWindow > 0 && float64(sumRunningTokens(runningTokens))/float64(opts.Model.ContextWindow) >= threshold {
			a.logger.Warn("token budget exceeded, triggering compression",
				"estimated", sumRunningTokens(runningTokens),
				"context_window", opts.Model.ContextWindow,
				"ratio", fmt.Sprintf("%.1f%%", float64(sumRunningTokens(runningTokens))/float64(opts.Model.ContextWindow)*100),
				"agent_type", opts.AgentType,
			)
			var compressErr error
			if opts.AgentType == "main" {
				compressErr = a.Compress(ctx, &opts, runningTokens)
			} else {
				compressErr = a.compressInMemory(ctx, &opts, runningTokens)
			}
			if compressErr != nil {
				a.logger.Warn("compression failed, continuing with original context", "err", compressErr)
			}
		}

		callOpts := &llm.CallOptions{}
		if opts.ReasoningEffort != "" {
			callOpts.ReasoningEffort = &opts.ReasoningEffort
		}
		stream := a.llm.ChatStream(ctx, opts.ProviderName, opts.Messages, tools, opts.Model.ID, callOpts)

		// ---- SSE 流处理 ----
	streamLoop:
		for {
			select {
			case <-ctx.Done():
				interrupted = true
				a.flushInterruptedTools(stream, &opts, &toolOutputs)
				break streamLoop

			case event, ok := <-stream:
				if !ok {
					break streamLoop
				}

				switch event.Type {
				case llm.EventThinking:
					isThinking = true
					thinkingBuffer += event.Data
					emit(AgentEvent{
						TurnID: opts.TurnID, Type: EventThinking,
						Data: event.Data, Timestamp: time.Now(),
					})

				case llm.EventContent:
					if isThinking {
						emit(AgentEvent{
							TurnID: opts.TurnID, Type: EventThinkingDone, Timestamp: time.Now(),
						})
						isThinking = false
					}
					retryCount = 0 // 收到正常内容，重置重试计数
					responseBuffer += event.Data
					fullResponse += event.Data
					emit(AgentEvent{
						TurnID: opts.TurnID, Type: EventContent,
						Data: event.Data, Timestamp: time.Now(),
					})

				case llm.EventToolCallStart:
					if isThinking {
						emit(AgentEvent{
							TurnID: opts.TurnID, Type: EventThinkingDone, Timestamp: time.Now(),
						})
						isThinking = false
					}
					name := event.Delta.ToolName
					id := event.Delta.ToolID
					display := a.buildDisplay(name, nil, mcp_tools.PhaseSelected, opts.NovelID)
					emit(AgentEvent{
						TurnID: opts.TurnID, Type: EventToolCall,
						ToolName: name, ToolID: id, Phase: "selected",
						DisplayText: display.DisplayText, ActivityKind: display.ActivityKind,
						Metadata: display.Metadata, Timestamp: time.Now(),
					})

				case llm.EventToolCallEnd:
					name := event.Delta.ToolName
					id := event.Delta.ToolID
					rawArgs := event.Delta.ArgumentsJSON

					args := parseArgs(rawArgs)
					display := a.buildDisplay(name, args, mcp_tools.PhaseExecuting, opts.NovelID)

					// ---- set_phase 特殊工具：不走 registry ----
					if name == "set_phase" {
						targetPhase, _ := args["phase"].(string)
						// 如果 phaseGate 为空但门禁应该启用，尝试重新解析
						if a.phaseGate == nil && opts.PhaseGateEnabled && opts.AgentType == "main" {
							mode := opts.PhaseMode
							if mode == "" {
								mode = "single"
							}
							if pg := parsePhaseGateFromMessages(opts.Messages, mode); pg != nil {
								if opts.PhaseCurrent != "" || opts.PhaseCalledJSON != "" {
									pg.LoadState(opts.PhaseCurrent, opts.PhaseCalledJSON)
								}
								a.phaseGate = pg
								a.logger.Warn("set_phase 时门禁为空，已重新初始化", "phase", pg.CurrentPhase())
							}
						}
						if a.phaseGate != nil {
							ok, warning := a.phaseGate.SetPhase(targetPhase)
							// 记录调用
							a.phaseGate.OnToolCall("set_phase", true)
							if ok {
								// 成功：发送状态
								resultJSON := fmt.Sprintf(`{"success":true,"phase":"%s","status":"%s"}`, a.phaseGate.CurrentPhase(), a.phaseGate.StatusString())
								injectMsg := fmt.Sprintf("<system-reminder>\n%s\n</system-reminder>", resultJSON)
								a.appendMsg("user", injectMsg, "", nil, &opts, runningTokens)
								ps := a.phaseGate.Status()
								emit(AgentEvent{TurnID: opts.TurnID, Type: EventPhaseGate, PhaseGate: &ps, Timestamp: time.Now()})
								toolOutputs = append(toolOutputs, toolOutput{name: name, id: id, rawArgs: rawArgs, result: &mcp_tools.ToolResult{Success: true, Data: map[string]any{"phase": a.phaseGate.CurrentPhase()}}, displayText: display.DisplayText, activityKind: display.ActivityKind})
							} else {
								// 失败：require 未满足或未知阶段
								resultJSON := fmt.Sprintf(`{"success":false,"error":"%s","current_phase":"%s"}`, warning, a.phaseGate.CurrentPhase())
								injectMsg := fmt.Sprintf("<system-reminder>\n%s\n</system-reminder>", resultJSON)
								a.appendMsg("user", injectMsg, "", nil, &opts, runningTokens)
								toolOutputs = append(toolOutputs, toolOutput{name: name, id: id, rawArgs: rawArgs, result: &mcp_tools.ToolResult{Success: false, Error: warning, Data: map[string]any{"current_phase": a.phaseGate.CurrentPhase()}}, displayText: display.DisplayText, activityKind: display.ActivityKind})
							}
							continue
						}
						toolOutputs = append(toolOutputs, toolOutput{name: name, id: id, rawArgs: rawArgs, result: &mcp_tools.ToolResult{Success: true, Data: map[string]any{"phase": targetPhase, "message": "门禁未启用"}}, displayText: display.DisplayText, activityKind: display.ActivityKind})
						continue
					}

					// ---- 阶段门禁：先检查，再执行（硬拦截） ----
					if a.phaseGate != nil && a.phaseGate.Active() && a.phaseGate.CurrentPhase() != "" {
						allowed, warning := a.phaseGate.CheckToolAllowed(name)
						if !allowed {
							// 硬拦截：不执行工具，返回错误结果
							a.logger.Warn("门禁拦截", "tool", name, "phase", a.phaseGate.CurrentPhase())
							injectMsg := fmt.Sprintf("<system-reminder>\n🚫 门禁拦截：当前阶段 [%s] 不允许使用 [%s]。%s\n</system-reminder>", a.phaseGate.CurrentPhase(), name, warning)
							a.appendMsg("user", injectMsg, "", nil, &opts, runningTokens)
							ps := a.phaseGate.Status()
							emit(AgentEvent{
								TurnID:    opts.TurnID,
								Type:      EventPhaseGate,
								PhaseGate: &ps,
								ErrMsg:    fmt.Sprintf("门禁拦截: %s", warning),
								Timestamp: time.Now(),
							})
							toolOutputs = append(toolOutputs, toolOutput{name: name, id: id, rawArgs: rawArgs, result: &mcp_tools.ToolResult{Success: false, Error: fmt.Sprintf("门禁拦截：%s", warning), ErrKind: "user"}, displayText: display.DisplayText, activityKind: display.ActivityKind})
							continue
						}
						// edit 工具路径检查：不同阶段只能编辑特定文件
						if name == "edit" {
							if editPath, ok := args["path"].(string); ok && editPath != "" {
								if pathAllowed, pathWarning := a.phaseGate.CheckEditPath(editPath); !pathAllowed {
									a.logger.Warn("编辑路径被拦截", "path", editPath, "phase", a.phaseGate.CurrentPhase())
									injectMsg := fmt.Sprintf("<system-reminder>\n🚫 %s\n</system-reminder>", pathWarning)
									a.appendMsg("user", injectMsg, "", nil, &opts, runningTokens)
									ps := a.phaseGate.Status()
									emit(AgentEvent{TurnID: opts.TurnID, Type: EventPhaseGate, PhaseGate: &ps, ErrMsg: pathWarning, Timestamp: time.Now()})
									toolOutputs = append(toolOutputs, toolOutput{name: name, id: id, rawArgs: rawArgs, result: &mcp_tools.ToolResult{Success: false, Error: pathWarning, ErrKind: "user"}, displayText: display.DisplayText, activityKind: display.ActivityKind})
									continue
								}
							}
						}
						// 门禁通过，准备执行工具
					}

					// ---- 执行工具 ----
					emit(AgentEvent{
						TurnID: opts.TurnID, Type: EventToolCall,
						ToolName: name, ToolID: id, Phase: "executing",
						ToolArgs: args, DisplayText: display.DisplayText, ActivityKind: display.ActivityKind,
						Metadata: display.Metadata, Timestamp: time.Now(),
					})

					tc := mcp_tools.ToolContext{
						DB:       a.db,
						NovelID:  opts.NovelID,
						ToolID:   id,
						Approver: a.approver,
						EmitApproval: func(toolID string, approvalType string, payload map[string]any) {
							emit(AgentEvent{
								TurnID: opts.TurnID, Type: EventToolCall,
								ToolName: name, ToolID: toolID, Phase: "awaiting_approval",
								Metadata: map[string]any{
									"approval_type": approvalType,
									"payload":       payload,
								},
								Timestamp: time.Now(),
							})
						},
						RunSubAgent: func(ctx context.Context, req mcp_tools.SubAgentRequest) (string, error) {
							return a.RunSubAgent(ctx, opts, req)
						},
						SkillStore:    a.skillStore,
						SearchService: a.searchService.Load(),
						WebSearch:     a.BuildWebSearch(),
					}
					result := a.registry.Execute(ctx, name, rawArgs, tc, opts.AllowedTools)
					a.logger.Info("tool executed", "tool", name, "success", result.Success, "phase", map[bool]string{true: "completed", false: "failed"}[result.Success])

					phase := "completed"
					if !result.Success {
						phase = "failed"
					}
					display = a.buildDisplay(name, args, displayPhase(phase), opts.NovelID)
					metadata := display.Metadata
					if (name == "web_search" || name == "web_fetch") && result.Success && result.Data != nil {
						if metadata == nil {
							metadata = make(map[string]any)
						}
						for k, v := range result.Data {
							metadata[k] = v
						}
					}
					emit(AgentEvent{
						TurnID: opts.TurnID, Type: EventToolCall,
						ToolName: name, ToolID: id, Phase: phase,
						ToolArgs: args, Success: result.Success, ErrMsg: result.Error,
						DisplayText: display.DisplayText, ActivityKind: display.ActivityKind,
						Metadata: metadata, Timestamp: time.Now(),
					})

					// 门禁：记录调用
					if a.phaseGate != nil && a.phaseGate.Active() {
						a.phaseGate.OnToolCall(name, result.Success)
						// get_chapter_list 字数校验状态注入
						if name == "get_chapter_list" && result.Data != nil {
							if wcOK, ok := result.Data["word_count_ok"].(bool); ok {
								a.phaseGate.SetWordCountOK(wcOK)
							}
						}
					}

					// 失败计数：仅系统异常计入
					if !result.Success && result.ErrKind == "system" {
						failCnt[name]++
					} else {
						failCnt[name] = 0
					}
					if failCnt[name] == 3 {
						content := fmt.Sprintf("<system-reminder>\n工具 %s 已连续失败 3 次，已被禁用，请不要再调用此工具。\n</system-reminder>", name)
						a.appendMsg("user", content, "", nil, &opts, runningTokens)
					}

					// 暂存 inject
					if len(result.Inject) > 0 {
						pendingInjects[id] = result.Inject
					}

					toolOutputs = append(toolOutputs, toolOutput{name: name, id: id, rawArgs: rawArgs, result: result, displayText: display.DisplayText, activityKind: display.ActivityKind})

				case llm.EventUsage:
					a.updateUsage(ctx, event.Usage, runningTokens, opts)

				case llm.EventError:
					// 检查是否可重试（429限流 + Retryable标记），无限重试直到连接恢复
					retryable := false
					if apiErr, ok := event.Error.(*llm.APIError); ok {
						retryable = apiErr.StatusCode == 429 || apiErr.Retryable
					}
					if retryable {
						retryCount++
						waitTime := time.Duration(retryCount) * 5 * time.Second
						if waitTime > 60*time.Second {
							waitTime = 60 * time.Second
						}
						a.logger.Warn("LLM 请求失败，自动重试", "retry", retryCount, "wait", waitTime, "err", event.Error)
						emit(AgentEvent{
							TurnID: opts.TurnID, Type: EventRetry,
							RetryCount: retryCount, RetryMax: 0, RetryWait: int(waitTime.Seconds()),
							Timestamp: time.Now(),
						})
						responseBuffer = ""
						thinkingBuffer = ""
						fullResponse = ""
						isThinking = false
						time.Sleep(waitTime)
						if ctx.Err() != nil {
							return AgentLoopResult{FinalText: fullResponse, ThinkingContent: thinkingBuffer, TurnCount: loopCount}, ctx.Err()
						}
						stream = a.llm.ChatStream(ctx, opts.ProviderName, opts.Messages, tools, opts.Model.ID, callOpts)
						continue streamLoop
					}
					// 不可重试或超过重试次数：保存 partial 后返回
					emit(AgentEvent{
						TurnID: opts.TurnID, Type: EventError,
						ErrMsg: FriendlyError(event.Error), Timestamp: time.Now(),
					})
					if responseBuffer != "" || thinkingBuffer != "" {
						a.appendMsg("assistant", responseBuffer, thinkingBuffer,
							nil, &opts, runningTokens)
					}
					return AgentLoopResult{FinalText: fullResponse, ThinkingContent: thinkingBuffer, TurnCount: loopCount}, event.Error
				}
			}
		}

		// ---- 流结束，判断是否有工具调用 ----
		if len(toolOutputs) == 0 {
			if isThinking {
				emit(AgentEvent{
					TurnID: opts.TurnID, Type: EventThinkingDone, Timestamp: time.Now(),
				})
			}
			if responseBuffer != "" || thinkingBuffer != "" {
				a.appendMsg("assistant", responseBuffer, thinkingBuffer,
					nil, &opts, runningTokens)
			} //此处持久化最终信息，主agent和subagent共享避免遗漏
			break
		}

		// 1. assistant + tool_calls + tool_displays

		a.appendMsg("assistant", responseBuffer, thinkingBuffer,
			map[string]any{
				"tool_calls":    buildToolCalls(toolOutputs),
				"tool_displays": buildToolDisplay(toolOutputs),
			}, &opts, runningTokens)

		// 2. tool 结果
		for _, to := range toolOutputs {
			a.appendMsg("tool", to.resultJSON(),
				"", map[string]any{"tool_call_id": to.id, "tool_name": to.name},
				&opts, runningTokens)
		}

		// 3. inject（role=user，<system-reminder> 包裹）
		for _, to := range toolOutputs {
			for _, inj := range pendingInjects[to.id] {
				content := "<system-reminder>\n" + inj.Content + "\n</system-reminder>"
				a.appendMsg(inj.Role, content, "", nil, &opts, runningTokens)
			}
		}

		if interrupted {
			break
		}

		// 4. 死循环检测
		patterns := append(recentPatterns, toolPattern(toolOutputs))
		if len(patterns) > 6 {
			patterns = patterns[1:]
		}
		if isStuckLoop(patterns, toolOutputs, loopCount) {
			content := "<system-reminder>\n系统检测到可能陷入重复调用。请基于已获取的信息直接开始写作，或明确告诉我你需要什么新的操作。\n</system-reminder>"
			a.appendMsg("user", content, "", nil, &opts, runningTokens)
			emit(AgentEvent{
				TurnID: opts.TurnID, Type: EventToolCall, Phase: "loop_detected", Timestamp: time.Now(),
			})
		}
		recentPatterns = patterns

		// 清空当前轮缓冲
		thinkingBuffer = ""
		responseBuffer = ""
		fullResponse = ""
		loopCount++
	}

	// 门禁强制提醒：require 已满足但未调用 set_phase 时，注入提醒消息
	if a.phaseGate != nil && a.phaseGate.Active() {
		ready, next := a.phaseGate.CheckTransitionReady()
		if ready && next != "" {
			reminder := fmt.Sprintf(
				"<system-reminder>\n⚠️ 门禁提醒：当前阶段 [%s] 的所有条件已满足，你必须调用 set_phase(\"%s\") 切换到下一阶段。这是强制要求，不调用将导致流程卡死。\n</system-reminder>",
				a.phaseGate.CurrentPhase(), next)
			a.appendMsg("user", reminder, "", nil, &opts, runningTokens)
			a.logger.Warn("注入阶段推进提醒", "phase", a.phaseGate.CurrentPhase(), "next", next)
		}
	}

	if interrupted {
		return AgentLoopResult{FinalText: fullResponse, ThinkingContent: thinkingBuffer, TurnCount: loopCount}, ctx.Err()
	}
	return AgentLoopResult{FinalText: fullResponse, ThinkingContent: thinkingBuffer, TurnCount: loopCount}, nil
}

// appendMsg 统一处理消息的内存追加 + 持久化 + token 计数。
// opts 必须传指针，因为 opts.Messages 需要被追加（Go 切片传值会丢失 append）。
func (a *Agent) appendMsg(role, content, thinkingContent string, extra map[string]any, opts *RunOptions, runningTokens map[string]int) {
	msg := &session.Message{
		SessionID:       opts.SessionID,
		TurnID:          opts.TurnID,
		AgentType:       opts.AgentType,
		SubTaskID:       opts.SubTaskID,
		Role:            role,
		Content:         content,
		ThinkingContent: thinkingContent,
		ExtraMetadata:   extraJSON(extra),
		Version:         opts.ActiveVersion,
		ToAPI:           opts.AgentType == "main",
		ToFrontend:      role == "assistant",
	}
	a.logger.Debug("appendMsg", "role", role, "agentType", opts.AgentType, "subTaskID", opts.SubTaskID, "turnID", opts.TurnID)
	if err := a.db.Create(msg).Error; err != nil {
		a.logger.Error("持久化消息失败", "role", role, "turnID", opts.TurnID, "err", err)
	}

	apiFormat := msg.ToAPIFormat()
	opts.Messages = append(opts.Messages, apiFormat)
	n, err := llm.CountMessageTokens(apiFormat)
	if err != nil {
		a.logger.Warn("token count failed", "role", role, "err", err)
	}
	runningTokens[role] += n
}

// sumRunningTokens 计算各角色 token 总数。
func sumRunningTokens(tokens map[string]int) int {
	total := 0
	for _, n := range tokens {
		total += n
	}
	return total
}

// displayPhase 将 completed/failed 字符串转为 DisplayPhase。
func displayPhase(phase string) mcp_tools.DisplayPhase {
	switch phase {
	case "completed":
		return mcp_tools.PhaseCompleted
	case "failed":
		return mcp_tools.PhaseFailed
	}
	return mcp_tools.PhaseCompleted
}

// BuildWebSearch 构建 WebSearch 闭包，通过 Exa AI MCP 端点执行搜索，无需 API key。
func (a *Agent) BuildWebSearch() func(ctx context.Context, query string) (*llm.WebSearchResult, error) {
	return func(ctx context.Context, query string) (*llm.WebSearchResult, error) {
		return llm.SearchWeb(ctx, query)
	}
}

// extraJSON 将 map 序列化为 JSON 字符串存入 ExtraMetadata。
func extraJSON(extra map[string]any) string {
	if len(extra) == 0 {
		return ""
	}
	b, _ := json.Marshal(extra)
	return string(b)
}

// parseArgs 将 JSON args 解析为 map。
func parseArgs(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	json.Unmarshal(raw, &m)
	return m
}

// parsePhaseGateFromMessages 从消息列表中扫描系统消息，提取 <!-- phase-gate-config --> 块。
func parsePhaseGateFromMessages(messages []map[string]any, mode string) *PhaseGate {
	for _, msg := range messages {
		if msg["role"] != "system" {
			continue
		}
		content, _ := msg["content"].(string)
		if content == "" {
			continue
		}
		if pg := ParsePhaseGateConfig(content, mode); pg != nil {
			return pg
		}
	}
	return nil
}
