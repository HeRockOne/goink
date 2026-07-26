package stats

import (
	"context"
	"log/slog"

	"gorm.io/gorm"

	"novel/internal/chapter"
	"novel/internal/character"
	"novel/internal/location"
	"novel/internal/storyarc"
	"novel/internal/timeline"
)

type Store struct {
	DB     *gorm.DB
	logger *slog.Logger
}

func NewStore(db *gorm.DB, logger *slog.Logger) *Store {
	return &Store{DB: db, logger: logger}
}

type NovelStats struct {
	TotalChapters      int    `json:"total_chapters"`
	TotalWords         int    `json:"total_words"`
	AvgChapterWords    int    `json:"avg_chapter_words"`
	LatestChapterNum   int    `json:"latest_chapter_num"`
	LatestChapterTitle string `json:"latest_chapter_title"`

	ArcCount     int `json:"arc_count"`
	ArcCompleted int `json:"arc_completed"`
	ArcActive    int `json:"arc_active"`

	ForeshadowingTotal    int `json:"foreshadowing_total"`
	ForeshadowingResolved int `json:"foreshadowing_resolved"`
	ForeshadowingPending  int `json:"foreshadowing_pending"`

	CharacterCount int64 `json:"character_count"`
	LocationCount  int64 `json:"location_count"`

	TotalWordsAllTime int `json:"total_words_all_time"`
}

// GetNovelStats 聚合指定小说的统计信息。
func (s *Store) GetNovelStats(ctx context.Context, novelID int64) (*NovelStats, error) {
	stats := &NovelStats{}

	// 章节统计
	var chs []chapter.Chapter
	if err := s.DB.WithContext(ctx).Where("novel_id = ?", novelID).Order("chapter_number DESC").Find(&chs).Error; err == nil {
		stats.TotalChapters = len(chs)
		for _, c := range chs {
			stats.TotalWords += c.WordCount
		}
		if stats.TotalChapters > 0 {
			stats.AvgChapterWords = stats.TotalWords / stats.TotalChapters
			stats.LatestChapterNum = chs[0].ChapterNumber
			stats.LatestChapterTitle = chs[0].Title
		}
	}

	// 弧线
	var arcs []storyarc.StoryArc
	if err := s.DB.WithContext(ctx).Where("novel_id = ?", novelID).Find(&arcs).Error; err == nil {
		stats.ArcCount = len(arcs)
		for _, a := range arcs {
			switch a.Status {
			case "completed":
				stats.ArcCompleted++
			case "active":
				stats.ArcActive++
			}
		}
	}

	// 伏笔
	var allTL []timeline.TimelineEntry
	if err := s.DB.WithContext(ctx).Where("novel_id = ?", novelID).Find(&allTL).Error; err == nil {
		for _, t := range allTL {
			switch t.Status {
			case "resolved":
				stats.ForeshadowingResolved++
			case "pending":
				stats.ForeshadowingPending++
			}
			stats.ForeshadowingTotal++
		}
	}

	// 角色
	s.DB.WithContext(ctx).Model(&character.Character{}).Where("novel_id = ?", novelID).Count(&stats.CharacterCount)
	s.DB.WithContext(ctx).Where("novel_id = ?", novelID).Model(&location.Location{}).Count(&stats.LocationCount)

	return stats, nil
}
