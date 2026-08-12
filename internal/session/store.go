package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"novel/internal/storage"

	"gorm.io/gorm"
)

// Store 管理 Session/Message 持久化。DB 导出供调用方做简单 CRUD（Create/First/Append）。
type Store struct {
	DB     *gorm.DB
	logger *slog.Logger
}

// NewStore 创建 session 存储。
func NewStore(db *gorm.DB, logger *slog.Logger) *Store {
	return &Store{DB: db, logger: logger}
}

// ========== Session 查询 ==========

// GetSession 按 session_id 加载单个 session。
func (s *Store) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	var sess Session
	if err := s.DB.WithContext(ctx).Where("session_id = ?", sessionID).First(&sess).Error; err != nil {
		return nil, fmt.Errorf("session store: get session: %w", err)
	}
	return &sess, nil
}

// ListSessionsOptions 是 ListSessions 的可选参数。
type ListSessionsOptions struct {
	PageParams storage.PageParams
	Search     string // 空=全部，非空=按消息内容 LIKE 模糊匹配
}

// ListSessions 按小说列出会话，updated_at 倒序，分页。Search 非空时搜索消息内容。
func (s *Store) ListSessions(ctx context.Context, novelID int64, opts ListSessionsOptions) (*storage.PageResult[Session], error) {
	pp := opts.PageParams
	pp.Normalize()

	if opts.Search == "" {
		return s.listAll(ctx, novelID, pp)
	}
	return s.search(ctx, novelID, opts.Search)
}

func (s *Store) listAll(ctx context.Context, novelID int64, pp storage.PageParams) (*storage.PageResult[Session], error) {
	q := s.DB.WithContext(ctx).Model(&Session{}).Where("novel_id = ?", novelID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("session store: count sessions: %w", err)
	}

	var sessions []Session
	offset := (pp.Page - 1) * pp.Size
	if err := q.Order("updated_at DESC").Offset(offset).Limit(pp.Size).Find(&sessions).Error; err != nil {
		return nil, fmt.Errorf("session store: list sessions: %w", err)
	}

	s.logger.Debug("session store: listed sessions", "novel_id", novelID, "total", total, "page", pp.Page)
	return storage.NewPageResult(sessions, total, pp.Page, pp.Size), nil
}

func (s *Store) search(ctx context.Context, novelID int64, search string) (*storage.PageResult[Session], error) {
	var sessions []Session
	if err := s.DB.WithContext(ctx).
		Distinct("sessions.*").
		Joins("JOIN messages ON messages.session_id = sessions.session_id").
		Where("sessions.novel_id = ? AND messages.content LIKE ?", novelID, "%"+search+"%").
		Order("sessions.updated_at DESC").
		Limit(100).
		Find(&sessions).Error; err != nil {
		return nil, fmt.Errorf("session store: search sessions: %w", err)
	}

	total := int64(len(sessions))
	s.logger.Debug("session store: searched sessions", "novel_id", novelID, "search", search, "total", total)
	return storage.NewPageResult(sessions, total, 1, 100), nil
}

// ========== Session 更新 ==========

// UpdateSessionMeta 增量更新标题、模型、推理深度。空字符串不更新。
func (s *Store) UpdateSessionMeta(ctx context.Context, sessionID, title, model, reasoningEffort string) error {
	if title == "" && model == "" && reasoningEffort == "" {
		return nil
	}

	var sess Session
	if err := s.DB.WithContext(ctx).
		Where("session_id = ?", sessionID).
		First(&sess).Error; err != nil {
		return fmt.Errorf("session store: update meta: %w", err)
	}

	if title != "" {
		sess.Title = title
	}
	if model != "" {
		sess.Model = model
	}
	if reasoningEffort != "" {
		sess.ReasoningEffort = reasoningEffort
	}

	if err := s.DB.WithContext(ctx).Save(&sess).Error; err != nil {
		return fmt.Errorf("session store: update meta: %w", err)
	}
	return nil
}

// UpdateSessionUsage 更新最近一次 LLM 的 token 用量。
func (s *Store) UpdateSessionUsage(ctx context.Context, sessionID, usageJSON string) error {
	var sess Session
	if err := s.DB.WithContext(ctx).
		Where("session_id = ?", sessionID).
		First(&sess).Error; err != nil {
		return fmt.Errorf("session store: update usage: %w", err)
	}

	sess.Usage = usageJSON

	if err := s.DB.WithContext(ctx).Save(&sess).Error; err != nil {
		return fmt.Errorf("session store: update usage: %w", err)
	}
	return nil
}

