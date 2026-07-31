package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"novel/internal/item"
)

// ── get_items ──

type GetItemsArgs struct {
	Mode     string `json:"mode" jsonschema:"required,description=查询模式：list=列表 detail=详情,enum=list,enum=detail,default=list" validate:"required,oneof=list detail"`
	ItemID   int64  `json:"item_id" jsonschema:"description=物品ID（detail模式必填）" validate:"omitempty,min=1"`
	ItemType string `json:"item_type" jsonschema:"description=按类型筛选（list模式可选）"`
	Status   string `json:"status" jsonschema:"description=按状态筛选：active/consumed/destroyed/lost"`
	Search   string `json:"search" jsonschema:"description=按名称/描述搜索"`
	PageArgs
}

type GetItemsTool struct{}

func (t *GetItemsTool) Name() string { return "get_items" }
func (t *GetItemsTool) Description() string { return "获取当前小说的物品/法宝列表或详情。返回格式：{items: [{id, name, item_type, grade, description, lore, ability, arc_id, first_chapter_id, status_changed_chapter_id, narrative_role, owner_id, previous_owner_id, location_id, status, tags}], total, page, size}。brief=true 只返回 id/name/item_type/status/owner_id。" }
func (t *GetItemsTool) Category() ToolCategory { return CategoryNovelManagement }
func (t *GetItemsTool) JSONSchema() json.RawMessage { return SchemaOf(GetItemsArgs{}) }
func (t *GetItemsTool) ExposeToLLM() bool           { return true }
func (t *GetItemsTool) NewArgs() any                { return &GetItemsArgs{} }
func (t *GetItemsTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*GetItemsArgs); a.NormalizePage()
	store := item.NewStore(tc.DB, slog.Default())
	if a.Mode == "detail" {
		if a.ItemID <= 0 { return &ToolResult{Success: false, Error: "detail 模式需要 item_id"}, nil }
		it, err := store.GetByID(ctx, a.ItemID, tc.NovelID)
		if err != nil { return &ToolResult{Success: false, Error: fmt.Sprintf("物品不存在: %v", err)}, nil }
		return &ToolResult{Success: true, Data: map[string]any{
			"id": it.ID, "name": it.Name, "item_type": it.ItemType, "grade": it.Grade,
			"description": it.Description, "lore": it.Lore, "ability": it.Ability,
			"arc_id": it.ArcID, "first_chapter_id": it.FirstChapterID,
			"narrative_role": it.NarrativeRole,
			"owner_id": it.OwnerID, "previous_owner_id": it.PreviousOwnerID,
			"location_id": it.LocationID, "status": it.Status, "tags": it.Tags,
		}}, nil
	}
	result, err := store.ListByNovel(ctx, tc.NovelID, item.ListOptions{Page: a.Page, Size: a.Size, ItemType: a.ItemType, Status: a.Status, Search: a.Search})
	if err != nil { return nil, fmt.Errorf("list items: %w", err) }
	items := make([]map[string]any, len(result.Items))
	for i, it := range result.Items {
		items[i] = map[string]any{"id": it.ID, "name": it.Name, "item_type": it.ItemType, "grade": it.Grade, "status": it.Status, "tags": it.Tags}
	}
	data := PageMeta(result); data["items"] = items
	return &ToolResult{Success: true, Data: data}, nil
}

// ── create_item ──

type CreateItemArgs struct {
	Name                    string `json:"name" jsonschema:"required,description=物品名称" validate:"required"`
	ItemType                string `json:"item_type" jsonschema:"required,description=类型：法宝/丹药/灵药/功法/地图/信物/武器/防具/普通物品"`
	Grade                   string `json:"grade" jsonschema:"description=品级"`
	Description             string `json:"description" jsonschema:"required,description=外观/功能描述" validate:"required"`
	Lore                    string `json:"lore" jsonschema:"description=来历/历史/传说"`
	Ability                 string `json:"ability" jsonschema:"description=特殊能力"`
	ArcID                   int64  `json:"arc_id" jsonschema:"required,description=所属弧线ID"`
	FirstChapterID          int64  `json:"first_chapter_id" jsonschema:"description=首次出现章节ID"`
	StatusChangedChapterID  int64  `json:"status_changed_chapter_id" jsonschema:"description=状态变化章节ID"`
	NarrativeRole           string `json:"narrative_role" jsonschema:"required,description=叙事重要性：key_prop/supporting/minor/normal"`
	OwnerID                 int64  `json:"owner_id" jsonschema:"required,description=当前持有者character_id"`
	PreviousOwnerID         int64  `json:"previous_owner_id" jsonschema:"description=上一任持有者character_id"`
	LocationID              int64  `json:"location_id" jsonschema:"description=当前位置location_id"`
	Tags                    string `json:"tags" jsonschema:"description=JSON标签数组"`
}

