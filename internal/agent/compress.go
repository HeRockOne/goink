package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	wails "github.com/wailsapp/wails/v2/pkg/runtime"
	"gorm.io/gorm"

	"novel/internal/agentcfg"
	"novel/internal/mcp_tools"
	"novel/internal/llm"
	"novel/internal/session"
)

const compressionPrompt = `<system-reminder>
你是上下文压缩助手。请基于完整对话历史生成结构化摘要，用于后续对话的上下文恢复。

## 已完成的事项
（每个一句话，最多 15 条，从最近的开始保留。不再重复执行的事项）

## 进行中（断点）
（最详细的部分：当前正在做什么、做到哪一步、下一步计划做什么。这是最重要的部分，请务必详尽）

## 用户偏好和要求
（从用户消息中提炼的核心写作风格、约束条件、反复强调的事项）

## 关键决策和设定变更
（已确认的情节走向、角色设定、世界观规则、命名等决定）

## 待办事项
（已计划但尚未开始的任务清单）
</system-reminder>`

const compressionReminder = "<system-reminder>\n上下文已压缩，请根据下面的摘要继续工作。\n</system-reminder>"

const maxUserRetain = 15
const minConversationTurns = 4

// generateSummary 调用 LLM 生成压缩摘要，返回摘要文本和应保留的历史消息。
// 缓存对齐：与主循环共享完全相同的 system+tools+历史前缀（fork 模式），
// 只在末尾追加压缩指令和动态 NS，压缩轮可从主会话缓存命中，而非全量重算。
func (a *Agent) generateSummary(ctx context.Context, opts *RunOptions) (string, []map[string]any, error) {
	msgs := make([]map[string]any, len(opts.Messages)+1)
	copy(msgs, opts.Messages)
	msgs[len(opts.Messages)] = map[string]any{"role": "user", "content": compressionPrompt}

	// 与主循环相同的全量工具定义，保证前缀字节一致
	tools := a.registry.OpenAI(nil)
	var sb strings.Builder
	stream := a.llm.ChatStream(ctx, opts.ProviderName, msgs, tools, opts.Model.ID, &llm.CallOptions{CacheKey: opts.SessionID})
	for ev := range stream {
		switch ev.Type {
		case llm.EventContent:
			sb.WriteString(ev.Data)
		case llm.EventError:
			a.logger.Warn("压缩摘要生成失败", "err", ev.Error)
			return "", nil, fmt.Errorf("compress: LLM summary failed: %w", ev.Error)
		}
	}

	summary := strings.TrimSpace(sb.String())
	if summary == "" {
		a.logger.Warn("压缩摘要为空，保持原上下文")
		return "", nil, fmt.Errorf("compress: empty summary")
	}

	a.logger.Debug("压缩摘要生成成功", "summary_len", len(summary))
	retained := retainMessages(msgs[:len(opts.Messages)])
	return summary, retained, nil
}

