package itemoccurrence

import "time"

// ItemOccurrence 物品在章节中的出现记录。
type ItemOccurrence struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	NovelID     int64     `gorm:"column:novel_id;not null;index"    json:"novel_id"`
	ItemID      int64     `gorm:"column:item_id;not null;index"     json:"item_id"`      // FK → items.id
	ChapterID   int64     `gorm:"column:chapter_id;not null;index"  json:"chapter_id"`   // FK → chapters.id
	Action      string    `gorm:"column:action"                     json:"action"`        // acquired/used/lost/destroyed/mentioned
	Description string    `gorm:"column:description"                json:"description"`   // 该章节中的具体描述
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"  json:"created_at"`
}

func (ItemOccurrence) TableName() string { return "item_occurrences" }
