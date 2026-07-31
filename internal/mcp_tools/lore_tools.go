package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"novel/internal/lore"
)

// ── get_lore ──

type GetLoreArgs struct {
	Mode     string `json:"mode" jsonschema:"required,description=查询模式：list=列表 detail=详情,enum=list,enum=detail,default=list" validate:"required,oneof=list detail"`
	LoreID   int64  `json:"lore_id" jsonschema:"description=设定ID（detail模式必填）" validate:"omitempty,min=1"`
	Category string `json:"category" jsonschema:"description=按分类筛选（list模式可选）"`
	Search   string `json:"search" jsonschema:"description=按标题/内容搜索（list模式可选）"`
	PageArgs
}

type GetLoreTool struct{}

func (t *GetLoreTool) Name() string { return "get_lore" }
func (t *GetLoreTool) Description() string {
	return "获取当前小说的世界观设定条目。list：按分类查看所有设定；detail：查看单条完整内容。设定包括力量体系、社会构成、历史事件、核心冲突等。"
}
func (t *GetLoreTool) Category() ToolCategory { return CategoryNovelManagement }
func (t *GetLoreTool) JSONSchema() json.RawMessage { return SchemaOf(GetLoreArgs{}) }
func (t *GetLoreTool) ExposeToLLM() bool           { return true }
func (t *GetLoreTool) NewArgs() any                { return &GetLoreArgs{} }
func (t *GetLoreTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*GetLoreArgs)
	a.NormalizePage()
	store := lore.NewStore(tc.DB, slog.Default())

	if a.Mode == "detail" {
		if a.LoreID <= 0 {
			return &ToolResult{Success: false, Error: "detail 模式需要 lore_id"}, nil
		}
		e, err := store.GetByID(ctx, a.LoreID, tc.NovelID)
		if err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("设定不存在: %v", err)}, nil
		}
		return &ToolResult{Success: true, Data: map[string]any{
			"id": e.ID, "title": e.Title, "category": e.Category,
			"content": e.Content, "summary": e.Summary,
			"reference_id": e.ReferenceID, "reference_type": e.ReferenceType,
			"tags": e.Tags, "version": e.Version,
		}}, nil
	}

	result, err := store.ListByNovel(ctx, tc.NovelID, lore.ListOptions{
		Page: a.Page, Size: a.Size, Category: a.Category, Search: a.Search,
	})
	if err != nil {
		return nil, fmt.Errorf("list lore: %w", err)
	}
	items := make([]map[string]any, len(result.Items))
	for i, e := range result.Items {
		items[i] = map[string]any{
			"id": e.ID, "title": e.Title, "category": e.Category,
			"summary": e.Summary, "tags": e.Tags, "version": e.Version,
		}
	}
	data := PageMeta(result)
	data["items"] = items
	return &ToolResult{Success: true, Data: data}, nil
}

// ── create_lore ──

type CreateLoreArgs struct {
	Title           string `json:"title" jsonschema:"required,description=设定标题" validate:"required"`
	Category        string `json:"category" jsonschema:"required,description=分类：力量体系/社会构成/历史事件/核心冲突/天道法则/文化习俗/种族/地理概述" validate:"required"`
	Content         string `json:"content" jsonschema:"required,description=设定正文（Markdown）" validate:"required"`
	Summary         string `json:"summary" jsonschema:"description=一句话摘要"`
	ArcID           int64  `json:"arc_id" jsonschema:"required,description=关联弧线ID"`
	RevealChapterID int64  `json:"reveal_chapter_id" jsonschema:"required,description=读者首次得知此设定的章节ID"`
	IsPublic        bool   `json:"is_public" jsonschema:"description=是否公开设定（读者已知），false=秘密,default=true"`
	ReferenceID     int64  `json:"reference_id" jsonschema:"description=关联实体ID"`
	ReferenceType   string `json:"reference_type" jsonschema:"description=关联类型：location/character"`
	Tags            string `json:"tags" jsonschema:"description=JSON标签数组"`
}

type CreateLoreTool struct{}

func (t *CreateLoreTool) Name() string { return "create_lore" }
func (t *CreateLoreTool) Description() string {
	return "创建世界观设定条目。创建后自动绑定到当前小说。与 update_lore 保持独立，新建用此工具，修改用 update_lore。" +
		"category 可选值：力量体系/社会构成/历史事件/核心冲突/天道法则/文化习俗/种族/地理概述。" +
		"arc_id 关联此设定所属的弧线；reveal_chapter_id 填入读者首次得知此设定的章节ID（控制信息投放节奏）；" +
		"is_public=true 表示读者已知的公开设定，false 表示秘密（未来反转用）。"
}
func (t *CreateLoreTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *CreateLoreTool) JSONSchema() json.RawMessage { return SchemaOf(CreateLoreArgs{}) }
func (t *CreateLoreTool) ExposeToLLM() bool           { return true }
func (t *CreateLoreTool) NewArgs() any                { return &CreateLoreArgs{} }
func (t *CreateLoreTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*CreateLoreArgs)
	entry := &lore.LoreEntry{
		NovelID: tc.NovelID, Title: a.Title, Category: a.Category,
		Content: a.Content, Summary: a.Summary, IsPublic: a.IsPublic,
		ReferenceID: &a.ReferenceID, ReferenceType: a.ReferenceType, Tags: a.Tags,
	}
	if a.ArcID > 0 { entry.ArcID = &a.ArcID }
	if a.RevealChapterID > 0 { entry.RevealChapterID = &a.RevealChapterID }
	if a.ReferenceID <= 0 { entry.ReferenceID = nil }
	if err := lore.NewStore(tc.DB, slog.Default()).Create(ctx, entry); err != nil {
		return nil, fmt.Errorf("create lore: %w", err)
	}
	return &ToolResult{Success: true, Data: map[string]any{"id": entry.ID}}, nil
}

