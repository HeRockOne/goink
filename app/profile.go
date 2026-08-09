package app

import (
	"encoding/json"
	"time"

	"novel/internal/chapter"
	"novel/internal/config"
	"novel/internal/novel"
	"novel/internal/session"
	"novel/internal/writing"
)

// GetWritingSnapshot 返回当前写作快照（active_chars / current_arc_id / current_location 等）。
func (a *App) GetWritingSnapshot(novelID int64) (*writing.WritingSnapshot, error) {
	snapStore := writing.NewSnapshotStore(a.db, a.logger)
	return snapStore.Get(a.ctx, novelID)
}

// GetWritingActivity 返回最近 months 个月每日写作字数汇总。
func (a *App) GetWritingActivity(months int) ([]writing.DailyActivity, error) {
	return a.writing.GetDailyActivity(a.ctx, months)
}

// GetWritingStats 返回全局写作统计，跨所有小说。
func (a *App) GetWritingStats() (*writing.WritingStats, error) {
	novels, err := a.novel.List(a.ctx, novel.ListNovelsOptions{})
	if err != nil {
		return nil, err
	}

	var chapterCount int64
	a.db.WithContext(a.ctx).Model(&chapter.Chapter{}).Count(&chapterCount)

	return a.writing.GetWritingStats(a.ctx, novels.Total, chapterCount)
}

// DailyTokenUsage 每日 token 用量（按模型拆分）。
type DailyTokenUsage struct {
	Date       string  `json:"date"`
	HitTokens  float64 `json:"hit_tokens"`
	MissTokens float64 `json:"miss_tokens"`
	Completion float64 `json:"completion"`
	Cost       float64 `json:"cost"`
	Model      string  `json:"model"`
}

// GetTokenUsageTrend 返回最近 days 天每日 token 消耗趋势，按模型 + 日期聚合。
func (a *App) GetTokenUsageTrend(days int) ([]DailyTokenUsage, error) {
	var msgs []session.Message
	cutoff := time.Now().AddDate(0, 0, -days)
	if err := a.db.WithContext(a.ctx).
		Where("role = 'assistant' AND created_at >= ?", cutoff).
		Order("created_at ASC").
		Find(&msgs).Error; err != nil {
		return nil, err
	}

	type dayModelAccum struct {
		hit, miss, comp float64
	}

	modelCache := make(map[string]string) // sessionID -> model
	byDateModel := make(map[string]map[string]*dayModelAccum)

	for _, msg := range msgs {
		if msg.ExtraMetadata == "" {
			continue
		}
		var meta map[string]any
		if err := json.Unmarshal([]byte(msg.ExtraMetadata), &meta); err != nil {
			continue
		}
		usage, ok := meta["usage"].(map[string]any)
		if !ok {
			continue
		}

		date := msg.CreatedAt.Format("2006-01-02")
		modelID, _ := usage["model"].(string)
		if modelID == "" {
			if m, ok := modelCache[msg.SessionID]; ok {
				modelID = m
			} else {
				var sess session.Session
				if err := a.db.WithContext(a.ctx).Select("model").Where("session_id = ?", msg.SessionID).First(&sess).Error; err == nil {
					modelID = sess.Model
					modelCache[msg.SessionID] = modelID
				}
			}
		}
		if modelID == "" {
			modelID = "unknown"
		}

		if byDateModel[date] == nil {
			byDateModel[date] = make(map[string]*dayModelAccum)
		}
		acc := byDateModel[date][modelID]
		if acc == nil {
			acc = &dayModelAccum{}
			byDateModel[date][modelID] = acc
		}
		prompt, _ := usage["prompt_tokens"].(float64)
		cached, _ := usage["cached_tokens"].(float64)
		comp, _ := usage["completion_tokens"].(float64)
		acc.hit += cached
		miss := prompt - cached
		if miss < 0 {
			miss = 0
		}
		acc.miss += miss
		acc.comp += comp
	}

	cachePrice := 0.02
	inputPrice := 1.0
	outputPrice := 2.0
	if s, err := config.LoadSettings(a.db); err == nil && s != nil {
		cachePrice = s.CachePrice
		inputPrice = s.PriceInput
		outputPrice = s.PriceOutput
	}

	result := make([]DailyTokenUsage, 0)
	for date, models := range byDateModel {
		for modelID, acc := range models {
			cost := acc.hit*cachePrice/1_000_000 +
				acc.miss*inputPrice/1_000_000 +
				acc.comp*outputPrice/1_000_000
			result = append(result, DailyTokenUsage{
				Date:       date,
				HitTokens:  acc.hit,
				MissTokens: acc.miss,
				Completion: acc.comp,
				Cost:       cost,
				Model:      modelID,
			})
		}
	}
	return result, nil
}
