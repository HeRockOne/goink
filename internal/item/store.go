package item

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

func (s *Store) ListByNovel(ctx context.Context, novelID int64, opts ListOptions) (*storage.PageResult[Item], error) {
	pp := (&storage.PageParams{Page: opts.Page, Size: opts.Size}).Normalize()
	query := s.DB.WithContext(ctx).Where("novel_id = ?", novelID)
	if opts.ItemType != "" {
		query = query.Where("item_type = ?", opts.ItemType)
	}
	if opts.Status != "" {
		query = query.Where("status = ?", opts.Status)
	} else {
		query = query.Where("status != ?", "destroyed")
	}
	if opts.Search != "" {
		query = query.Where("name LIKE ? OR description LIKE ?", "%"+opts.Search+"%", "%"+opts.Search+"%")
	}
	var total int64
	if err := query.Model(&Item{}).Count(&total).Error; err != nil {
		return nil, fmt.Errorf("item count: %w", err)
	}
	var items []Item
	offset := (pp.Page - 1) * pp.Size
	if err := query.Order("item_type ASC, name ASC").Offset(offset).Limit(pp.Size).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("item list: %w", err)
	}
	return storage.NewPageResult(items, total, pp.Page, pp.Size), nil
}

func (s *Store) GetByID(ctx context.Context, id, novelID int64) (*Item, error) {
	var it Item
	if err := s.DB.WithContext(ctx).Where("id = ? AND novel_id = ?", id, novelID).First(&it).Error; err != nil {
		return nil, err
	}
	return &it, nil
}

func (s *Store) Create(ctx context.Context, it *Item) error {
	return s.DB.WithContext(ctx).Create(it).Error
}

func (s *Store) Update(ctx context.Context, it *Item) error {
	return s.DB.WithContext(ctx).Model(it).Updates(map[string]any{
		"name": it.Name, "item_type": it.ItemType, "grade": it.Grade,
		"description": it.Description, "lore": it.Lore, "ability": it.Ability,
		"arc_id": it.ArcID, "first_chapter_id": it.FirstChapterID,
		"status_changed_chapter_id": it.StatusChangedChapterID,
		"narrative_role": it.NarrativeRole, "owner_id": it.OwnerID,
		"previous_owner_id": it.PreviousOwnerID, "location_id": it.LocationID,
		"status": it.Status, "tags": it.Tags,
	}).Error
}

func (s *Store) Delete(ctx context.Context, id, novelID int64) error {
	return s.DB.WithContext(ctx).Where("id = ? AND novel_id = ?", id, novelID).Delete(&Item{}).Error
}

func (s *Store) Search(ctx context.Context, novelID int64, query string, limit int) ([]Item, error) {
	if limit <= 0 {
		limit = 10
	}
	var items []Item
	if err := s.DB.WithContext(ctx).Where("novel_id = ? AND (name LIKE ? OR description LIKE ? OR ability LIKE ? OR lore LIKE ?)", novelID, "%"+query+"%", "%"+query+"%", "%"+query+"%", "%"+query+"%").
		Order("updated_at DESC").Limit(limit).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

type ListOptions struct {
	Page     int
	Size     int
	ItemType string
	Status   string
	Search   string
}
