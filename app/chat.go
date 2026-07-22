package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	wails "github.com/wailsapp/wails/v2/pkg/runtime"

	"gorm.io/gorm"

	"novel/internal/agent"
	"novel/internal/agentcfg"
	"novel/internal/config"
	"novel/internal/git"
	"novel/internal/rollback"
	"novel/internal/session"
)

// ChatInput 是一次对话请求的入参。
type ChatInput struct {
	SessionID       string `json:"session_id"` // 空=新建会话
	NovelID         int64  `json:"novel_id"`
	Message         string `json:"message"`
	ProviderName    string `json:"provider_name"`    // "deepseek"
	ModelID         string `json:"model_id"`         // "deepseek-v4-pro"
	ReasoningEffort string `json:"reasoning_effort"` // "high" | "max" | ""
}

// ChatResult 是一次对话请求的返回值。
type ChatResult struct {
	SessionID string `json:"session_id"`
	TurnID    int    `json:"turn_id"`
	FinalText string `json:"final_text"`
}

// Chat 是对话入口。Wails 绑定，前端调用后同步执行，期间通过 EventsEmit 推流。
func (a *App) Chat(input ChatInput) (*ChatResult, error) {
	return a.chatImpl(input, nil)
}

// ChatWithCallback 是 API 服务器调用的对话入口，通过回调推送事件。
func (a *App) ChatWithCallback(input ChatInput, eventCallback func(map[string]any)) (*ChatResult, error) {
	return a.chatImpl(input, eventCallback)
}

