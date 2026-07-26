package scene

import (
	"context"
	"log/slog"

	"gorm.io/gorm"
)

type Store struct {
	DB     *gorm.DB
	logger *slog.Logger
}

func NewStore(db *gorm.DB, logger *slog.Logger) *Store {
	return &Store{DB: db, logger: logger}
}

func (s *Store) ListByChapter(ctx context.Context, novelID, chapterID int64) ([]Scene, error) {
	var items []Scene
	if err := s.DB.WithContext(ctx).Where("novel_id = ? AND chapter_id = ?", novelID, chapterID).
		Order("scene_number ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) GetByID(ctx context.Context, id, novelID int64) (*Scene, error) {
	var sc Scene
	if err := s.DB.WithContext(ctx).Where("id = ? AND novel_id = ?", id, novelID).First(&sc).Error; err != nil {
		return nil, err
	}
	return &sc, nil
}

func (s *Store) Create(ctx context.Context, sc *Scene) error {
	return s.DB.WithContext(ctx).Create(sc).Error
}

func (s *Store) Update(ctx context.Context, sc *Scene) error {
	return s.DB.WithContext(ctx).Model(sc).Updates(map[string]any{
		"scene_number": sc.SceneNumber, "title": sc.Title,
		"location_id": sc.LocationID, "character_ids": sc.CharacterIDs,
		"arc_id": sc.ArcID, "arc_node_id": sc.ArcNodeID,
		"word_count": sc.WordCount, "summary": sc.Summary,
	}).Error
}

func (s *Store) Delete(ctx context.Context, id, novelID int64) error {
	return s.DB.WithContext(ctx).Where("id = ? AND novel_id = ?", id, novelID).Delete(&Scene{}).Error
}
