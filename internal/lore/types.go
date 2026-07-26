package lore

import "time"

// LoreEntry 世界观设定条目。
type LoreEntry struct {
	ID              int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	NovelID         int64     `gorm:"column:novel_id;not null;index"    json:"novel_id"`
	Title           string    `gorm:"column:title;not null;index"       json:"title"`
	Category        string    `gorm:"column:category;not null;index"    json:"category"`
	Content         string    `gorm:"column:content;not null"           json:"content"`
	Summary         string    `gorm:"column:summary"                    json:"summary"`
	ArcID           *int64    `gorm:"column:arc_id;index"               json:"arc_id"`           // 所属弧线
	RevealChapterID *int64    `gorm:"column:reveal_chapter_id;index"    json:"reveal_chapter_id"` // 读者首次得知此设定的章节
	IsPublic        bool      `gorm:"column:is_public;default:true"     json:"is_public"`         // 公开设定/秘密
	ReferenceID     *int64    `gorm:"column:reference_id;index"         json:"reference_id"`
	ReferenceType   string    `gorm:"column:reference_type"             json:"reference_type"`
	Tags            string    `gorm:"column:tags"                       json:"tags"`
	Source          string    `gorm:"column:source;default:'ai'"        json:"source"` // ai/user/import
	Version         int       `gorm:"column:version;default:1"          json:"version"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime"  json:"created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime"  json:"updated_at"`
}

func (LoreEntry) TableName() string { return "lore_entries" }
