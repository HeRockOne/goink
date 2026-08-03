package scene

import "time"

// Scene 章节内的场景条目。
type Scene struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	NovelID      int64     `gorm:"column:novel_id;not null;index"    json:"novel_id"`
	ChapterID    *int64    `gorm:"column:chapter_id;index"  json:"chapter_id"`
	SceneNumber  int       `gorm:"column:scene_number;not null"      json:"scene_number"`
	Title        string    `gorm:"column:title"                      json:"title"`
	LocationID   *int64    `gorm:"column:location_id;index"          json:"location_id"`
	CharacterIDs string    `gorm:"column:character_ids"              json:"character_ids"`
	ArcID        *int64    `gorm:"column:arc_id;index"               json:"arc_id"`         // 所属弧线
	ArcNodeID    *int64    `gorm:"column:arc_node_id;index"          json:"arc_node_id"`     // 对应弧线节点
	WordCount    int       `gorm:"column:word_count;default:0"       json:"word_count"`
	Summary      string    `gorm:"column:summary"                    json:"summary"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"  json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"  json:"updated_at"`
}

func (Scene) TableName() string { return "scenes" }
