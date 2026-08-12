package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"novel/internal/item"
	"novel/internal/itemoccurrence"
)

// ── get_items ──

type GetItemsArgs struct {
	Mode     string `json:"mode" jsonschema:"required,description=查询模式：list=列表 detail=详情,enum=list,enum=detail,default=list" validate:"required,oneof=list detail"`
	ItemID   int64  `json:"item_id" jsonschema:"description=物品ID（detail模式必填）" validate:"omitempty,min=1"`
	ItemType string `json:"item_type" jsonschema:"description=按类型筛选（list模式可选）"`
	Status   string `json:"status" jsonschema:"description=按状态筛选：active/consumed/destroyed/lost"`
	Search   string `json:"search" jsonschema:"description=按名称/描述搜索"`
	Brief    bool   `json:"brief" jsonschema:"description=true=只返回 id/name/item_type/status/owner_id（省token）；false=返回含 grade/tags 的列表字段"`
	PageArgs
}

type GetItemsTool struct{}

func (t *GetItemsTool) Name() string { return "get_items" }
func (t *GetItemsTool) Description() string {
	return "获取当前小说的物品/法宝列表或详情。list：按条件浏览物品（默认 50 条/页，返回 id/name/item_type/grade/status/tags；brief=true 只返回 id/name/item_type/status/owner_id）；detail：按 item_id 获取单件完整信息（description/lore/ability/流转史）。" +
		"【使用时机】写作前查角色持有物品/物品现状用 detail 或 brief list；不确定名称时用 search 关键词检索。" +
		"【省token】list 默认 50 条/页，用 size 缩小（如 size=10），不要翻页拉全量；要单件详情用 detail 不要 list 全量自己筛。"
}
func (t *GetItemsTool) Category() ToolCategory      { return CategoryNovelManagement }
func (t *GetItemsTool) JSONSchema() json.RawMessage { return SchemaOf(GetItemsArgs{}) }
func (t *GetItemsTool) ExposeToLLM() bool           { return true }
func (t *GetItemsTool) NewArgs() any                { return &GetItemsArgs{} }
func (t *GetItemsTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*GetItemsArgs)
	a.NormalizePage()
	store := item.NewStore(tc.DB, slog.Default())
	if a.Mode == "detail" {
		if a.ItemID <= 0 {
			return &ToolResult{Success: false, Error: "detail 模式需要 item_id"}, nil
		}
		it, err := store.GetByID(ctx, a.ItemID, tc.NovelID)
		if err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("物品不存在: %v", err)}, nil
		}
		return &ToolResult{Success: true, Data: map[string]any{
			"id": it.ID, "name": it.Name, "item_type": it.ItemType, "grade": it.Grade,
			"description": it.Description, "lore": it.Lore, "ability": it.Ability,
			"arc_id": it.ArcID, "first_chapter_id": it.FirstChapterID,
			"narrative_role": it.NarrativeRole,
			"owner_id":       it.OwnerID, "previous_owner_id": it.PreviousOwnerID,
			"location_id": it.LocationID, "status": it.Status, "tags": it.Tags,
		}}, nil
	}
	result, err := store.ListByNovel(ctx, tc.NovelID, item.ListOptions{Page: a.Page, Size: a.Size, ItemType: a.ItemType, Status: a.Status, Search: a.Search})
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	items := make([]map[string]any, len(result.Items))
	for i, it := range result.Items {
		if a.Brief {
			items[i] = map[string]any{"id": it.ID, "name": it.Name, "item_type": it.ItemType, "status": it.Status, "owner_id": it.OwnerID}
		} else {
			items[i] = map[string]any{"id": it.ID, "name": it.Name, "item_type": it.ItemType, "grade": it.Grade, "status": it.Status, "tags": it.Tags}
		}
	}
	data := PageMeta(result)
	data["items"] = items
	return &ToolResult{Success: true, Data: data}, nil
}

// ── create_item ──

