package app

import "novel/internal/review"

// GetReviewRecords 获取指定小说的审稿记录（按时间倒序，最新在前）。
func (a *App) GetReviewRecords(novelID int64, limit int) ([]review.ReviewRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var records []review.ReviewRecord
	err := a.db.Where("novel_id = ?", novelID).Order("created_at DESC, id DESC").Limit(limit).Find(&records).Error
	return records, err
}
