package lore

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

func (s *Store) ListByNovel(ctx context.Context, novelID int64, opts ListOptions) (*storage.PageResult[LoreEntry], error) {
	pp := (&storage.PageParams{Page: opts.Page, Size: opts.Size}).Normalize()
	query := s.DB.WithContext(ctx).Where("novel_id = ?", novelID)
	if opts.Category != "" {
		query = query.Where("category = ?", opts.Category)
	}
	if opts.Search != "" {
		query = query.Where("title LIKE ? OR content LIKE ?", "%"+opts.Search+"%", "%"+opts.Search+"%")
	}
	var total int64
	if err := query.Model(&LoreEntry{}).Count(&total).Error; err != nil {
		return nil, fmt.Errorf("lore count: %w", err)
	}
	var items []LoreEntry
	offset := (pp.Page - 1) * pp.Size
	if err := query.Order("category ASC, title ASC").Offset(offset).Limit(pp.Size).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("lore list: %w", err)
	}
	return storage.NewPageResult(items, total, pp.Page, pp.Size), nil
}

func (s *Store) GetByID(ctx context.Context, id, novelID int64) (*LoreEntry, error) {
	var e LoreEntry
	if err := s.DB.WithContext(ctx).Where("id = ? AND novel_id = ?", id, novelID).First(&e).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *Store) Create(ctx context.Context, e *LoreEntry) error {
	return s.DB.WithContext(ctx).Create(e).Error
}

func (s *Store) Update(ctx context.Context, e *LoreEntry) error {
	return s.DB.WithContext(ctx).Model(e).Updates(map[string]any{
		"title": e.Title, "category": e.Category, "content": e.Content,
		"summary": e.Summary, "arc_id": e.ArcID, "reveal_chapter_id": e.RevealChapterID,
		"is_public": e.IsPublic, "reference_id": e.ReferenceID,
		"reference_type": e.ReferenceType, "tags": e.Tags, "version": gorm.Expr("version + 1"),
	}).Error
}

func (s *Store) Delete(ctx context.Context, id, novelID int64) error {
	return s.DB.WithContext(ctx).Where("id = ? AND novel_id = ?", id, novelID).Delete(&LoreEntry{}).Error
}

func (s *Store) Search(ctx context.Context, novelID int64, query string, limit int) ([]LoreEntry, error) {
	if limit <= 0 {
		limit = 10
	}
	var items []LoreEntry
	if err := s.DB.WithContext(ctx).Where("novel_id = ? AND (title LIKE ? OR content LIKE ? OR summary LIKE ?)", novelID, "%"+query+"%", "%"+query+"%", "%"+query+"%").
		Order("updated_at DESC").Limit(limit).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

type ListOptions struct {
	Page     int
	Size     int
	Category string
	Search   string
}
