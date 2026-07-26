package item

import "time"

// Item 物品/法宝条目。
type Item struct {
	ID                      int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	NovelID                 int64     `gorm:"column:novel_id;not null;index"    json:"novel_id"`
	Name                    string    `gorm:"column:name;not null;index"        json:"name"`
	ItemType                string    `gorm:"column:item_type;index"            json:"item_type"`
	Grade                   string    `gorm:"column:grade"                      json:"grade"`
	Description             string    `gorm:"column:description"                json:"description"`
	Lore                    string    `gorm:"column:lore"                       json:"lore"`
	Ability                 string    `gorm:"column:ability"                    json:"ability"`
	ArcID                   *int64    `gorm:"column:arc_id;index"               json:"arc_id"`                       // 所属弧线
	FirstChapterID          *int64    `gorm:"column:first_chapter_id;index"     json:"first_chapter_id"`             // 首次出现章节
	StatusChangedChapterID  *int64    `gorm:"column:status_changed_chapter_id"  json:"status_changed_chapter_id"`    // 状态变化章节
	NarrativeRole           string    `gorm:"column:narrative_role;default:'normal'" json:"narrative_role"`          // key_prop/supporting/minor
	OwnerID                 *int64    `gorm:"column:owner_id;index"             json:"owner_id"`
	PreviousOwnerID         *int64    `gorm:"column:previous_owner_id;index"    json:"previous_owner_id"`            // 上一任持有者
	LocationID              *int64    `gorm:"column:location_id;index"          json:"location_id"`
	Status                  string    `gorm:"column:status;default:'active'"    json:"status"`
	Tags                    string    `gorm:"column:tags"                       json:"tags"`
	Source                  string    `gorm:"column:source;default:'ai'"        json:"source"` // ai/user/import
	CreatedAt               time.Time `gorm:"column:created_at;autoCreateTime"  json:"created_at"`
	UpdatedAt               time.Time `gorm:"column:updated_at;autoUpdateTime"  json:"updated_at"`
}

func (Item) TableName() string { return "items" }
