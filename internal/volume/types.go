package volume

import "time"

// Volume 是全书分卷（物理章节分卷），独立于叙事弧线。
type Volume struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	NovelID      int64     `gorm:"column:novel_id;not null;index;constraint:OnDelete:CASCADE" json:"novel_id"`
	Name         string    `gorm:"column:name;not null"                json:"name"`
	Description  string    `gorm:"column:description;type:text"        json:"description"`
	StartChapter int       `gorm:"column:start_chapter;default:0"      json:"start_chapter"`
	EndChapter   int       `gorm:"column:end_chapter;default:0"        json:"end_chapter"`
	DetailJSON   string    `gorm:"column:detail_json;type:text"        json:"detail_json"` // 核心事件/主角变化/收尾钩子/爽点分布
	SortOrder    int       `gorm:"column:sort_order;default:0"         json:"sort_order"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"    json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"    json:"updated_at"`
}

func (Volume) TableName() string { return "volumes" }
