package chapter

import "time"

// Chapter 是章节元数据，正文和大纲以文件形式存储在 Git 仓库中。
type Chapter struct {
	ID            int64     `gorm:"column:id;primaryKey;autoIncrement"                                    json:"id"`
	NovelID       int64     `gorm:"column:novel_id;not null;uniqueIndex:uk_novel_chapter;index"           json:"novel_id"`
	ChapterNumber int       `gorm:"column:chapter_number;not null;uniqueIndex:uk_novel_chapter"           json:"chapter_number"`
	Title         string    `gorm:"column:title"                                                          json:"title"`
	Summary       string    `gorm:"column:summary"                                                        json:"summary"`
	KeyEvents     string    `gorm:"column:key_events"                                                     json:"key_events"`     // JSON 数组，关键事件列表
	CharactersIn  string    `gorm:"column:characters_in"                                                  json:"characters_in"`  // JSON 数组，出场角色 ID
	ArcIDs        string    `gorm:"column:arc_ids"                                                        json:"arc_ids"`        // JSON 数组，本章涉及的弧线 ID
	WordCount     int       `gorm:"column:word_count;default:0"                                           json:"word_count"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime"                                      json:"created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime"                                      json:"updated_at"`
	FilePath      string    `gorm:"-"                                                                     json:"file_path"`
}

func (Chapter) TableName() string { return "chapters" }

// ChapterArc 章节与弧线的多对多关联表。
type ChapterArc struct {
	ID          int64 `gorm:"column:id;primaryKey;autoIncrement"                    json:"id"`
	NovelID     int64 `gorm:"column:novel_id;not null;index"                       json:"novel_id"`
	ChapterID   int64 `gorm:"column:chapter_id;not null;uniqueIndex:uk_chapter_arc" json:"chapter_id"`
	StoryArcID  int64 `gorm:"column:story_arc_id;not null;uniqueIndex:uk_chapter_arc" json:"story_arc_id"`
}

func (ChapterArc) TableName() string { return "chapter_arcs" }
