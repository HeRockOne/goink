package reader

import (
	"context"
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"novel/internal/storage"
)

// Store 管理 ReaderPerspective 持久化。DB 导出供调用方做简单 CRUD。
type Store struct {
	DB     *gorm.DB
	logger *slog.Logger
}

// NewStore 创建 reader 存储。
func NewStore(db *gorm.DB, logger *slog.Logger) *Store {
	return &Store{DB: db, logger: logger}
}

// ListByNovelOptions 是 ListByNovel 的可选参数。
type ListByNovelOptions struct {
	PageParams storage.PageParams
	Type       string // 空字符串=不过滤，"known"/"suspense"/"misconception"
}

// ListByNovel 分页列出某小说的读者认知条目，支持按类型过滤。前端管理页用。
func (s *Store) ListByNovel(ctx context.Context, novelID int64, opts ListByNovelOptions) (*storage.PageResult[ReaderPerspective], error) {
	pp := opts.PageParams
	pp.Normalize()

	q := s.DB.WithContext(ctx).Model(&ReaderPerspective{}).Where("novel_id = ?", novelID)

	if opts.Type != "" {
		q = q.Where("type = ?", opts.Type)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("reader store: count: %w", err)
	}

	var items []ReaderPerspective
	offset := (pp.Page - 1) * pp.Size
	if err := q.Order("type, planted_chapter ASC").Offset(offset).Limit(pp.Size).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("reader store: list: %w", err)
	}

	s.logger.Debug("reader store: listed", "novel_id", novelID, "total", total, "page", pp.Page)
	return storage.NewPageResult(items, total, pp.Page, pp.Size), nil
}

// ListActive 返回未回收（revealed_chapter=0）的读者认知条目。
// 硬上限 100 条（按种植章节升序）——悬念只种不收会随章节线性增长，
// 全量返回会撑爆上下文；最近的条目优先保留（counts 用 Count 查询，不受截断影响）。
func (s *Store) ListActive(ctx context.Context, novelID int64) ([]ReaderPerspective, error) {
	var items []ReaderPerspective
	if err := s.DB.WithContext(ctx).
		Where("novel_id = ? AND revealed_chapter = 0", novelID).
		Order("type, planted_chapter ASC").
		Limit(100).
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("reader store: list active: %w", err)
	}
	return items, nil
}

// ListActiveFiltered 返回未回收的读者认知条目，支持定向过滤（search 关键词 + 种植章节范围），
// 硬上限 100 条。定向查询替代"全量拉取后自己筛"——省 token 且不丢目标信息。
func (s *Store) ListActiveFiltered(ctx context.Context, novelID int64, search string, plantedFrom, plantedTo int) ([]ReaderPerspective, error) {
	q := s.DB.WithContext(ctx).
		Where("novel_id = ? AND revealed_chapter = 0", novelID)
	if search != "" {
		q = q.Where("content LIKE ?", "%"+search+"%")
	}
	if plantedFrom > 0 {
		q = q.Where("planted_chapter >= ?", plantedFrom)
	}
	if plantedTo > 0 {
		q = q.Where("planted_chapter <= ?", plantedTo)
	}
	var items []ReaderPerspective
	if err := q.Order("type, planted_chapter ASC").Limit(100).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("reader store: list active filtered: %w", err)
	}
	return items, nil
}
