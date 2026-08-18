package outline

import "time"

// Outline 是全书总纲（存储在数据库，替代 book-outline.md）。
type Outline struct {
	ID              int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	NovelID         int64     `gorm:"column:novel_id;not null;uniqueIndex;constraint:OnDelete:CASCADE" json:"novel_id"`
	CoreConflict    string    `gorm:"column:core_conflict;type:text" json:"core_conflict"`
	GrowthArc       string    `gorm:"column:growth_arc;type:text" json:"growth_arc"`
	EndingDirection string    `gorm:"column:ending_direction;type:text" json:"ending_direction"`
	WordCountPlan   int       `gorm:"column:word_count_plan;default:0" json:"word_count_plan"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (Outline) TableName() string { return "outlines" }

// OutlineBeat 是大爽点/关键节点（存储在数据库，替代 book-outline.md 中的大爽点列表）。
type OutlineBeat struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	NovelID     int64     `gorm:"column:novel_id;not null;index;constraint:OnDelete:CASCADE" json:"novel_id"`
	Chapter     int       `gorm:"column:chapter;not null" json:"chapter"`
	Description string    `gorm:"column:description;type:text;not null" json:"description"`
	BeatType    string    `gorm:"column:beat_type;not null;default:shuangdian" json:"beat_type"`
	Importance  int       `gorm:"column:importance;default:3" json:"importance"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (OutlineBeat) TableName() string { return "outline_beats" }
