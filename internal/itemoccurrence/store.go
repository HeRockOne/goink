package itemoccurrence

import (
	"context"
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"novel/internal/storage"
)

type Store struct {
	DB     *gorm.DB
	logger *slog.Logger
}

func NewStore(db *gorm.DB, logger *slog.Logger) *Store {
	return &Store{DB: db, logger: logger}
}

// ListByItem 返回某物品的出现记录，按章节号倒序（最近的优先），硬上限 50 条——
// 核心道具每章出现时记录随章节线性增长，全量返回会撑爆上下文（MCP 工具路径）；
// 旧记录按需翻页。
func (s *Store) ListByItem(ctx context.Context, novelID, itemID int64) ([]ItemOccurrence, error) {
	var items []ItemOccurrence
	if err := s.DB.WithContext(ctx).
		Where("novel_id = ? AND item_id = ?", novelID, itemID).
		Order("chapter_id DESC").
		Limit(50).
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("itemoccurrence list by item: %w", err)
	}
	return items, nil
}

// ListAllByItem 返回某物品的全部出现记录（无上限）——仅前端/API 展示用（非 LLM 上下文）。
func (s *Store) ListAllByItem(ctx context.Context, novelID, itemID int64) ([]ItemOccurrence, error) {
	var items []ItemOccurrence
	if err := s.DB.WithContext(ctx).
		Where("novel_id = ? AND item_id = ?", novelID, itemID).
		Order("chapter_id ASC").
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("itemoccurrence list all by item: %w", err)
	}
	return items, nil
}

// ListByChapter 返回某章节的全部物品出现记录。
func (s *Store) ListByChapter(ctx context.Context, novelID, chapterID int64) ([]ItemOccurrence, error) {
	var items []ItemOccurrence
	if err := s.DB.WithContext(ctx).
		Where("novel_id = ? AND chapter_id = ?", novelID, chapterID).
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("itemoccurrence list by chapter: %w", err)
	}
	return items, nil
}

// Create 创建一条物品出现记录。
func (s *Store) Create(ctx context.Context, o *ItemOccurrence) error {
	if err := s.DB.WithContext(ctx).Create(o).Error; err != nil {
		return fmt.Errorf("itemoccurrence create: %w", err)
	}
	return nil
}

// Delete 删除一条物品出现记录。
func (s *Store) Delete(ctx context.Context, id, novelID int64) error {
	if err := s.DB.WithContext(ctx).Where("id = ? AND novel_id = ?", id, novelID).Delete(&ItemOccurrence{}).Error; err != nil {
		return fmt.Errorf("itemoccurrence delete: %w", err)
	}
	return nil
}

// ListByNovel 分页列出某小说的全部物品出现记录。
func (s *Store) ListByNovel(ctx context.Context, novelID int64, page storage.PageParams) (*storage.PageResult[ItemOccurrence], error) {
	page.Normalize()
	var total int64
	if err := s.DB.WithContext(ctx).Model(&ItemOccurrence{}).Where("novel_id = ?", novelID).Count(&total).Error; err != nil {
		return nil, fmt.Errorf("itemoccurrence count: %w", err)
	}
	var items []ItemOccurrence
	offset := (page.Page - 1) * page.Size
	if err := s.DB.WithContext(ctx).Where("novel_id = ?", novelID).Order("chapter_id ASC").Offset(offset).Limit(page.Size).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("itemoccurrence list: %w", err)
	}
	return storage.NewPageResult(items, total, page.Page, page.Size), nil
}
