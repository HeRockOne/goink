package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	wails "github.com/wailsapp/wails/v2/pkg/runtime"

	"gorm.io/gorm"

	"novel/internal/agentcfg"
	"novel/internal/approval"
	"novel/internal/config"
	"novel/internal/llm"
	"novel/internal/mcp_tools"
	"novel/internal/search"
	"novel/internal/session"
	"novel/internal/skill"
	"novel/internal/storage"
)

// agentDB 包级变量，供 parsePhaseGateFromMessages 读取配置用。
// 在 NewAgent 中设置。
var agentDB *gorm.DB

func getDB() *gorm.DB { return agentDB }

// maxLLMRetries LLM 请求最大重试次数（429/可重试错误），超过后按不可恢复错误返回。
const maxLLMRetries = 10

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
	phaseGateMu   sync.Mutex
	phaseGate     *PhaseGate // 从 always-mode skill 解析的阶段门禁配置（受 phaseGateMu 保护）
	prefixHashMu  sync.RWMutex
	prefixHash    map[string]uint64 // sessionID → 上一轮前缀哈希，用于缓存监控
}

// phaseGate 读写加锁，避免并发竞争
func (a *Agent) getPG() *PhaseGate {
	a.phaseGateMu.Lock()
	defer a.phaseGateMu.Unlock()
	return a.phaseGate
}

func (a *Agent) setPG(pg *PhaseGate) {
	a.phaseGateMu.Lock()
	defer a.phaseGateMu.Unlock()
	a.phaseGate = pg
}

// RunOptions 是单次 Run() 的参数。
type RunOptions struct {
	TurnID               int
	SessionID            string
	NovelID              int64
	Messages             []map[string]any
	AllowedTools         map[string]bool
	ActiveVersion        int
	SubAgentVersion      int // 子 Agent 内存版本计数器，不持久化
	Model                *llm.ModelInfo
	ProviderName         string
	AgentType            string
	SubTaskID            string                                      // 子 Agent 事件路由 ID
	EventSeq             *int                                        // 共享事件序号，nil 时自建（主Agent）；子Agent传入父的指针
	EventCallback        func(AgentEvent)                            // API 模式的事件回调（非 nil 时替代 wails.EventsEmit）
	Broadcast            func(eventType string, data map[string]any) // 双端同步广播（桌面端对话时推送到移动端 WebSocket）
	MaxTurns             int
	ReasoningEffort      string     // 用户选择的推理等级
	CompressionThreshold float64    // 压缩触发阈值（0.0-1.0）
	PhaseConfig          *PhaseGate // 从 always-mode skill 解析的阶段门禁配置
	PhaseCurrent         string     // 从 session 恢复的当前阶段
	PhaseCalledJSON      string     // 从 session 恢复的已调用工具 JSON
	PhaseMode            string     // 门禁模式："single" | "batch"
	PhaseGateEnabled     bool       // 门禁总开关，false 时跳过所有门禁检查
}

