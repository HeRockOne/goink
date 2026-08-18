package outline

import (
	"context"

	"gorm.io/gorm"
)

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// GetByNovelID 获取指定小说的总纲（不存在则返回 nil）。
func (s *Store) GetByNovelID(ctx context.Context, novelID int64) (*Outline, error) {
	var o Outline
	err := s.db.WithContext(ctx).Where("novel_id = ?", novelID).First(&o).Error
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// Upsert 创建或更新总纲（novel_id 唯一）。
func (s *Store) Upsert(ctx context.Context, o *Outline) error {
	var existing Outline
	err := s.db.WithContext(ctx).Where("novel_id = ?", o.NovelID).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return s.db.WithContext(ctx).Create(o).Error
	}
	if err != nil {
		return err
	}
	o.ID = existing.ID
	return s.db.WithContext(ctx).Save(o).Error
}

// ListBeats 获取指定小说的所有大爽点（按章号排序）。
func (s *Store) ListBeats(ctx context.Context, novelID int64) ([]OutlineBeat, error) {
	var beats []OutlineBeat
	err := s.db.WithContext(ctx).Where("novel_id = ?", novelID).Order("chapter").Find(&beats).Error
	return beats, err
}

// CreateBeat 创建大爽点。
func (s *Store) CreateBeat(ctx context.Context, b *OutlineBeat) error {
	return s.db.WithContext(ctx).Create(b).Error
}

// UpdateBeat 更新大爽点。
func (s *Store) UpdateBeat(ctx context.Context, b *OutlineBeat) error {
	return s.db.WithContext(ctx).Save(b).Error
}

// DeleteBeat 删除大爽点。
func (s *Store) DeleteBeat(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Delete(&OutlineBeat{}, id).Error
}

// DeleteBeatsByNovelID 删除指定小说的所有大爽点。
func (s *Store) DeleteBeatsByNovelID(ctx context.Context, novelID int64) error {
	return s.db.WithContext(ctx).Where("novel_id = ?", novelID).Delete(&OutlineBeat{}).Error
}
