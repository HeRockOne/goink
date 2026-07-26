package app

import "novel/internal/stats"

func (a *App) GetNovelStats(novelID int64) (*stats.NovelStats, error) {
	return a.stats.GetNovelStats(a.ctx, novelID)
}
