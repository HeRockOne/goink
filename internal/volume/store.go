package volume

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// Store 管理 Volume 持久化。
type Store struct {
	DB *gorm.DB
}

// NewStore 创建 volume 存储。
func NewStore(db *gorm.DB) *Store {
	return &Store{DB: db}
}

// ListByNovel 返回某小说的所有卷（按排序）。
func (s *Store) ListByNovel(ctx context.Context, novelID int64) ([]Volume, error) {
	var volumes []Volume
	if err := s.DB.WithContext(ctx).
		Where("novel_id = ?", novelID).
		Order("sort_order ASC, start_chapter ASC").
		Find(&volumes).Error; err != nil {
		return nil, fmt.Errorf("volume store: list: %w", err)
	}
	return volumes, nil
}

// GetByID 返回指定卷。
func (s *Store) GetByID(ctx context.Context, id int64) (*Volume, error) {
	var v Volume
	if err := s.DB.WithContext(ctx).First(&v, id).Error; err != nil {
		return nil, err
	}
	return &v, nil
}

// Create 创建一条卷。
func (s *Store) Create(ctx context.Context, v *Volume) error {
	return s.DB.WithContext(ctx).Create(v).Error
}

// Update 更新一条卷。
func (s *Store) Update(ctx context.Context, v *Volume) error {
	return s.DB.WithContext(ctx).Save(v).Error
}

// Delete 删除一条卷。
func (s *Store) Delete(ctx context.Context, id int64) error {
	return s.DB.WithContext(ctx).Delete(&Volume{}, id).Error
}

// DeleteByNovel 删除某小说的所有卷（批量清理用）。
func (s *Store) DeleteByNovel(ctx context.Context, novelID int64) error {
	return s.DB.WithContext(ctx).Where("novel_id = ?", novelID).Delete(&Volume{}).Error
}