type CreateItemArgs struct {
	Name                   string `json:"name" jsonschema:"required,description=物品名称" validate:"required"`
	ItemType               string `json:"item_type" jsonschema:"required,description=类型：法宝/丹药/灵药/功法/地图/信物/武器/防具/普通物品"`
	Grade                  string `json:"grade" jsonschema:"description=品级"`
	Description            string `json:"description" jsonschema:"required,description=外观/功能描述" validate:"required"`
	Lore                   string `json:"lore" jsonschema:"description=来历/历史/传说"`
	Ability                string `json:"ability" jsonschema:"description=特殊能力"`
	ArcID                  int64  `json:"arc_id" jsonschema:"description=所属弧线ID（开书阶段可先不关联，>0 才写入）"`
	FirstChapterID         int64  `json:"first_chapter_id" jsonschema:"description=首次出现章节ID"`
	StatusChangedChapterID int64  `json:"status_changed_chapter_id" jsonschema:"description=状态变化章节ID"`
	NarrativeRole          string `json:"narrative_role" jsonschema:"description=叙事重要性：key_prop/supporting/minor/normal（默认 normal）"`
	OwnerID                int64  `json:"owner_id" jsonschema:"description=当前持有者character_id（无主物品可不传，>0 才写入）"`
	PreviousOwnerID        int64  `json:"previous_owner_id" jsonschema:"description=上一任持有者character_id"`
	LocationID             int64  `json:"location_id" jsonschema:"description=当前位置location_id"`
	Tags                   string `json:"tags" jsonschema:"description=JSON标签数组，纯字符串数组，如[\"法宝\"，\"护主\"],禁止对象数组"`
}

type CreateItemTool struct{}

func (t *CreateItemTool) Name() string { return "create_item" }
func (t *CreateItemTool) Description() string {
	return "创建物品/法宝条目。填写 arc_id 和 first_chapter_id 以建立关联，填写 narrative_role 标记重要性。" +
		"item_type 可选值：法宝/丹药/灵药/功法/地图/信物/武器/防具/普通物品。" +
		"description 描述外观/功能，lore 描述来历/历史/传说。" +
		"narrative_role 四级：key_prop（核心道具，影响主线）/ supporting（辅助道具）/ minor（小道具）/ normal（普通物品）。" +
		"【使用时机】关键物品设计时（金手指/信物/功法）建档案；已有物品不要重复创建（先 get_items/search_items 确认）。" +
		"【注意】narrative_role 决定了物品在一致性检查中的关注度——key_prop 的流转史会被重点核对，创建时务必准确标注。"
}
func (t *CreateItemTool) Category() ToolCategory      { return CategoryWritingAssistant }
func (t *CreateItemTool) JSONSchema() json.RawMessage { return SchemaOf(CreateItemArgs{}) }
func (t *CreateItemTool) ExposeToLLM() bool           { return true }
func (t *CreateItemTool) NewArgs() any                { return &CreateItemArgs{} }
func (t *CreateItemTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*CreateItemArgs)
	tags, err := NormalizeStringArray(a.Tags)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("tags 格式错误: %v", err)}, nil
	}
	it := &item.Item{NovelID: tc.NovelID, Name: a.Name, ItemType: a.ItemType, Grade: a.Grade,
		Description: a.Description, Lore: a.Lore, Ability: a.Ability, Tags: tags,
		NarrativeRole: a.NarrativeRole}
	if a.ArcID > 0 {
		it.ArcID = &a.ArcID
	}
	if a.FirstChapterID > 0 {
		it.FirstChapterID = &a.FirstChapterID
	}
	if a.OwnerID > 0 {
		it.OwnerID = &a.OwnerID
	}
	if a.PreviousOwnerID > 0 {
		it.PreviousOwnerID = &a.PreviousOwnerID
	}
	if a.LocationID > 0 {
		it.LocationID = &a.LocationID
	}
	if err := item.NewStore(tc.DB, slog.Default()).Create(ctx, it); err != nil {
		return nil, fmt.Errorf("create item: %w", err)
	}
	return &ToolResult{Success: true, Data: map[string]any{"id": it.ID}}, nil
}

// ── update_item ──

type UpdateItemArgs struct {
	ItemID                 int64  `json:"item_id" jsonschema:"required,description=物品ID" validate:"required,min=1"`
	Name                   string `json:"name" jsonschema:"description=新的名称"`
	ItemType               string `json:"item_type" jsonschema:"description=新的类型：法宝/丹药/灵药/功法/地图/信物/武器/防具/普通物品"`
	Grade                  string `json:"grade" jsonschema:"description=新的品级"`
	Description            string `json:"description" jsonschema:"description=新的外观/功能描述（完全替换）"`
	Lore                   string `json:"lore" jsonschema:"description=新的来历/历史/传说（完全替换）"`
	Ability                string `json:"ability" jsonschema:"description=新的特殊能力（完全替换）"`
	ArcID                  int64  `json:"arc_id" jsonschema:"description=所属弧线ID（>0 才更新）"`
	FirstChapterID         int64  `json:"first_chapter_id" jsonschema:"description=首次出现章节ID（>0 才更新）"`
	StatusChangedChapterID int64  `json:"status_changed_chapter_id" jsonschema:"description=状态变化发生的章节ID"`
	NarrativeRole          string `json:"narrative_role" jsonschema:"description=叙事重要性：key_prop/supporting/minor/normal"`
	OwnerID                int64  `json:"owner_id" jsonschema:"description=新的当前持有者character_id。持有者变更时系统自动写入 item_occurrence 流转记录，无需手动调用 create_item_occurrence"`
	PreviousOwnerID        int64  `json:"previous_owner_id" jsonschema:"description=上一任持有者character_id（>0 才更新）"`
	LocationID             int64  `json:"location_id" jsonschema:"description=新的当前位置location_id（>0 才更新）"`
	Status                 string `json:"status" jsonschema:"enum=active,enum=consumed,enum=destroyed,enum=lost。注意：destroyed（已销毁）/consumed（已消耗）是终态，不可改回"`
	Tags                   string `json:"tags" jsonschema:"description=新的JSON标签数组（完全替换），纯字符串数组"`
	ChapterID              int64  `json:"chapter_id" jsonschema:"description=当前章节ID（maintain 阶段处理本章物品时必填）。持有者变更/状态变化时用于记录流转，必须是有效章节"`
}