// ── update_lore ──

type UpdateLoreArgs struct {
	LoreID          int64   `json:"lore_id" jsonschema:"required,description=设定ID" validate:"required,min=1"`
	Title           string  `json:"title" jsonschema:"description=新的标题"`
	Category        string  `json:"category" jsonschema:"description=新的分类"`
	Content         string  `json:"content" jsonschema:"description=新的正文"`
	Summary         string  `json:"summary" jsonschema:"description=新的摘要"`
	ArcID           *int64  `json:"arc_id,omitempty" jsonschema:"description=关联弧线ID"`
	RevealChapterID *int64  `json:"reveal_chapter_id,omitempty" jsonschema:"description=揭示章节ID"`
	IsPublic        *bool   `json:"is_public,omitempty" jsonschema:"description=是否公开"`
	ReferenceID     int64   `json:"reference_id" jsonschema:"description=关联实体ID"`
	ReferenceType   string  `json:"reference_type" jsonschema:"description=关联实体类型"`
	Tags            string  `json:"tags" jsonschema:"description=标签JSON数组"`
}

type UpdateLoreTool struct{}

func (t *UpdateLoreTool) Name() string { return "update_lore" }
func (t *UpdateLoreTool) Description() string {
	return "更新世界观设定条目。PATCH 语义，只传需要修改的字段。version 自动递增。"
}
func (t *UpdateLoreTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *UpdateLoreTool) JSONSchema() json.RawMessage { return SchemaOf(UpdateLoreArgs{}) }
func (t *UpdateLoreTool) ExposeToLLM() bool           { return true }
func (t *UpdateLoreTool) NewArgs() any                { return &UpdateLoreArgs{} }
func (t *UpdateLoreTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpdateLoreArgs)
	store := lore.NewStore(tc.DB, slog.Default())
	existing, err := store.GetByID(ctx, a.LoreID, tc.NovelID)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("设定不存在: %v", err)}, nil
	}
	if a.Title != "" { existing.Title = a.Title }
	if a.Category != "" { existing.Category = a.Category }
	if a.Content != "" { existing.Content = a.Content }
	if a.Summary != "" { existing.Summary = a.Summary }
	if a.ArcID != nil { existing.ArcID = a.ArcID }
	if a.RevealChapterID != nil { existing.RevealChapterID = a.RevealChapterID }
	if a.IsPublic != nil { existing.IsPublic = *a.IsPublic }
	if a.ReferenceID > 0 { existing.ReferenceID = &a.ReferenceID; existing.ReferenceType = a.ReferenceType }
	if a.Tags != "" { existing.Tags = a.Tags }
	if err := store.Update(ctx, existing); err != nil {
		return nil, fmt.Errorf("update lore: %w", err)
	}
	return &ToolResult{Success: true, Data: map[string]any{"lore_id": existing.ID, "version": existing.Version + 1}}, nil
}

// ── delete_lore ──

type DeleteLoreArgs struct {
	LoreID int64 `json:"lore_id" jsonschema:"required,description=设定ID" validate:"required,min=1"`
}

type DeleteLoreTool struct{}

func (t *DeleteLoreTool) Name() string { return "delete_lore" }
func (t *DeleteLoreTool) Description() string { return "删除世界观设定条目。" }
func (t *DeleteLoreTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *DeleteLoreTool) JSONSchema() json.RawMessage { return SchemaOf(DeleteLoreArgs{}) }
func (t *DeleteLoreTool) ExposeToLLM() bool           { return true }
func (t *DeleteLoreTool) NewArgs() any                { return &DeleteLoreArgs{} }
func (t *DeleteLoreTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*DeleteLoreArgs)
	store := lore.NewStore(tc.DB, slog.Default())
	if err := store.Delete(ctx, a.LoreID, tc.NovelID); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("删除失败: %v", err)}, nil
	}
	return &ToolResult{Success: true, Data: map[string]any{"deleted": true}}, nil
}

// ── search_lore ──

type SearchLoreArgs struct {
	Query string `json:"query" jsonschema:"required,description=搜索关键词" validate:"required"`
}

type SearchLoreTool struct{}

func (t *SearchLoreTool) Name() string { return "search_lore" }
func (t *SearchLoreTool) Description() string { return "全文搜索所有世界观设定条目。返回格式：{lore: [{id, title, category, content, arc_id}]}。匹配 title/category/content/tags 字段。当需要查找特定概念、规则、历史时使用。" }
func (t *SearchLoreTool) Category() ToolCategory { return CategoryMemoryRetrieval }
func (t *SearchLoreTool) JSONSchema() json.RawMessage { return SchemaOf(SearchLoreArgs{}) }
func (t *SearchLoreTool) ExposeToLLM() bool           { return true }
func (t *SearchLoreTool) NewArgs() any                { return &SearchLoreArgs{} }
func (t *SearchLoreTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*SearchLoreArgs)
	items, err := lore.NewStore(tc.DB, slog.Default()).Search(ctx, tc.NovelID, a.Query, 10)
	if err != nil {
		return nil, fmt.Errorf("search lore: %w", err)
	}
	return &ToolResult{Success: true, Data: map[string]any{"items": items, "count": len(items)}}, nil
}

// ── 注册 ──

func RegisterLoreTools(r *Registry) {
	r.Register(&GetLoreTool{})
	r.Register(&CreateLoreTool{})
	r.Register(&UpdateLoreTool{})
	r.Register(&DeleteLoreTool{})
	r.Register(&SearchLoreTool{})
}