// Compress 执行主 Agent 上下文压缩：调 LLM 生成摘要，重建 SkillCatalog/NovelState，保留近期关键消息。
// opts 不会被修改，成功后才赋值新的 Messages / ActiveVersion / runningTokens。
func (a *Agent) Compress(ctx context.Context, opts *RunOptions, runningTokens map[string]int, pg *PhaseGate) error {
	a.logger.Info("开始上下文压缩",
		"session_id", opts.SessionID,
		"turn_id", opts.TurnID,
		"estimated_tokens", sumRunningTokens(runningTokens),
		"msg_count", len(opts.Messages),
	)

	a.emitCompression(ctx, opts.SessionID, opts.TurnID, "compressing", "", "")

	summary, retained, err := a.generateSummary(ctx, opts)
	if err != nil {
		return err
	}

	// 重建系统消息（顺序与 writeSystemMessages 一致）
	identity := agentcfg.AgentIdentity(agentcfg.MainAgent)
	var always string
	var catalog string
	if a.skillStore != nil {
		all := a.skillStore.ListMeta(opts.NovelID)
		catalog = agentcfg.BuildSkillCatalog(a.skillStore.ListMetaForCatalog(all))
		always = agentcfg.BuildAlwaysSkillsContent(all, a.skillStore, opts.NovelID)
	}
	novelState, err := agentcfg.NovelState(a.db, opts.NovelID)
	if err != nil {
		a.logger.Warn("压缩时 NovelState 构建失败", "novel_id", opts.NovelID, "err", err)
		novelState = ""
	}

	// 当前阶段必读技能全文：压缩清掉了历史中的技能消息，重建时补回。
	// 否则压缩后同阶段 set_phase 走"同阶段直接成功"分支不注入（readsByPhase 保留），
	// 创作指导缺失（旧实现依赖"后续 set_phase 自动恢复"，压缩后当前阶段内不成立）。
	phaseSkills := ""
	if pg != nil && pg.Active() {
		pc := pg.findPhase(pg.CurrentPhase())
		if pc != nil && len(pc.AutoSkillInjection) > 0 {
			if c, err := mcp_tools.BuildSkillsContent(a.skillStore, opts.NovelID, pc.AutoSkillInjection); err == nil {
				phaseSkills = c
			}
		}
	}

	// 在事务中完成版本递增 + 全部 DB 写入（NS 作为新版本末尾的消息落库，见 persistCompression）
	newVersion, err := a.persistCompression(ctx, opts, identity, always, catalog, novelState, summary, retained, phaseSkills)
	if err != nil {
		return fmt.Errorf("compress: persist failed: %w", err)
	}

	// 从 DB 加载新版本消息，与 Chat() 走同一条路径
	apiMsgs, err := a.session.GetMessagesForAPI(ctx, opts.SessionID, newVersion)
	if err != nil {
		return fmt.Errorf("compress: load messages after persist: %w", err)
	}
	opts.ActiveVersion = newVersion
	opts.Messages = make([]map[string]any, len(apiMsgs))
	for i, m := range apiMsgs {
		opts.Messages[i] = m.ToAPIFormat()
	}

	newTokens := a.InitRunningTokens(opts.Messages)
	clear(runningTokens)
	maps.Copy(runningTokens, newTokens)

	a.emitCompression(ctx, opts.SessionID, opts.TurnID, "done", summary, "")

	a.logger.Info("上下文压缩完成",
		"session_id", opts.SessionID,
		"new_version", newVersion,
		"retained", len(retained),
		"new_msg_count", len(opts.Messages),
	)
	return nil
}

// compressInMemory 执行子 Agent 上下文压缩：纯内存操作，AgentIdentity/NovelState 不动，仅写边界标记到 DB。
func (a *Agent) compressInMemory(ctx context.Context, opts *RunOptions, runningTokens map[string]int) error {
	a.logger.Info("子Agent上下文压缩",
		"session_id", opts.SessionID,
		"turn_id", opts.TurnID,
		"agent_type", opts.AgentType,
		"sub_task_id", opts.SubTaskID,
		"estimated_tokens", sumRunningTokens(runningTokens),
		"msg_count", len(opts.Messages),
	)

	a.emitCompression(ctx, opts.SessionID, opts.TurnID, "compressing", "", opts.SubTaskID)

	summary, retained, err := a.generateSummary(ctx, opts)
	if err != nil {
		return err
	}

	// 提取头部 system 消息，不动
	sysEnd := 0
	for i, m := range opts.Messages {
		role, _ := m["role"].(string)
		if role == "system" {
			sysEnd = i + 1
		} else {
			break
		}
	}
	sysMsgs := make([]map[string]any, sysEnd)
	copy(sysMsgs, opts.Messages[:sysEnd])

	// 内存重建 opts.Messages
	newMsgs := make([]map[string]any, 0, sysEnd+2+len(retained))
	newMsgs = append(newMsgs, sysMsgs...)
	newMsgs = append(newMsgs,
		map[string]any{"role": "user", "content": compressionReminder},
		map[string]any{"role": "user", "content": "<system-reminder>\n" + summary + "\n</system-reminder>"},
	)
	newMsgs = append(newMsgs, retained...)
	opts.Messages = newMsgs

	// 边界标记
	if err := a.db.Create(&session.Message{
		SessionID:  opts.SessionID,
		TurnID:     opts.TurnID,
		Role:       "system",
		Content:    "",
		Version:    opts.ActiveVersion,
		ToAPI:      false,
		ToFrontend: true,
		EventType:  "compression",
		AgentType:  opts.AgentType,
		SubTaskID:  opts.SubTaskID,
	}).Error; err != nil {
		a.logger.Warn("子Agent压缩边界标记写入失败", "err", err)
	}

	newTokens := a.InitRunningTokens(opts.Messages)
	clear(runningTokens)
	maps.Copy(runningTokens, newTokens)

	opts.SubAgentVersion++

	a.emitCompression(ctx, opts.SessionID, opts.TurnID, "done", summary, opts.SubTaskID)

	a.logger.Info("子Agent上下文压缩完成",
		"session_id", opts.SessionID,
		"sub_agent_version", opts.SubAgentVersion,
		"retained", len(retained),
		"new_msg_count", len(opts.Messages),
	)
	return nil
}