type UpdateItemTool struct{}

func (t *UpdateItemTool) Name() string { return "update_item" }
func (t *UpdateItemTool) Description() string {
	return "更新物品。PATCH 语义。持有者变更（owner_id 变化）时系统自动写入 item_occurrence 流转记录（acquired/lost），必须提供 chapter_id（当前章节ID）。destroyed/consumed 是终态，不允许改回 active。" +
		"【使用时机】物品易主/消耗/销毁/丢失时（每章维护阶段同步物品状态）；持有者变更不要手动调 create_item_occurrence（自动记录）。"
}
func (t *UpdateItemTool) Category() ToolCategory      { return CategoryWritingAssistant }
func (t *UpdateItemTool) JSONSchema() json.RawMessage { return SchemaOf(UpdateItemArgs{}) }
func (t *UpdateItemTool) ExposeToLLM() bool           { return true }
func (t *UpdateItemTool) NewArgs() any                { return &UpdateItemArgs{} }
func (t *UpdateItemTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpdateItemArgs)

	// 空更新守卫：至少提供一个要修改的字段（PATCH 语义）
	if a.Name == "" && a.ItemType == "" && a.Grade == "" && a.Description == "" && a.Lore == "" &&
		a.Ability == "" && a.ArcID <= 0 && a.FirstChapterID <= 0 && a.NarrativeRole == "" &&
		a.OwnerID <= 0 && a.PreviousOwnerID <= 0 && a.LocationID <= 0 && a.Status == "" &&
		a.Tags == "" && a.StatusChangedChapterID <= 0 {
		return &ToolResult{Success: false, Error: "至少需要提供一个要修改的字段"}, nil
	}

	store := item.NewStore(tc.DB, slog.Default())
	existing, err := store.GetByID(ctx, a.ItemID, tc.NovelID)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("物品不存在: %v", err)}, nil
	}

	// 终态守卫：destroyed/consumed 不可逆，避免已销毁神器在后期章节被误用
	if (existing.Status == "destroyed" || existing.Status == "consumed") && a.Status != "" &&
		a.Status != "destroyed" && a.Status != "consumed" {
		return &ToolResult{Success: false,
			Error: fmt.Sprintf("物品 [%s] 状态为 %s（终态），不允许改回 %s。如需恢复请人工在物品面板确认。", existing.Name, existing.Status, a.Status)}, nil
	}

	// 持有者变更检测：自动写流转记录（acquired/lost），要求提供当前章节ID
	ownerChanged := false
	oldOwnerID := int64(0)
	if a.OwnerID > 0 && existing.OwnerID != nil && *existing.OwnerID != a.OwnerID {
		if a.ChapterID <= 0 {
			return &ToolResult{Success: false,
				Error: "持有者变更时必须提供 chapter_id（当前章节ID），用于自动写入物品流转记录"}, nil
		}
		ownerChanged = true
		oldOwnerID = *existing.OwnerID
	}

	if a.Name != "" {
		existing.Name = a.Name
	}
	if a.ItemType != "" {
		existing.ItemType = a.ItemType
	}
	if a.Grade != "" {
		existing.Grade = a.Grade
	}
	if a.Description != "" {
		existing.Description = a.Description
	}
	if a.Lore != "" {
		existing.Lore = a.Lore
	}
	if a.Ability != "" {
		existing.Ability = a.Ability
	}
	if a.ArcID > 0 {
		existing.ArcID = &a.ArcID
	}
	if a.FirstChapterID > 0 {
		existing.FirstChapterID = &a.FirstChapterID
	}
	if a.NarrativeRole != "" {
		existing.NarrativeRole = a.NarrativeRole
	}
	if a.Status != "" {
		existing.Status = a.Status
	}
	if a.Tags != "" {
		tags, err := NormalizeStringArray(a.Tags)
		if err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("tags 格式错误: %v", err)}, nil
		}
		existing.Tags = tags
	}
	if a.OwnerID > 0 {
		existing.OwnerID = &a.OwnerID
	}
	if a.PreviousOwnerID > 0 {
		existing.PreviousOwnerID = &a.PreviousOwnerID
	}
	if a.LocationID > 0 {
		existing.LocationID = &a.LocationID
	}
	if a.StatusChangedChapterID > 0 {
		existing.StatusChangedChapterID = &a.StatusChangedChapterID
	}
	if err := store.Update(ctx, existing); err != nil {
		return nil, fmt.Errorf("update item: %w", err)
	}

	// 持有者变更：服务层自动写流转记录，保证物品流转链完整（不依赖 AI 手动调用）
	if ownerChanged {
		occStore := itemoccurrence.NewStore(tc.DB, slog.Default())
		desc := fmt.Sprintf("物品从持有者 %d 转至 %d", oldOwnerID, a.OwnerID)
		if err := occStore.Create(ctx, &itemoccurrence.ItemOccurrence{
			NovelID: tc.NovelID, ItemID: existing.ID, ChapterID: a.ChapterID,
			Action: "lost", Description: desc,
		}); err != nil {
			return nil, fmt.Errorf("record ownership lost: %w", err)
		}
		if err := occStore.Create(ctx, &itemoccurrence.ItemOccurrence{
			NovelID: tc.NovelID, ItemID: existing.ID, ChapterID: a.ChapterID,
			Action: "acquired", Description: desc,
		}); err != nil {
			return nil, fmt.Errorf("record ownership acquired: %w", err)
		}
	}

	return &ToolResult{Success: true, Data: map[string]any{"item_id": existing.ID, "ownership_recorded": ownerChanged}}, nil
}