// chatImpl 是 Chat 的核心实现，eventCallback 非 nil 时通过回调推送事件。
func (a *App) chatImpl(input ChatInput, eventCallback func(map[string]any)) (*ChatResult, error) {
	// Panic recovery
	defer func() {
		if rec := recover(); rec != nil {
			if eventCallback != nil {
				eventCallback(map[string]any{"type": "error", "error": fmt.Sprintf("内部错误: %v", rec)})
			}
		}
	}()

	// 安全检查
	if a.llmClient == nil || a.agent == nil {
		return nil, fmt.Errorf("服务未初始化，请重启 Goink")
	}
	if a.settings == nil {
		return nil, fmt.Errorf("配置未加载")
	}

	// 1. 验证模型
	model, ok := a.llmClient.ProviderModel(input.ProviderName, input.ModelID)
	if !ok {
		return nil, fmt.Errorf("模型未找到: %s/%s", input.ProviderName, input.ModelID)
	}

	ctx, cancel := context.WithCancel(a.ctx)
	defer cancel()

	// 辅助：推事件
	emitEvent := func(eventType string, data map[string]any) {
		if eventCallback != nil {
			// API 模式：通过回调推送到 SSE 流
			data["type"] = eventType
			eventCallback(data)
			// 同时通过 Wails 事件实时推送到桌面前端
			if a.ctx != nil {
				ev := make(map[string]any, len(data))
				for k, v := range data {
					ev[k] = v
				}
				ev["type"] = eventType
				wails.EventsEmit(a.ctx, "chat:api_event", ev)
			}
		} else {
			// Wails 模式：通过 EventsEmit 发送事件到前端
			wails.EventsEmit(ctx, "chat:"+eventType, data)
			// 同时通过 WebSocket 广播到移动端（双端同步）
			a.BroadcastChatEvent(eventType, data)
		}
	}

	// 2. 加载或创建 Session
	sess, isNew, err := a.loadOrCreateSession(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("session 初始化失败: %w", err)
	}

	a.agent.Cancel(sess.SessionID)
	a.agent.RegisterCancel(sess.SessionID, cancel)
	defer func() {
		if ctx.Err() == nil {
			a.agent.UnregisterCancel(sess.SessionID)
		}
	}()

	// 3. 新会话自动生成标题（异步，与 agent LLM 调用并发）
	if isNew && sess.Title == "" {
		go a.generateTitle(sess.SessionID, input.Message, input.ProviderName, input.ModelID)
	}

	// 4. 获取下一个 turn ID
	turnID, err := a.session.NextTurn(ctx, sess.SessionID)
	if err != nil {
		return nil, fmt.Errorf("获取 turn ID 失败: %w", err)
	}

	// 5. 打开 git 仓库，提交用户在对话间隙的手动编辑
	repo, repoErr := git.New(input.NovelID, a.settings.GitName, a.settings.GitEmail, a.logger)
	if repoErr != nil {
		a.logger.Warn("auto-commit: 打开 git 仓库失败，跳过本轮自动提交", "err", repoErr)
	} else {
		a.commitUserChanges(repo, turnID, sess.SessionID)
		defer a.commitAIChanges(repo, turnID, sess.SessionID, model.Name)
	}

	// 6. 解析 / 命令（skill 或 quick command），构建注入内容
	var slashInject string
	var injectName string
	if strings.HasPrefix(input.Message, "/") {
		parts := strings.Fields(input.Message)
		if len(parts) > 0 {
			cmdName := strings.TrimPrefix(parts[0], "/")
			if inj, name := a.resolveSlashCommand(input.NovelID, cmdName); inj != "" {
				slashInject = inj
				injectName = name
			}
		}
	}

	// 6.5 每次对话都激活阶段门禁（系统级强制）
	// 确保 currentPhase 始终有值，门禁不会因空值跳过
	if sess.CurrentPhase == "" {
		sess.CurrentPhase = "prepare"
	}
	if err := a.session.DB.Model(&session.Session{}).
		Where("session_id = ?", sess.SessionID).
		Update("current_phase", sess.CurrentPhase).Error; err != nil {
		a.logger.Warn("保存阶段门禁状态失败", "err", err)
	}

	// 7. 持久化本轮消息（事务：System 消息 + slash inject + 用户消息原子写入）
	userMsg := &session.Message{
		SessionID:  sess.SessionID,
		TurnID:     turnID,
		Role:       "user",
		Content:    input.Message,
		Version:    sess.ActiveVersion,
		ToAPI:      true,
		ToFrontend: true,
		AgentType:  "main",
	}
	if err := a.session.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if isNew {
			if err := a.writeSystemMessages(tx, sess.SessionID, input.NovelID, turnID); err != nil {
				return err
			}
		}
		if slashInject != "" {
			if err := tx.Create(&session.Message{
				SessionID:  sess.SessionID,
				TurnID:     turnID,
				Role:       "user",
				Content:    slashInject,
				Version:    sess.ActiveVersion,
				ToAPI:      true,
				ToFrontend: false,
				AgentType:  "main",
			}).Error; err != nil {
				return err
			}
		}
		return tx.Create(userMsg).Error
	}); err != nil {
		return nil, fmt.Errorf("持久化消息失败: %w", err)
	}

	if injectName != "" {
		a.logger.Info("slash injected", "name", injectName, "session", sess.SessionID)
	}

	// 8. 构建消息列表：全部来自 DB（含系统消息/历史/用户消息）
	messages, err := a.loadAPIMessages(ctx, sess.SessionID, sess.ActiveVersion)
	if err != nil {
		return nil, fmt.Errorf("加载 API 消息失败: %w", err)
	}

	// 9. 运行 Agent 循环
	emitEvent("started", map[string]any{"session_id": sess.SessionID, "turn_id": turnID})

	// 构建 Agent RunOptions
	runOpts := agent.RunOptions{
		TurnID:               turnID,
		SessionID:            sess.SessionID,
		NovelID:              input.NovelID,
		Messages:             messages,
		AllowedTools:         agentcfg.Allowlist(agentcfg.MainAgent),
		ActiveVersion:        sess.ActiveVersion,
		Model:                model,
		ProviderName:         input.ProviderName,
		AgentType:            "main",
		MaxTurns:             50,
		ReasoningEffort:      input.ReasoningEffort,
		CompressionThreshold: a.settings.CompressionThreshold,
		PhaseCurrent:         sess.CurrentPhase,
		PhaseCalledJSON:      sess.CalledTools,
		PhaseMode:            "single",
		PhaseGateEnabled:     a.settings.PhaseGateEnabled == nil || *a.settings.PhaseGateEnabled,
		Broadcast:            a.BroadcastChatEvent, // 双端同步：agent 事件广播到移动端
	}

	// API 模式需要 EventCallback 转发事件；Wails 模式让 agent.go 自己 emit 到 "agent:${turnID}"
	if eventCallback != nil {
		runOpts.EventCallback = func(event agent.AgentEvent) {
			ev := map[string]any{
				"type":      eventTypeString(event.Type),
				"turn_id":   event.TurnID,
				"data":      event.Data,
				"error":     event.ErrMsg,
				"tool_name": event.ToolName,
			}
			eventCallback(ev)
			// 同时通过 Wails 事件实时推送到桌面前端，实现双端同步
			if a.ctx != nil {
				wails.EventsEmit(a.ctx, "chat:api_event", ev)
			}
		}
	}

	result, runErr := a.agent.Run(ctx, runOpts)

	// 10. 最终回复已由 agent.Run() 内部 appendMsg 持久化，此处不重复存储
	if runErr != nil {
		a.logger.Error("对话失败", "err", runErr)
		eventType := "system_interrupted"
		if errors.Is(runErr, context.Canceled) {
			eventType = "user_stopped"
		}
		a.session.DB.Create(&session.Message{
			SessionID:  sess.SessionID,
			TurnID:     turnID,
			Role:       "system",
			EventType:  eventType,
			Content:    agent.FriendlyError(runErr),
			ToFrontend: true,
			ToAPI:      false,
			AgentType:  "main",
		})
		// 错误时也通知前端刷新
		if eventCallback != nil && a.ctx != nil {
			wails.EventsEmit(a.ctx, "chat:api_done", map[string]any{
				"session_id": sess.SessionID,
				"turn_id":    turnID,
			})
		}
		return &ChatResult{
			SessionID: sess.SessionID,
			TurnID:    turnID,
			FinalText: result.FinalText,
		}, fmt.Errorf("%s", agent.FriendlyError(runErr))
	}

	// 11. 对话结束后自动导出到 outputs 目录（供 WebDAV 阅读）
	a.ExportToOutputs(input.NovelID, "txt")

	// 12. 发送完成事件
	emitEvent("done", map[string]any{"turn_id": turnID, "text": result.FinalText})

	// API 模式（移动端对话）结束后，通过 Wails 事件通知桌面前端刷新
	if eventCallback != nil && a.ctx != nil {
		wails.EventsEmit(a.ctx, "chat:api_done", map[string]any{
			"session_id": sess.SessionID,
			"turn_id":    turnID,
		})
	}

	return &ChatResult{
		SessionID: sess.SessionID,
		TurnID:    turnID,
		FinalText: result.FinalText,
	}, nil
}