type CreateItemTool struct{}

func (t *CreateItemTool) Name() string { return "create_item" }
func (t *CreateItemTool) Description() string { return "创建物品/法宝条目。填写 arc_id 和 first_chapter_id 以建立关联，填写 narrative_role 标记重要性。" +
		"item_type 可选值：法宝/丹药/灵药/功法/地图/信物/武器/防具/普通物品。" +
		"description 描述外观/功能，lore 描述来历/历史/传说。" +
		"narrative_role 四级：key_prop（核心道具，影响主线）/ supporting（辅助道具）/ minor（小道具）/ normal（普通物品）。" }
func (t *CreateItemTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *CreateItemTool) JSONSchema() json.RawMessage { return SchemaOf(CreateItemArgs{}) }
func (t *CreateItemTool) ExposeToLLM() bool { return true }
func (t *CreateItemTool) NewArgs() any      { return &CreateItemArgs{} }
func (t *CreateItemTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*CreateItemArgs)
	it := &item.Item{NovelID: tc.NovelID, Name: a.Name, ItemType: a.ItemType, Grade: a.Grade,
		Description: a.Description, Lore: a.Lore, Ability: a.Ability, Tags: a.Tags,
		NarrativeRole: a.NarrativeRole}
	if a.ArcID > 0 { it.ArcID = &a.ArcID }
	if a.FirstChapterID > 0 { it.FirstChapterID = &a.FirstChapterID }
	if a.OwnerID > 0 { it.OwnerID = &a.OwnerID }
	if a.PreviousOwnerID > 0 { it.PreviousOwnerID = &a.PreviousOwnerID }
	if a.LocationID > 0 { it.LocationID = &a.LocationID }
	if err := item.NewStore(tc.DB, slog.Default()).Create(ctx, it); err != nil {
		return nil, fmt.Errorf("create item: %w", err)
	}
	return &ToolResult{Success: true, Data: map[string]any{"id": it.ID}}, nil
}

// ── update_item ──

type UpdateItemArgs struct {
	ItemID                  int64  `json:"item_id" jsonschema:"required,description=物品ID" validate:"required,min=1"`
	Name                    string `json:"name"`
	ItemType                string `json:"item_type"`
	Grade                   string `json:"grade"`
	Description             string `json:"description"`
	Lore                    string `json:"lore"`
	Ability                 string `json:"ability"`
	ArcID                   int64  `json:"arc_id"`
	FirstChapterID          int64  `json:"first_chapter_id"`
	StatusChangedChapterID  int64  `json:"status_changed_chapter_id" jsonschema:"description=状态变化发生的章节ID"`
	NarrativeRole           string `json:"narrative_role"`
	OwnerID                 int64  `json:"owner_id"`
	PreviousOwnerID         int64  `json:"previous_owner_id"`
	LocationID              int64  `json:"location_id"`
	Status                  string `json:"status" jsonschema:"enum=active,enum=consumed,enum=destroyed,enum=lost"`
	Tags                    string `json:"tags"`
}

type UpdateItemTool struct{}