// ── delete_item / search_items ──

type DeleteItemArgs struct {
	ItemID int64 `json:"item_id" jsonschema:"required,description=物品ID" validate:"required,min=1"`
}
type DeleteItemTool struct{}

func (t *DeleteItemTool) Name() string                { return "delete_item" }
func (t *DeleteItemTool) Description() string         { return "删除物品条目（不可恢复）。【注意】删除前确认物品没有在剧情中承担叙事角色（key_prop/supporting）——被引用的物品应保留（用 update_item 标记 destroyed/consumed 终态），删除会导致历史章节物品记录悬空。" }
func (t *DeleteItemTool) Category() ToolCategory      { return CategoryWritingAssistant }
func (t *DeleteItemTool) JSONSchema() json.RawMessage { return SchemaOf(DeleteItemArgs{}) }
func (t *DeleteItemTool) ExposeToLLM() bool           { return true }
func (t *DeleteItemTool) NewArgs() any                { return &DeleteItemArgs{} }
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
func (t *SearchItemsTool) Description() string {
	return "按名称/能力/描述/来历搜索物品（最多返回 10 条）。返回格式：{items: [{id, name, item_type, grade, description, ability, owner_id, location_id, status}]}。" +
		"【使用时机】写物品相关情节前查特定物品；不确定物品名称/ID 时用它替代 get_items 的 list 浏览。" +
		"【省token】返回上限 10 条，命中按相关度排序——足够定位目标，命中后按需 get_items detail 取完整信息。"
}
func (t *SearchItemsTool) Category() ToolCategory      { return CategoryMemoryRetrieval }
func (t *SearchItemsTool) JSONSchema() json.RawMessage { return SchemaOf(SearchItemsArgs{}) }
func (t *SearchItemsTool) ExposeToLLM() bool           { return true }
func (t *SearchItemsTool) NewArgs() any                { return &SearchItemsArgs{} }
func (t *SearchItemsTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*SearchItemsArgs)
	items, err := item.NewStore(tc.DB, slog.Default()).Search(ctx, tc.NovelID, a.Query, 10)
	if err != nil {
		return nil, fmt.Errorf("search items: %w", err)
	}
	return &ToolResult{Success: true, Data: map[string]any{"items": items, "count": len(items)}}, nil
}

func RegisterItemTools(r *Registry) {
	r.Register(&GetItemsTool{})
	r.Register(&CreateItemTool{})
	r.Register(&UpdateItemTool{})
	r.Register(&DeleteItemTool{})
	r.Register(&SearchItemsTool{})
}