// New 创建 Agent 实例。
func New(llmClient *llm.Client, registry *mcp_tools.Registry, session *session.Store, db *gorm.DB, approver approval.Approver, logger *slog.Logger, skillStore *skill.Store, cancelMgr *CancelManager) *Agent {
	agentDB = db
	return &Agent{
		llm:        llmClient,
		registry:   registry,
		session:    session,
		db:         db,
		approver:   approver,
		logger:     logger,
		skillStore: skillStore,
		cancelMgr:  cancelMgr,
		prefixHash: make(map[string]uint64),
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
// 缓存协议：子 agent 请求 = 完整主会话历史原文 + 尾部追加子 agent 身份/NS/指令
// （Anthropic fork 模式完整版）。主历史含正文/设定/NS（上一轮主请求的完整字节），
// 子 agent 首轮即完整命中主会话缓存，miss 只余身份+指令（几 K）；
// 且子 agent 从历史直接看到正文与设定，无需重复 read（每次重复读 = 重复 miss 4-10K）。
func (a *Agent) RunSubAgent(ctx context.Context, parentOpts RunOptions, req mcp_tools.SubAgentRequest) (string, error) {
	at := agentTypeFromString(req.AgentType)
	sysPrompt := agentcfg.AgentIdentity(at)
	allowed := agentcfg.Allowlist(at)

	// 完整主历史（含 NS、工具结果）原样复用，保证前缀字节与主会话一致
	msgs := make([]map[string]any, 0, len(parentOpts.Messages)+4)
	msgs = append(msgs, parentOpts.Messages...)
	// 子 agent 专用层（历史之后，动态区），按稳定前缀顺序拆分消息：
	// [身份]（常量）→ [sub-* 技能]（review 专属，常量，见 buildSubagentSkills）→ [NS]（动态）
	// 身份与技能字节跨 review 不变，放在 NS 之前可命中缓存；NS/指令动态 miss。
	msgs = append(msgs, map[string]any{"role": "system", "content": sysPrompt})
	if at == agentcfg.ReviewAgent {
		if skills := a.buildSubagentSkills(req.NovelID); skills != "" {
			msgs = append(msgs, map[string]any{"role": "system", "content": skills})
		}
	}
	// 按需注入：主历史末尾已含最新 NS 时，若新生成 NS 字节相同（进度/指纹未变）
	// 则跳过重复注入——重复字节的新消息同样计入 miss 尾部（所有模型通用）
	if novelState, err := agentcfg.NovelState(a.db, req.NovelID); err == nil && novelState != "" {
		var lastNS string
		if err := a.db.Model(&session.Message{}).
			Where("session_id = ? AND extra_metadata LIKE ?", parentOpts.SessionID, agentcfg.NovelStateKindLike).
			Order("id DESC").Limit(1).Pluck("content", &lastNS).Error; err == nil && lastNS == novelState {
			novelState = ""
		}
		if novelState != "" {
			msgs = append(msgs, map[string]any{"role": "system", "content": novelState})
		}
	}
	msgs = append(msgs,
		map[string]any{"role": "user", "content": req.Instruction},
	)

	// 保存主 agent 的阶段门禁状态，子 agent 运行期间不会被清空
	savedPhaseGate := a.getPG()

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
		MaxTurns:        100,
		Model:           parentOpts.Model,
		ProviderName:    parentOpts.ProviderName,
		ReasoningEffort: parentOpts.ReasoningEffort,
		Broadcast:       parentOpts.Broadcast, // 子代理事件也广播到移动端
	}
	result, err := a.Run(ctx, subOpts)

	// 恢复主 agent 的阶段门禁状态
	a.setPG(savedPhaseGate)

	return result.FinalText, err
}

// buildSubagentSkills 扫描所有 sub- 前缀技能并拼接内容（子代理专属方法论）。
// 命名约定：sub- 前缀 = 子代理专用技能，新建 sub-*.md 自动纳入，零代码扩展。
// 内容从 SkillStore 三层查找（小说级 > 用户级 > 内置），技能名不在此硬编码。
func (a *Agent) buildSubagentSkills(novelID int64) string {
	if a.skillStore == nil {
		return ""
	}
	metas := a.skillStore.ListMeta(novelID)
	var b strings.Builder
	for _, m := range metas {
		if !strings.HasPrefix(m.Name, "sub-") {
			continue
		}
		sk, ok := a.skillStore.Get(novelID, m.Name)
		if !ok {
			continue
		}
		b.WriteString("--- ")
		b.WriteString(sk.Name)
		b.WriteString(" ---\n")
		b.WriteString(sk.RawContent)
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

// injectPhaseSkills 自动注入指定阶段的必读技能（auto_skill_injection）为 system 消息。
// 在 set_phase 成功或门禁自动推进时调用，技能以 system 消息追加到上下文，
// 模型无需再调 auto_skill_injection——技能是创作指导，系统保证其在创作动作前就绪。
// 去重：历史中已有相同技能全文（同 session 未压缩）则跳过重复注入——
// 技能已在上下文中，重复注入 = 每阶段切换固定 miss 全文（占 miss 构成 26%）。
// 压缩重建历史后技能消息被清掉，后续 set_phase 时历史中无该内容，自动恢复注入。
func (a *Agent) injectPhaseSkills(phase string, opts *RunOptions) {
	pg := a.getPG()
	if pg == nil || !pg.Active() {
		return
	}
	pc := pg.findPhase(phase)
	if pc == nil || len(pc.AutoSkillInjection) == 0 {
		return
	}
	content, err := mcp_tools.BuildSkillsContent(a.skillStore, opts.NovelID, pc.AutoSkillInjection)
	if err != nil {
		a.logger.Warn("自动注入必读技能失败", "phase", phase, "err", err)
		return
	}
	if content == "" {
		return
	}
	// 去重：opts.Messages 已含相同技能全文 → 跳过落库（门禁状态照常标记）
	for _, m := range opts.Messages {
		if role, _ := m["role"].(string); role == "system" {
			if c, ok := m["content"].(string); ok && c == content {
				a.logger.Debug("技能已在上下文中，跳过重复注入", "phase", phase, "skills", pc.AutoSkillInjection)
				for _, name := range pc.AutoSkillInjection {
					if !strings.Contains(name, "*") {
						pg.OnSkillInjected(name)
					}
				}
				return
			}
		}
	}
	a.appendMsg("system", content, "", nil, opts, nil)
	for _, name := range pc.AutoSkillInjection {
		if !strings.Contains(name, "*") {
			pg.OnSkillInjected(name)
		}
	}
	a.logger.Info("自动注入必读技能", "phase", phase, "skills", pc.AutoSkillInjection)
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
		opts.MaxTurns = 100
	}
	if opts.Model == nil {
		return AgentLoopResult{}, errors.New("agent: Model is required in RunOptions")
	}

	ctx = storage.WithTurn(ctx, opts.SessionID, opts.TurnID)

	// 门禁开关关闭时，强制清空上次残留的门禁状态
	if !opts.PhaseGateEnabled {
		a.setPG(nil)
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
		a.setPG(phaseGate)
		a.logger.Info("阶段门禁已启用", "phase", phaseGate.CurrentPhase(), "mode", phaseGate.mode)
		// Run 结束时保存门禁状态到 session
		defer func() {
			if a.getPG() != nil {
				phase, toolsJSON := a.getPG().SaveState()
				a.session.SavePhaseGateState(opts.SessionID, phase, toolsJSON)
				// 持久化门禁模式：批量会话跨 turn 必须保持 batch（防退化 single）
				mode := opts.PhaseMode
				if mode == "" {
					mode = "single"
				}
				// batch 完整流程走完回到 prepare（一轮结束），清除模式标记，
				// 避免后续单章会话误继承 batch 白名单
				if phase == "prepare" && a.getPG().wasVisited("prepare") && a.getPG().VisitedCount() > 1 {
					mode = ""
				}
				a.session.SavePhaseGateMode(opts.SessionID, mode)
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
	// 始终发送全量 tools（优化 Prompt Caching），用 allowed_tools 限制可用工具
	tools := a.registry.OpenAI(nil) // nil = 不限制，发送全量

	// 工具定义 token（固定前缀）：压缩触发判定必须计入，否则实际占用被低估 10-20%，
	// 0.7 阈值触发偏晚，中小窗口模型可能先撞 context overflow（400 不可重试，整轮失败）
	toolTokens := 0
	if tb, err := json.Marshal(tools); err == nil {
		if n, err := llm.CountMessageTokens(map[string]any{"role": "user", "content": string(tb)}); err == nil {
			toolTokens = n
		}
	}

	// 计算前缀哈希，检测缓存稳定性
	prefixHash := computePrefixHash(opts.Messages, tools)
	a.prefixHashMu.RLock()
	lastHash := a.prefixHash[opts.SessionID]
	a.prefixHashMu.RUnlock()
	if lastHash != 0 && lastHash != prefixHash {
		a.logger.Warn("前缀变化，缓存可能失效",
			"session_id", opts.SessionID,
			"last_hash", lastHash,
			"current_hash", prefixHash,
			"system_blocks", fmt.Sprintf("%+v", computeSystemBlockHashes(opts.Messages)),
			"tool_count", len(tools))
	}
	a.prefixHashMu.Lock()
	a.prefixHash[opts.SessionID] = prefixHash
	a.prefixHashMu.Unlock()

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
				"turn_id":    opts.TurnID,
				"type":       event.Type.String(),
				"session_id": opts.SessionID,
				"data":       event.Data,
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
	if a.getPG() != nil {
		ps := a.getPG().Status()
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
		usedTokens := sumRunningTokens(runningTokens) + toolTokens
		if opts.Model.ContextWindow > 0 && float64(usedTokens)/float64(opts.Model.ContextWindow) >= threshold {
			a.logger.Warn("token budget exceeded, triggering compression",
				"estimated", usedTokens,
				"context_window", opts.Model.ContextWindow,
				"ratio", fmt.Sprintf("%.1f%%", float64(usedTokens)/float64(opts.Model.ContextWindow)*100),
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

		callOpts := &llm.CallOptions{CacheKey: opts.SessionID}
		if opts.ReasoningEffort != "" {
			callOpts.ReasoningEffort = &opts.ReasoningEffort
		}
		// 传递门禁白名单到 API，让模型知道哪些工具可调用（不改变 tools 数组，保留缓存前缀）
		if len(opts.AllowedTools) > 0 {
			allowed := make([]string, 0, len(opts.AllowedTools))
			for tool := range opts.AllowedTools {
				allowed = append(allowed, tool)
			}
			callOpts.AllowedTools = allowed
		}

		stream := a.llm.ChatStream(ctx, opts.ProviderName, opts.Messages, tools, opts.Model.ID, callOpts)

		// ---- SSE 流处理 ----
		var pendingUsage map[string]any
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
						if a.getPG() == nil && opts.PhaseGateEnabled && opts.AgentType == "main" {
							mode := opts.PhaseMode
							if mode == "" {
								mode = "single"
							}
							if pg := parsePhaseGateFromMessages(opts.Messages, mode); pg != nil {
								if opts.PhaseCurrent != "" || opts.PhaseCalledJSON != "" {
									pg.LoadState(opts.PhaseCurrent, opts.PhaseCalledJSON)
								}
								a.setPG(pg)
								a.logger.Warn("set_phase 时门禁为空，已重新初始化", "phase", pg.CurrentPhase())
							}
						}
						if a.getPG() != nil {
							ok, warning := a.getPG().SetPhase(targetPhase)
							// 记录调用
							a.getPG().OnToolCall("set_phase", true)
							if ok {
								// 成功：自动注入新阶段必读技能。
								// 去重只针对同阶段 set_phase（批量 write 循环每章声明章边界）：
								// 技能已在上下文中，重复注入纯浪费；真切换（from != target）才注入。
								if from := a.getPG().CurrentPhase(); from != targetPhase {
									a.injectPhaseSkills(targetPhase, &opts)
								}
								// 批量 write 章边界：强制每章字数校验（真机验证：LLM 整批写完才
								// 在转出 review 时被字数拦截，被迫一次性扩写全部正文）。
								// wordCountOK 为 nil 或未达标时注入提醒，要求先 get_chapter_list。
								if a.getPG().mode == "batch" && targetPhase == "write" {
									if wc := a.getPG().WordCountCheck(); wc == nil || !*wc {
										wcMsg := "<system-reminder>\n本章尚未通过 get_chapter_list 字数校验（当前章节字数未达标或未校验）。请先调用 get_chapter_list 校验本章字数达标（min_words~max_words）后，再声明下一章边界。\n</system-reminder>"
										a.appendMsg("user", wcMsg, "", nil, &opts, runningTokens)
									}
								}
								// 发送状态（静态确认，禁止动态 StatusString）：
								// StatusString 含 called 工具列表（"已调用: get_characters(x1)..."），
								// 每次 set_phase 后内容随工具调用增长而变化——注入到历史中段后，
								// 后续所有请求的前缀包含这条动态消息，前缀缓存每次失效。
								// 8/9 批量每章 set_phase("write") 后每条都不同 → 命中率 89-93% 掉到 86%。
								// 工具结果已返回 phase，LLM 不需要 StatusString 细节。
								injectMsg := fmt.Sprintf("<system-reminder>\n已切换到 [%s] 阶段，继续执行该阶段任务。\n</system-reminder>", a.getPG().CurrentPhase())
								a.appendMsg("user", injectMsg, "", nil, &opts, runningTokens)
								ps := a.getPG().Status()
								emit(AgentEvent{TurnID: opts.TurnID, Type: EventPhaseGate, PhaseGate: &ps, Timestamp: time.Now()})
								toolOutputs = append(toolOutputs, toolOutput{name: name, id: id, rawArgs: rawArgs, result: &mcp_tools.ToolResult{Success: true, Data: map[string]any{"phase": a.getPG().CurrentPhase()}}, displayText: display.DisplayText, activityKind: display.ActivityKind})
							} else {
								// 失败：require 未满足或未知阶段
								resultJSON := fmt.Sprintf(`{"success":false,"error":"%s","current_phase":"%s"}`, warning, a.getPG().CurrentPhase())
								injectMsg := fmt.Sprintf("<system-reminder>\n%s\n</system-reminder>", resultJSON)
								a.appendMsg("user", injectMsg, "", nil, &opts, runningTokens)
								toolOutputs = append(toolOutputs, toolOutput{name: name, id: id, rawArgs: rawArgs, result: &mcp_tools.ToolResult{Success: false, Error: warning, Data: map[string]any{"current_phase": a.getPG().CurrentPhase()}}, displayText: display.DisplayText, activityKind: display.ActivityKind})
							}
							continue
						}
						toolOutputs = append(toolOutputs, toolOutput{name: name, id: id, rawArgs: rawArgs, result: &mcp_tools.ToolResult{Success: true, Data: map[string]any{"phase": targetPhase, "message": "门禁未启用"}}, displayText: display.DisplayText, activityKind: display.ActivityKind})
						continue
					}

					// ---- 阶段门禁：先检查，再执行（硬拦截） ----
					if a.getPG() != nil && a.getPG().Active() && a.getPG().CurrentPhase() != "" {
						allowed, warning := a.getPG().CheckToolAllowed(name)
						if !allowed {
							// 硬拦截：不执行工具，返回错误结果
							a.logger.Warn("门禁拦截", "tool", name, "phase", a.getPG().CurrentPhase())
							injectMsg := fmt.Sprintf("<system-reminder>\n🚫 门禁拦截：当前阶段 [%s] 不允许使用 [%s]。%s\n</system-reminder>", a.getPG().CurrentPhase(), name, warning)
							a.appendMsg("user", injectMsg, "", nil, &opts, runningTokens)
							ps := a.getPG().Status()
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
								if pathAllowed, pathWarning := a.getPG().CheckEditPath(editPath); !pathAllowed {
									a.logger.Warn("编辑路径被拦截", "path", editPath, "phase", a.getPG().CurrentPhase())
									injectMsg := fmt.Sprintf("<system-reminder>\n🚫 %s\n</system-reminder>", pathWarning)
									a.appendMsg("user", injectMsg, "", nil, &opts, runningTokens)
									ps := a.getPG().Status()
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
					if a.getPG() != nil && a.getPG().Active() {
						a.getPG().OnToolCall(name, result.Success)
// auto_skill_injection 成功：上报本次读取的技能名（auto_skill_injection 检查用）
					if name == "auto_skill_injection" && result.Success && result.Data != nil {
							if skills, ok := result.Data["skills"].([]string); ok {
								for _, s := range skills {
									a.getPG().OnSkillInjected(s)
								}
							}
						}
						// get_chapter_list 字数校验状态注入
						if name == "get_chapter_list" && result.Data != nil {
							if wcOK, ok := result.Data["word_count_ok"].(bool); ok {
								a.getPG().SetWordCountOK(wcOK)
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
					// 流式中途可能多次推送 usage（部分值），只保留请求结束时的最终值，
					// 避免中途值导致显示回跳和累计重复
					pendingUsage = event.Usage

				case llm.EventError:
					// 检查是否可重试（429限流 + Retryable标记）。最多重试 maxLLMRetries 次，
					// 防止账号欠费/持续限流时无限重试死循环
					retryable := false
					if apiErr, ok := event.Error.(*llm.APIError); ok {
						retryable = apiErr.StatusCode == 429 || apiErr.Retryable
					}
					if retryable && retryCount < maxLLMRetries {
						retryCount++
						waitTime := time.Duration(retryCount) * 5 * time.Second
						if waitTime > 60*time.Second {
							waitTime = 60 * time.Second
						}
						a.logger.Warn("LLM 请求失败，自动重试", "retry", retryCount, "wait", waitTime, "err", event.Error)
						emit(AgentEvent{
							TurnID: opts.TurnID, Type: EventRetry,
							RetryCount: retryCount, RetryMax: maxLLMRetries, RetryWait: int(waitTime.Seconds()),
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

		// 请求结束：统一处理最终 usage（过滤流式中途的部分值，避免显示回跳与重复累计）
		if pendingUsage != nil {
			a.updateUsage(ctx, pendingUsage, runningTokens, toolTokens, opts)
			pendingUsage = nil
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

		// 门禁自动推进（每轮检查）：require 满足即自动 set_phase 进入下一阶段。
		// 不能只放在循环后——LLM 在 write 阶段会持续调维护工具（create_scene/update_*）
		// 不输出收尾文本，循环永不 break，循环后的推进永远不会执行，卡死在 write
		// （真机：批量写完后 LLM 反复调 get_chapter_list/维护工具，不调 set_phase("review")，
		// 状态栏显示 review 但实际 phase=write，run_subagent 被 write 白名单拦截）。
		if a.getPG() != nil && a.getPG().Active() {
			ready, next := a.getPG().CheckTransitionReady()
			if ready && next != "" {
				current := a.getPG().CurrentPhase()
				if ok, _ := a.getPG().SetPhase(next); ok {
					a.injectPhaseSkills(next, &opts)
					a.logger.Info("门禁自动推进", "from", current, "to", next)
					reminder := fmt.Sprintf(
						"<system-reminder>\n阶段 [%s] 条件已满足，已自动推进到 [%s]\n</system-reminder>",
						current, next)
					a.appendMsg("user", reminder, "", nil, &opts, runningTokens)
					ps := a.getPG().Status()
					emit(AgentEvent{TurnID: opts.TurnID, Type: EventPhaseGate, PhaseGate: &ps, Timestamp: time.Now()})
				}
			}
		}
	}

	// 门禁自动推进（循环后兜底）：LLM 收尾 break 场景下，require 满足仍推进。
	// 循环内已推进过的（current==next）跳过，避免同阶段重复注入技能。
	if a.getPG() != nil && a.getPG().Active() {
		ready, next := a.getPG().CheckTransitionReady()
		if ready && next != "" && a.getPG().CurrentPhase() != next {
			current := a.getPG().CurrentPhase()
			a.getPG().SetPhase(next)
			a.injectPhaseSkills(next, &opts)
			a.logger.Info("门禁自动推进", "from", current, "to", next)
			reminder := fmt.Sprintf(
				"<system-reminder>\n阶段 [%s] 条件已满足，已自动推进到 [%s]\n</system-reminder>",
				current, next)
			a.appendMsg("user", reminder, "", nil, &opts, runningTokens)
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

// BuildWebSearch 构建 WebSearch 闭包，通过 Exa AI MCP 端点执行搜索。
// 优先使用用户在设置中配置的 Exa API key（x-api-key header），未配置则走免费 tier。
func (a *Agent) BuildWebSearch() func(ctx context.Context, query string) (*llm.WebSearchResult, error) {
	return func(ctx context.Context, query string) (*llm.WebSearchResult, error) {
		apiKey := ""
		if a.db != nil {
			if s, err := config.LoadSettings(a.db); err == nil && s != nil {
				apiKey = s.ExaAPIKey
			}
		}
		return llm.SearchWebWithKey(ctx, query, apiKey)
	}
}

// extraJSON 将 map 序列化为 JSON 字符串存入 ExtraMetadata。
func extraJSON(extra map[string]any) string {
	if len(extra) == 0 {
		return ""
	}
	b, err := json.Marshal(extra)
	if err != nil {
		slog.Warn("extraJSON marshal failed", "err", err)
		return ""
	}
	return string(b)
}

// parseArgs 将 JSON args 解析为 map。
func parseArgs(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		slog.Warn("parseArgs unmarshal failed", "err", err)
		return nil
	}
	return m
}

// parsePhaseGateFromMessages 从消息列表中扫描系统消息，提取 <!-- phase-gate-config --> 块。
// 优先从 app_config.phase_gate_config 读取（若有），完全避免占用 AI 上下文 token。
func parsePhaseGateFromMessages(messages []map[string]any, mode string) *PhaseGate {
	// 尝试从数据库配置读取
	if db := getDB(); db != nil {
		if s, err := config.LoadSettings(db); err == nil && s.PhaseGateConfig != "" {
			if pg := ParsePhaseGateConfig(s.PhaseGateConfig, mode); pg != nil {
				return pg
			}
		}
	}
	// 回退：从系统消息中扫描（兼容旧版 skill 内嵌配置）
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

// computeSystemBlockHashes 计算前导 system 消息各自的短哈希（诊断用，定位哪个块变化）。
func computeSystemBlockHashes(messages []map[string]any) []string {
	var hashes []string
	for _, m := range messages {
		if role, _ := m["role"].(string); role == "system" {
			if content, _ := m["content"].(string); content != "" {
				sum := sha256.Sum256([]byte(content))
				hashes = append(hashes, fmt.Sprintf("%x", sum[:4]))
			}
		} else {
			break
		}
	}
	return hashes
}

// computePrefixHash 计算前缀哈希，用于检测缓存稳定性。
// 前缀 = 前导系统消息（identity + always + catalog）+ 工具定义（按顺序）。
// 只哈希前导 system（到第一条非 system 消息为止），尾部动态 system（NS）不参与，
// 避免 NS 每轮变化触发误报。
func computePrefixHash(messages []map[string]any, tools []map[string]any) uint64 {
	h := sha256.New()

	// 哈希前导系统消息（role=system，遇第一条非 system 停止）
	for _, m := range messages {
		if role, _ := m["role"].(string); role == "system" {
			if content, _ := m["content"].(string); content != "" {
				h.Write([]byte(content))
				h.Write([]byte{0}) // 分隔符
			}
		} else {
			break
		}
	}

	// 哈希工具定义
	for _, t := range tools {
		if name, _ := t["function"].(map[string]any)["name"].(string); name != "" {
			h.Write([]byte(name))
			h.Write([]byte{0})
		}
	}

	// 取前 8 字节作为 uint64
	hash := h.Sum(nil)
	return uint64(hash[0])<<56 | uint64(hash[1])<<48 | uint64(hash[2])<<40 | uint64(hash[3])<<32 |
		uint64(hash[4])<<24 | uint64(hash[5])<<16 | uint64(hash[6])<<8 | uint64(hash[7])
}
