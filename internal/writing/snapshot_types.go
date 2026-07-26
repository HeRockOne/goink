package writing

import "time"

// WritingSnapshot 当前写作进度快照，每本书仅一条。
type WritingSnapshot struct {
	NovelID         int64     `gorm:"column:novel_id;primaryKey"    json:"novel_id"`
	LastChapterID   int64     `gorm:"column:last_chapter_id"        json:"last_chapter_id"`
	LastChapterNum  int       `gorm:"column:last_chapter_num"       json:"last_chapter_num"`
	CurrentArcID    *int64    `gorm:"column:current_arc_id"         json:"current_arc_id"`
	CurrentLocation string    `gorm:"column:current_location"       json:"current_location"`
	ActiveChars     string    `gorm:"column:active_chars"           json:"active_chars"`
	PendingThreads  string    `gorm:"column:pending_threads"        json:"pending_threads"`
	Summary         string    `gorm:"column:summary"                json:"summary"`
	DetailedState   string    `gorm:"column:detailed_state"         json:"detailed_state"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (WritingSnapshot) TableName() string { return "writing_snapshots" }