// persistCompression 在事务中递增 active_version 并写入所有压缩消息。
func (a *Agent) persistCompression(ctx context.Context, opts *RunOptions, identity, always, catalog, novelState, summary string, retained []map[string]any, phaseSkills string) (int, error) {
	var newVersion int

	err := a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sess session.Session
		if err := tx.First(&sess, "session_id = ?", opts.SessionID).Error; err != nil {
			return fmt.Errorf("查询 session 失败: %w", err)
		}
		sess.ActiveVersion++
		if err := tx.Save(&sess).Error; err != nil {
			return fmt.Errorf("递增 active_version 失败: %w", err)
		}
		newVersion = sess.ActiveVersion

		msg := func(role, content string, toAPI, toFE bool, eventType string) error {
			return tx.Create(&session.Message{
				SessionID:  opts.SessionID,
				TurnID:     opts.TurnID,
				Role:       role,
				Content:    content,
				Version:    newVersion,
				ToAPI:      toAPI,
				ToFrontend: toFE,
				EventType:  eventType,
				AgentType:  "main",
			}).Error
		}

		// AgentIdentity
		if err := msg("system", identity, true, false, ""); err != nil {
			return fmt.Errorf("write AgentIdentity: %w", err)
		}
		// AlwaysSkills
		if always != "" {
			if err := msg("system", always, true, false, ""); err != nil {
				return fmt.Errorf("write AlwaysSkills: %w", err)
			}
		}
		// SkillCatalog
		if catalog != "" {
			if err := msg("system", catalog, true, false, ""); err != nil {
				return fmt.Errorf("write SkillCatalog: %w", err)
			}
		}
		// 提醒语
		if err := msg("user", compressionReminder, true, false, ""); err != nil {
			return fmt.Errorf("write compression reminder: %w", err)
		}
		// 摘要
		if err := msg("user", "<system-reminder>\n"+summary+"\n</system-reminder>", true, false, ""); err != nil {
			return fmt.Errorf("write summary: %w", err)
		}
		// 保留消息副本
		for _, m := range retained {
			rm := apiMsgToMessage(m, opts.SessionID, opts.TurnID, newVersion)
			if err := tx.Create(rm).Error; err != nil {
				return fmt.Errorf("写入保留消息失败: %w", err)
			}
		}
		// 当前阶段必读技能全文（压缩清掉的创作指导，重建时补回）
		if phaseSkills != "" {
			if err := msg("system", phaseSkills, true, false, ""); err != nil {
				return fmt.Errorf("write phase skills: %w", err)
			}
		}
		// 最新 NS 快照落库到新版本末尾（缓存协议：NS 作为消息进历史、永不清理；
		// 压缩后第一轮请求 = [fp][reminder][summary][retained][NS]，与压缩请求
		// [fp][历史][compressionPrompt] 前缀不同属一次性重建成本，之后恢复完整匹配）
		if novelState != "" {
			if err := tx.Create(&session.Message{
				SessionID:     opts.SessionID,
				TurnID:        opts.TurnID,
				Role:          "system",
				Content:       novelState,
				Version:       newVersion,
				ToAPI:         true,
				ToFrontend:    false,
				AgentType:     "main",
				ExtraMetadata: agentcfg.NovelStateKindJSON,
			}).Error; err != nil {
				return fmt.Errorf("write NovelState: %w", err)
			}
		}
		// 边界标记
		if err := msg("system", "", false, true, "compression"); err != nil {
			return fmt.Errorf("write compression marker: %w", err)
		}

		return nil
	})

	return newVersion, err
}