func (t *UpdateItemTool) Name() string { return "update_item" }
func (t *UpdateItemTool) Description() string { return "更新物品。PATCH 语义。如果更新了 owner_id（持有者变更），请同步用 create_item_occurrence 记录一条 action=acquired 给新持有者、action=lost 给原持有者。" }
func (t *UpdateItemTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *UpdateItemTool) JSONSchema() json.RawMessage { return SchemaOf(UpdateItemArgs{}) }
func (t *UpdateItemTool) ExposeToLLM() bool { return true }
func (t *UpdateItemTool) NewArgs() any      { return &UpdateItemArgs{} }
func (t *UpdateItemTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpdateItemArgs)
	store := item.NewStore(tc.DB, slog.Default())
	existing, err := store.GetByID(ctx, a.ItemID, tc.NovelID)
	if err != nil { return &ToolResult{Success: false, Error: fmt.Sprintf("物品不存在: %v", err)}, nil }
	if a.Name != "" { existing.Name = a.Name }
	if a.ItemType != "" { existing.ItemType = a.ItemType }
	if a.Grade != "" { existing.Grade = a.Grade }
	if a.Description != "" { existing.Description = a.Description }
	if a.Lore != "" { existing.Lore = a.Lore }
	if a.Ability != "" { existing.Ability = a.Ability }
	if a.ArcID > 0 { existing.ArcID = &a.ArcID }
	if a.FirstChapterID > 0 { existing.FirstChapterID = &a.FirstChapterID }
	if a.NarrativeRole != "" { existing.NarrativeRole = a.NarrativeRole }
	if a.Status != "" { existing.Status = a.Status }
	if a.Tags != "" { existing.Tags = a.Tags }
	if a.OwnerID > 0 { existing.OwnerID = &a.OwnerID }
	if a.PreviousOwnerID > 0 { existing.PreviousOwnerID = &a.PreviousOwnerID }
	if a.LocationID > 0 { existing.LocationID = &a.LocationID }
	if a.StatusChangedChapterID > 0 { existing.StatusChangedChapterID = &a.StatusChangedChapterID }
	if err := store.Update(ctx, existing); err != nil { return nil, fmt.Errorf("update item: %w", err) }
	return &ToolResult{Success: true, Data: map[string]any{"item_id": existing.ID}}, nil
}

// ── delete_item / search_items ──

type DeleteItemArgs struct {
	ItemID int64 `json:"item_id" jsonschema:"required,description=物品ID" validate:"required,min=1"`
}
type DeleteItemTool struct{}
func (t *DeleteItemTool) Name() string { return "delete_item" }
func (t *DeleteItemTool) Description() string { return "删除物品条目。" }
func (t *DeleteItemTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *DeleteItemTool) JSONSchema() json.RawMessage { return SchemaOf(DeleteItemArgs{}) }
func (t *DeleteItemTool) ExposeToLLM() bool { return true }
func (t *DeleteItemTool) NewArgs() any      { return &DeleteItemArgs{} }
func (t *DeleteItemTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*DeleteItemArgs)
	if err := item.NewStore(tc.DB, slog.Default()).Delete(ctx, a.ItemID, tc.NovelID); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("删除失败: %v", err)}, nil
	}
	return &ToolResult{Success: true, Data: map[string]any{"deleted": true}}, nil
}

type SearchItemsArgs struct {
	Query string `json:"query" jsonschema:"required,description=搜索关键词" validate:"required"`
}
type SearchItemsTool struct{}
func (t *SearchItemsTool) Name() string { return "search_items" }
func (t *SearchItemsTool) Description() string { return "按名称/能力/描述/来历搜索物品。返回格式：{items: [{id, name, item_type, grade, description, ability, owner_id, location_id, status}]}。" }
func (t *SearchItemsTool) Category() ToolCategory { return CategoryMemoryRetrieval }
func (t *SearchItemsTool) JSONSchema() json.RawMessage { return SchemaOf(SearchItemsArgs{}) }
func (t *SearchItemsTool) ExposeToLLM() bool { return true }
func (t *SearchItemsTool) NewArgs() any      { return &SearchItemsArgs{} }
func (t *SearchItemsTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*SearchItemsArgs)
	items, err := item.NewStore(tc.DB, slog.Default()).Search(ctx, tc.NovelID, a.Query, 10)
	if err != nil { return nil, fmt.Errorf("search items: %w", err) }
	return &ToolResult{Success: true, Data: map[string]any{"items": items, "count": len(items)}}, nil
}

func RegisterItemTools(r *Registry) {
	r.Register(&GetItemsTool{}); r.Register(&CreateItemTool{}); r.Register(&UpdateItemTool{})
	r.Register(&DeleteItemTool{}); r.Register(&SearchItemsTool{})
}