// UpdateMessageUsage 将 API 返回的精确 token 用量保存到指定 agent 的最后一条 assistant 消息的 ExtraMetadata。
// agentType 区分主 agent 与子 agent，避免同 turn 内互相覆盖导致统计口径漂移。
func (s *Store) UpdateMessageUsage(ctx context.Context, sessionID string, turnID int, agentType string, usage map[string]any) error {
	if agentType == "" {
		agentType = "main"
	}
	var msg Message
	if err := s.DB.WithContext(ctx).
		Where("session_id = ? AND turn_id = ? AND role = 'assistant' AND agent_type = ?", sessionID, turnID, agentType).
		Order("id DESC").Limit(1).First(&msg).Error; err != nil {
		return fmt.Errorf("session store: find message: %w", err)
	}

	var meta map[string]any
	if msg.ExtraMetadata != "" {
		if err := json.Unmarshal([]byte(msg.ExtraMetadata), &meta); err != nil {
			meta = make(map[string]any)
		}
	}
	if meta == nil {
		meta = make(map[string]any)
	}
	meta["usage"] = usage

	b, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("session store: marshal usage: %w", err)
	}
	return s.DB.WithContext(ctx).Model(&msg).Update("extra_metadata", string(b)).Error
}

// GetSessionCumulativeUsage 查询 session 内所有消息的累计 token 用量。
// 从每条 assistant 消息的 ExtraMetadata.usage 中提取并累加。
func (s *Store) GetSessionCumulativeUsage(ctx context.Context, sessionID string) map[string]float64 {
	var msgs []Message
	s.DB.WithContext(ctx).
		Where("session_id = ? AND role = 'assistant'", sessionID).
		Order("id ASC").
		Find(&msgs)

	accHit, accMiss, accCompletion := float64(0), float64(0), float64(0)
	for _, msg := range msgs {
		if msg.ExtraMetadata == "" {
			continue
		}
		var meta map[string]any
		if err := json.Unmarshal([]byte(msg.ExtraMetadata), &meta); err != nil {
			continue
		}
		raw, ok := meta["usage"].(map[string]any)
		if !ok {
			continue
		}
		if v, _ := raw["cached_tokens"].(float64); v > 0 {
			accHit += v
		}
		if v, _ := raw["completion_tokens"].(float64); v > 0 {
			accCompletion += v
		}
		if v, _ := raw["prompt_tokens"].(float64); v > 0 {
			miss := v
			if cached, _ := raw["cached_tokens"].(float64); cached > 0 {
				miss -= cached
			}
			if miss > 0 {
				accMiss += miss
			}
		}
	}

	return map[string]float64{
		"prompt_cache_hit_tokens":  accHit,
		"prompt_cache_miss_tokens": accMiss,
		"acc_completion_tokens":    accCompletion,
	}
}

// BumpActiveVersion 递增 active_version 并返回新值。
func (s *Store) BumpActiveVersion(ctx context.Context, sessionID string) (int, error) {
	var sess Session
	if err := s.DB.WithContext(ctx).
		Where("session_id = ?", sessionID).
		First(&sess).Error; err != nil {
		return 0, fmt.Errorf("session store: bump version: %w", err)
	}

	sess.ActiveVersion++

	if err := s.DB.WithContext(ctx).Save(&sess).Error; err != nil {
		return 0, fmt.Errorf("session store: bump version: %w", err)
	}

	newV := sess.ActiveVersion
	s.logger.Debug("session store: bumped version", "session_id", sessionID, "new_version", newV)
	return newV, nil
}

// ========== Message 查询 ==========

// NextTurn 原子递增 last_turn_id 并返回新值。
// agent loop 在每个 turn 开始时调用，一步完成递增 + 持久化。
func (s *Store) NextTurn(ctx context.Context, sessionID string) (int, error) {
	var turnID int
	if err := s.DB.WithContext(ctx).
		Raw("UPDATE sessions SET last_turn_id = last_turn_id + 1, updated_at = ? WHERE session_id = ? RETURNING last_turn_id", time.Now(), sessionID).
		Scan(&turnID).Error; err != nil {
		return 0, fmt.Errorf("session store: next turn: %w", err)
	}

	s.logger.Debug("session store: next turn", "session_id", sessionID, "turn_id", turnID)
	return turnID, nil
}