// apiMsgToMessage 将 API 格式的 map 反向转换为 session.Message，提取 ExtraMetadata。
func apiMsgToMessage(m map[string]any, sessionID string, turnID, version int) *session.Message {
	role, _ := m["role"].(string)
	content, _ := m["content"].(string)

	msg := &session.Message{
		SessionID:  sessionID,
		TurnID:     turnID,
		Role:       role,
		Content:    content,
		Version:    version,
		ToAPI:      true,
		ToFrontend: false,
		AgentType:  "main",
	}

	if tc, ok := m["reasoning_content"].(string); ok {
		msg.ThinkingContent = tc
	}

	meta := make(map[string]any)
	switch role {
	case "assistant":
		if tc, ok := m["tool_calls"]; ok && tc != nil {
			meta["tool_calls"] = tc
		}
	case "tool":
		if id, ok := m["tool_call_id"]; ok && id != nil {
			meta["tool_call_id"] = id
		}
		if name, ok := m["name"]; ok && name != nil {
			meta["tool_name"] = name
		}
	}
	if len(meta) > 0 {
		b, _ := json.Marshal(meta)
		msg.ExtraMetadata = string(b)
	}

	return msg
}

// emitCompression 推送压缩事件到前端。
// 事件名带 session_id，与 agent.go emit 保持一致（turn_id 按会话独立递增，不带会话前缀会串台）。
func (a *Agent) emitCompression(ctx context.Context, sessionID string, turnID int, phase, summary, subTaskID string) {
	wails.EventsEmit(ctx, "agent:"+sessionID+":"+strconv.Itoa(turnID), AgentEvent{
		TurnID:           turnID,
		Type:             EventCompression,
		CompressionPhase: phase,
		Summary:          summary,
		SubTaskID:        subTaskID,
		Timestamp:        time.Now(),
	})
}

// retainMessages 筛选应保留的历史消息。
// 跳过前 N 条 system 消息，后续应用保留规则。
func retainMessages(messages []map[string]any) []map[string]any {
	if len(messages) == 0 {
		return nil
	}

	// 跳过前部 system 消息
	sysEnd := 0
	for _, m := range messages {
		role, _ := m["role"].(string)
		if role == "system" {
			sysEnd++
		} else {
			break
		}
	}

	history := messages[sysEnd:]
	if len(history) == 0 {
		return nil
	}

	// 找到所有 user 消息的位置
	userIdx := make([]int, 0)
	for i, m := range history {
		role, _ := m["role"].(string)
		if role == "user" {
			userIdx = append(userIdx, i)
		}
	}

	if len(userIdx) == 0 {
		return nil
	}

	// 保留最近 maxUserRetain 条 user 消息
	keepFrom := 0
	if len(userIdx) > maxUserRetain {
		keepFrom = userIdx[len(userIdx)-maxUserRetain]
	}

	// 确保至少保留 minConversationTurns 轮对话
	if len(userIdx) >= minConversationTurns {
		minKeep := userIdx[len(userIdx)-minConversationTurns]
		if minKeep < keepFrom {
			keepFrom = minKeep
		}
	}

	retained := make([]map[string]any, 0, len(history)-keepFrom)
	for _, m := range history[keepFrom:] {
		// NS 快照不进入保留副本（压缩后统一重新落库一份，见 persistCompression）
		if role, _ := m["role"].(string); role == "system" {
			if content, ok := m["content"].(string); ok && strings.HasPrefix(content, agentcfg.NovelStatePrefix) {
				continue
			}
		}
		dup := make(map[string]any, len(m))
		maps.Copy(dup, m)
		retained = append(retained, dup)
	}

	return retained
}

// IsRunning 检查指定 session 是否有正在进行的 turn。
func (a *Agent) IsRunning(sessionID string) bool {
	return a.cancelMgr.IsRegistered(CancelPrefixChat + sessionID)
}
