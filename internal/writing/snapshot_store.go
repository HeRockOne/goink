package writing

import (
	"context"
	"log/slog"

	"gorm.io/gorm"
)

type SnapshotStore struct {
	DB     *gorm.DB
	logger *slog.Logger
}

func NewSnapshotStore(db *gorm.DB, logger *slog.Logger) *SnapshotStore {
	return &SnapshotStore{DB: db, logger: logger}
}

func (s *SnapshotStore) Get(ctx context.Context, novelID int64) (*WritingSnapshot, error) {
	var snap WritingSnapshot
	if err := s.DB.WithContext(ctx).First(&snap, novelID).Error; err != nil {
		return nil, err
	}
	return &snap, nil
}

// Upsert 插入或更新快照（每本书仅一条）。
func (s *SnapshotStore) Upsert(ctx context.Context, snap *WritingSnapshot) error {
	return s.DB.WithContext(ctx).Save(snap).Error
}