// GetMessagesForAPI 返回 LLM context 所需的消息。
// 用 DESC+反转取最新 1000 条：旧实现 ORDER BY id ASC LIMIT 1000 取的是最早的 1000 条，
// 消息数超限时本轮刚写入的 user 消息与 NS 快照（id 尾部）被截断，agent 第一轮无用户输入。
func (s *Store) GetMessagesForAPI(ctx context.Context, sessionID string, version int) ([]Message, error) {
	var msgs []Message
	if err := s.DB.WithContext(ctx).
		Where("session_id = ? AND to_api = ? AND version = ?", sessionID, true, version).
		Order("id DESC").
		Limit(1000).
		Find(&msgs).Error; err != nil {
		return nil, fmt.Errorf("session store: get api messages: %w", err)
	}
	// 反转回 id 升序（历史顺序是请求前缀顺序）
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

// GetMessagesForFrontend 返回前端展示所需的消息。
func (s *Store) GetMessagesForFrontend(ctx context.Context, sessionID string) ([]Message, error) {
	var msgs []Message
	if err := s.DB.WithContext(ctx).
		Where("session_id = ? AND to_frontend = ?", sessionID, true).
		Order("id ASC").
		Find(&msgs).Error; err != nil {
		return nil, fmt.Errorf("session store: get frontend messages: %w", err)
	}
	return msgs, nil
}

// GetAllMessages 返回全部消息，审计/回退用。
func (s *Store) GetAllMessages(ctx context.Context, sessionID string) ([]Message, error) {
	var msgs []Message
	if err := s.DB.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("id ASC").
		Find(&msgs).Error; err != nil {
		return nil, fmt.Errorf("session store: get all messages: %w", err)
	}
	return msgs, nil
}

// DeleteSession 删除指定 session 及其所有消息。
func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session_id = ?", sessionID).Delete(&Message{}).Error; err != nil {
			return fmt.Errorf("session store: delete messages: %w", err)
		}
		if err := tx.Where("session_id = ?", sessionID).Delete(&Session{}).Error; err != nil {
			return fmt.Errorf("session store: delete session: %w", err)
		}
		return nil
	})
}

// SavePhaseGateState 保存阶段门禁状态到 session（current_phase + called_tools）。
func (s *Store) SavePhaseGateState(sessionID, currentPhase, calledToolsJSON string) {
	if err := s.DB.Model(&Session{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]any{
			"current_phase": currentPhase,
			"called_tools":  calledToolsJSON,
		}).Error; err != nil {
		s.logger.Warn("保存阶段门禁状态失败", "session_id", sessionID, "err", err)
	}
}

// SavePhaseGateMode 持久化门禁模式（single/batch）。跨 turn 必须恢复，
// 否则批量会话第二条消息后 chat.go 只按当前消息判断批量意图，退化成 single。
func (s *Store) SavePhaseGateMode(sessionID, mode string) {
	if err := s.DB.Model(&Session{}).
		Where("session_id = ?", sessionID).
		Update("phase_mode", mode).Error; err != nil {
		s.logger.Warn("保存门禁模式失败", "session_id", sessionID, "err", err)
	}
}

// UpsertModelUsage 更新或插入模型级 token 消耗累计。
func (s *Store) UpsertModelUsage(ctx context.Context, sessionID, modelID string, hit, miss, comp float64) error {
	var existing ModelUsage
	if err := s.DB.WithContext(ctx).
		Where("session_id = ? AND model_id = ?", sessionID, modelID).
		First(&existing).Error; err != nil {
		// 插入
		return s.DB.WithContext(ctx).Create(&ModelUsage{
			SessionID:        sessionID,
			ModelID:          modelID,
			HitTokens:        hit,
			MissTokens:       miss,
			CompletionTokens: comp,
		}).Error
	}
	// 更新
	return s.DB.WithContext(ctx).Model(&existing).Updates(map[string]any{
		"hit_tokens":        existing.HitTokens + hit,
		"miss_tokens":       existing.MissTokens + miss,
		"completion_tokens": existing.CompletionTokens + comp,
	}).Error
}