// loadOrCreateSession 加载已有 session 或创建新 session。
func (a *App) loadOrCreateSession(ctx context.Context, input ChatInput) (*session.Session, bool, error) {
	if input.SessionID != "" {
		var sess session.Session
		err := a.session.DB.WithContext(ctx).
			Where("session_id = ?", input.SessionID).
			First(&sess).Error
		if err == nil {
			return &sess, false, nil
		}
	}

	// 创建新 session
	sess := &session.Session{
		SessionID:       fmt.Sprintf("sess_%d_%x", input.NovelID, time.Now().UnixNano()),
		NovelID:         input.NovelID,
		Model:           input.ModelID,
		ReasoningEffort: input.ReasoningEffort,
	}
	if err := a.session.DB.WithContext(ctx).Create(sess).Error; err != nil {
		return nil, false, err
	}

	// 仅 Wails 上下文发送事件，API 模式跳过
	if a.ctx != nil && ctx == a.ctx {
		wails.EventsEmit(ctx, "chat:session_created", sess)
	}
	return sess, true, nil
}

// writeSystemMessages 在新 session 的事务内写入 AgentIdentity、AlwaysSkills、SkillCatalog、NovelState 到 messages 表。
func (a *App) writeSystemMessages(tx *gorm.DB, sessionID string, novelID int64, turnID int) error {
	sysMsg := func(content string) *session.Message {
		return &session.Message{
			SessionID: sessionID, TurnID: turnID, Role: "system", Content: content,
			Version: 1, ToAPI: true, ToFrontend: false, AgentType: "main",
		}
	}

	identity := agentcfg.AgentIdentity(agentcfg.MainAgent)

	var always string
	var catalog string
	if a.skill != nil {
		all := a.skill.ListMeta(novelID)
		catalog = agentcfg.BuildSkillCatalog(a.skill.ListMetaForCatalog(all))
		always = agentcfg.BuildAlwaysSkillsContent(all, a.skill, novelID)
	}

	novelState, err := agentcfg.NovelState(tx, novelID)
	if err != nil {
		a.logger.Warn("NovelState 构建失败，写入空消息", "novel_id", novelID, "err", err)
		novelState = ""
	}

	for _, c := range []string{identity, always, catalog, novelState} {
		if c != "" {
			if err := tx.Create(sysMsg(c)).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// loadAPIMessages 加载指定 version 的所有 to_api 消息，转为 map 格式。
func (a *App) loadAPIMessages(ctx context.Context, sessionID string, version int) ([]map[string]any, error) {
	msgs, err := a.session.GetMessagesForAPI(ctx, sessionID, version)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		result = append(result, m.ToAPIFormat())
	}
	return result, nil
}

// generateTitle 用 LLM 为非流式调用生成对话标题（≤10 字）。
func (a *App) generateTitle(sessionID, userMessage, providerName, modelID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	messages := []map[string]any{
		{
			"role":    "system",
			"content": "基于用户消息，生成一个不超过10个字的对话标题。只需输出标题文本，不要添加引号、标点或者额外解释。",
		},
		{"role": "user", "content": userMessage},
	}

	title, err := a.llmClient.GenerateText(ctx, providerName, messages, modelID)
	if err != nil {
		a.logger.Warn("自动生成标题失败", "err", err)
		return
	}

	title = strings.TrimSpace(title)
	if len([]rune(title)) > 30 {
		title = string([]rune(title)[:30])
	}
	if title == "" {
		return
	}

	if err := a.session.UpdateSessionMeta(a.ctx, sessionID, title, "", ""); err != nil {
		a.logger.Warn("更新标题失败", "err", err)
		return
	}

	wails.EventsEmit(a.ctx, "chat:title_updated", map[string]any{
		"session_id": sessionID,
		"title":      title,
	})
}

// ApproveTool 前端调用，响应审批请求。
func (a *App) ApproveTool(toolID string, approved bool, feedback string) error {
	return a.approvals.Complete(toolID, approved, feedback)
}

// SetApprovalMode 前端调用，切换审批模式并持久化。"auto" 自动批准，"manual" 等待用户操作。
func (a *App) SetApprovalMode(mode string) error {
	a.approvals.SetMode(mode)
	a.settings.ApprovalMode = mode
	return config.SaveSettings(a.db, a.settings)
}

// CancelChat 前端调用，取消一个正在进行的对话。
func (a *App) CancelChat(sessionID string) {
	a.agent.Cancel(sessionID)
}

// DeleteSession 删除指定会话及其所有消息。
func (a *App) DeleteSession(sessionID string) error {
	return a.session.DeleteSession(a.ctx, sessionID)
}

// CompressInput 是手动压缩请求的入参。
type CompressInput struct {
	SessionID    string `json:"session_id"`
	ProviderName string `json:"provider_name"`
	ModelID      string `json:"model_id"`
}

// CompressResult 是手动压缩请求的返回值。
type CompressResult struct {
	TurnID int `json:"turn_id"`
}

// CompressContext 手动触发上下文压缩。仅在无进行中 turn 时允许。
func (a *App) CompressContext(input CompressInput) (*CompressResult, error) {
	if a.agent.IsRunning(input.SessionID) {
		return nil, fmt.Errorf("对话进行中，无法手动压缩上下文，请等待当前对话完成")
	}

	// 1. 加载 Session
	var sess session.Session
	if err := a.session.DB.Where("session_id = ?", input.SessionID).First(&sess).Error; err != nil {
		return nil, fmt.Errorf("session 不存在: %w", err)
	}

	// 2. 查找模型
	model, ok := a.llmClient.ProviderModel(input.ProviderName, input.ModelID)
	if !ok {
		return nil, fmt.Errorf("模型未找到: %s/%s", input.ProviderName, input.ModelID)
	}

	// 3. 获取下一个 turn ID（手动压缩独立成一个 turn）
	turnID, err := a.session.NextTurn(context.Background(), sess.SessionID)
	if err != nil {
		return nil, fmt.Errorf("获取 turn ID 失败: %w", err)
	}

	// 4. 构建消息列表：全部来自 DB
	messages, err := a.loadAPIMessages(context.Background(), sess.SessionID, sess.ActiveVersion)
	if err != nil {
		return nil, fmt.Errorf("加载 API 消息失败: %w", err)
	}

	// 5. 创建上下文（含 cancel，支持打断）
	ctx, cancel := context.WithCancel(a.ctx)
	defer cancel()
	a.agent.RegisterCancel(sess.SessionID, cancel)
	defer a.agent.UnregisterCancel(sess.SessionID)

	// 6. 初始化 runningTokens
	runningTokens := a.agent.InitRunningTokens(messages)

	// 7. 执行压缩
	opts := agent.RunOptions{
		TurnID:        turnID,
		SessionID:     sess.SessionID,
		NovelID:       sess.NovelID,
		Messages:      messages,
		ActiveVersion: sess.ActiveVersion,
		Model:         model,
		ProviderName:  input.ProviderName,
		AgentType:     "main",
		MaxTurns:      50,
	}

	if err := a.agent.Compress(ctx, &opts, runningTokens); err != nil {
		return nil, err
	}
	return &CompressResult{TurnID: turnID}, nil
}

// commitUserChanges 在 turn 开始时提交用户在对话间隙对章节文件的手动修改。
// git 操作失败只记日志，不阻塞对话流程。
func (a *App) commitUserChanges(repo *git.Repo, turnID int, sessionID string) {
	has, err := repo.HasUncommitted()
	if err != nil {
		a.logger.Warn("auto-commit: 检查未提交变更失败", "err", err)
		return
	}
	if !has {
		return
	}

	msg := fmt.Sprintf("turn %d: user manual changes\n\nSession: %s", turnID, sessionID)
	a.doCommit(repo, turnID, sessionID, "user", msg)
}

// commitAIChanges 在 turn 结束时提交 AI 对章节文件的所有修改。
// 通过 defer 调用，确保正常结束、用户打断、异常退出时都会执行。
func (a *App) commitAIChanges(repo *git.Repo, turnID int, sessionID string, modelName string) {
	has, err := repo.HasUncommitted()
	if err != nil {
		a.logger.Warn("auto-commit: 检查未提交变更失败", "err", err)
		return
	}
	if !has {
		return
	}

	msg := fmt.Sprintf("turn %d: AI changes\n\nSession: %s\n\nCo-authored-by: Goink (%s)", turnID, sessionID, modelName)
	a.doCommit(repo, turnID, sessionID, "ai", msg)
}

// doCommit 执行 git add + commit，并将 hash 写入 turn_commits 表。
func (a *App) doCommit(repo *git.Repo, turnID int, sessionID, commitType, msg string) {
	if err := repo.StageAll(); err != nil {
		a.logger.Warn("auto-commit: git add 失败", "err", err)
		return
	}

	hash, err := repo.Commit(msg)
	if err != nil {
		a.logger.Warn("auto-commit: git commit 失败", "err", err)
		return
	}

	tc := &rollback.TurnCommit{
		SessionID:  sessionID,
		TurnID:     turnID,
		CommitType: commitType,
		CommitHash: hash,
	}
	if err := a.db.Create(tc).Error; err != nil {
		a.logger.Warn("auto-commit: 写入 turn_commits 失败", "err", err)
		return
	}

	a.logger.Info("auto-commit: 提交成功", "turn", turnID, "type", commitType, "hash", hash[:7])
}

// isChapterCreationIntent 检测用户消息是否包含章节创作意图（保留备用，当前未使用）。
func isChapterCreationIntent(msg string) bool {
	return true // 永远返回 true，门禁始终激活
}

// isBatchCreationIntent 检测用户消息是否包含批量创作意图（保留备用，当前未使用）。
func isBatchCreationIntent(msg string) bool {
	return false
}

// ChatFromAPI 供 API 服务器调用，通过 channel 推送事件。
// 直接复用 Chat 方法，EventCallback 将事件转到 channel。
func (a *App) ChatFromAPI(message string, novelID int64, modelKey, providerName string, events chan<- map[string]any, sessionID string) {
	// Panic recovery
	defer func() {
		if rec := recover(); rec != nil {
			a.logger.Error("ChatFromAPI panic", "err", rec)
			select {
			case events <- map[string]any{"type": "error", "error": fmt.Sprintf("服务器内部错误: %v", rec)}:
			default:
			}
		}
	}()

	// 安全检查
	if a.llmClient == nil || a.agent == nil {
		events <- map[string]any{"type": "error", "error": "服务未初始化，请重启 Goink"}
		return
	}
	if a.settings == nil {
		events <- map[string]any{"type": "error", "error": "配置未加载"}
		return
	}

	// 解析 provider/model
	provider, modelID := "", ""
	if idx := strings.IndexByte(modelKey, '/'); idx > 0 {
		provider = modelKey[:idx]
		modelID = modelKey[idx+1:]
	} else if providerName != "" {
		provider = providerName
		modelID = modelKey
	} else {
		provider = a.settings.SelectedModelKey
		if idx := strings.IndexByte(provider, '/'); idx > 0 {
			modelID = provider[idx+1:]
			provider = provider[:idx]
		}
	}

	if provider == "" || modelID == "" {
		events <- map[string]any{"type": "error", "error": "未配置模型，请先在电脑端设置模型"}
		return
	}

	// 构建 ChatInput（与电脑端完全一致）
	chatInput := ChatInput{
		SessionID:       sessionID,
		NovelID:         novelID,
		Message:         message,
		ProviderName:    provider,
		ModelID:         modelID,
		ReasoningEffort: "",
	}

	// EventCallback 将事件转到 channel
	callback := func(ev map[string]any) {
		select {
		case events <- ev:
		default:
		}
	}

	// 调用 ChatWithCallback（复用全部逻辑）
	_, err := a.ChatWithCallback(chatInput, callback)
	if err != nil {
		select {
		case events <- map[string]any{"type": "error", "error": err.Error()}:
		default:
		}
	}
}

func eventTypeString(t agent.AgentEventType) string {
	switch t {
	case agent.EventContent:
		return "content"
	case agent.EventThinking:
		return "thinking"
	case agent.EventThinkingDone:
		return "thinking_done"
	case agent.EventToolCall:
		return "tool_call"
	case agent.EventError:
		return "error"
	case agent.EventRetry:
		return "retry"
	case agent.EventPhaseGate:
		return "phase_gate"
	default:
		return "unknown"
	}
}
