package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"novel/internal/scene"
)

// ── get_scenes ──

type GetScenesArgs struct {
	ChapterID int64 `json:"chapter_id" jsonschema:"required,description=章节ID" validate:"required,min=1"`
	Brief     bool  `json:"brief"      jsonschema:"description=true=只返回id/title/chapter_id（省token）；false=返回完整数据"`
}
type GetScenesTool struct{}

func (t *GetScenesTool) Name() string { return "get_scenes" }
func (t *GetScenesTool) Description() string { return "获取指定章节的场景列表，按场景序号排列。brief=true 只返回id/title/chapter_id。" }
func (t *GetScenesTool) Category() ToolCategory { return CategoryNovelManagement }
func (t *GetScenesTool) JSONSchema() json.RawMessage { return SchemaOf(GetScenesArgs{}) }
func (t *GetScenesTool) ExposeToLLM() bool { return true }
func (t *GetScenesTool) NewArgs() any      { return &GetScenesArgs{} }
func (t *GetScenesTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*GetScenesArgs)
	items, err := scene.NewStore(tc.DB, slog.Default()).ListByChapter(ctx, tc.NovelID, a.ChapterID)
	if err != nil { return nil, fmt.Errorf("list scenes: %w", err) }
	if a.Brief {
		type briefScene struct {
			ID        int64  `json:"id"`
			Title     string `json:"title"`
			ChapterID int64  `json:"chapter_id"`
		}
		briefs := make([]briefScene, len(items))
		for i, s := range items {
			briefs[i] = briefScene{ID: s.ID, Title: s.Title, ChapterID: s.ChapterID}
		}
		return &ToolResult{Success: true, Data: map[string]any{"scenes": briefs, "count": len(briefs)}}, nil
	}
	return &ToolResult{Success: true, Data: map[string]any{"scenes": items, "count": len(items)}}, nil
}

// ── create_scene ──

type CreateSceneArgs struct {
	ChapterID    int64  `json:"chapter_id" jsonschema:"required,description=章节ID" validate:"required,min=1"`
	SceneNumber  int    `json:"scene_number" jsonschema:"required,description=场景序号（从1开始）,minimum=1" validate:"required,min=1"`
	Title        string `json:"title" jsonschema:"description=场景标题" jsonschema:"required,description=场景标题"`
	LocationID   int64  `json:"location_id" jsonschema:"description=场景地点ID" jsonschema:"required,description=场景地点ID"`
	CharacterIDs string `json:"character_ids" jsonschema:"description=出场角色ID数组JSON" jsonschema:"required,description=出场角色ID数组JSON，如[127,128,129]"`
	ArcID        int64  `json:"arc_id" jsonschema:"description=所属弧线ID" jsonschema:"description=所属弧线ID"`
	ArcNodeID    int64  `json:"arc_node_id" jsonschema:"description=对应弧线节点ID" jsonschema:"description=对应弧线节点ID"`
	WordCount    int    `json:"word_count" jsonschema:"description=场景字数"`
	Summary      string `json:"summary" jsonschema:"description=场景摘要，50-100字" jsonschema:"required,description=场景摘要，50-100字概述本场景发生什么"`
}
type CreateSceneTool struct{}

func (t *CreateSceneTool) Name() string { return "create_scene" }
func (t *CreateSceneTool) Description() string { return "为某章创建一个场景条目。" }
func (t *CreateSceneTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *CreateSceneTool) JSONSchema() json.RawMessage { return SchemaOf(CreateSceneArgs{}) }
func (t *CreateSceneTool) ExposeToLLM() bool { return true }
func (t *CreateSceneTool) NewArgs() any      { return &CreateSceneArgs{} }
func (t *CreateSceneTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*CreateSceneArgs)
	sc := &scene.Scene{
		NovelID: tc.NovelID, ChapterID: a.ChapterID, SceneNumber: a.SceneNumber,
		Title: a.Title, CharacterIDs: a.CharacterIDs, WordCount: a.WordCount, Summary: a.Summary,
	}
	if a.LocationID > 0 { sc.LocationID = &a.LocationID }
	if a.ArcID > 0 { sc.ArcID = &a.ArcID }
	if a.ArcNodeID > 0 { sc.ArcNodeID = &a.ArcNodeID }
	if err := scene.NewStore(tc.DB, slog.Default()).Create(ctx, sc); err != nil {
		return nil, fmt.Errorf("create scene: %w", err)
	}
	return &ToolResult{Success: true, Data: map[string]any{"id": sc.ID}}, nil
}

// ── update_scene ──

type UpdateSceneArgs struct {
	SceneID      int64  `json:"scene_id" jsonschema:"required,description=场景ID" validate:"required,min=1"`
	SceneNumber  int    `json:"scene_number" jsonschema:"description=场景序号,minimum=1"`
	Title        string `json:"title" jsonschema:"description=场景标题"`
	LocationID   int64  `json:"location_id" jsonschema:"description=场景地点ID"`
	CharacterIDs string `json:"character_ids" jsonschema:"description=出场角色ID数组JSON"`
	ArcID        int64  `json:"arc_id" jsonschema:"description=所属弧线ID"`
	ArcNodeID    int64  `json:"arc_node_id" jsonschema:"description=对应弧线节点ID"`
	WordCount    int    `json:"word_count" jsonschema:"minimum=0"`
	Summary      string `json:"summary" jsonschema:"description=场景摘要，50-100字"`
}
type UpdateSceneTool struct{}

func (t *UpdateSceneTool) Name() string { return "update_scene" }
func (t *UpdateSceneTool) Description() string { return "更新场景信息。PATCH 语义。" }
func (t *UpdateSceneTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *UpdateSceneTool) JSONSchema() json.RawMessage { return SchemaOf(UpdateSceneArgs{}) }
func (t *UpdateSceneTool) ExposeToLLM() bool { return true }
func (t *UpdateSceneTool) NewArgs() any      { return &UpdateSceneArgs{} }
func (t *UpdateSceneTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpdateSceneArgs)
	store := scene.NewStore(tc.DB, slog.Default())
	existing, err := store.GetByID(ctx, a.SceneID, tc.NovelID)
	if err != nil { return &ToolResult{Success: false, Error: fmt.Sprintf("场景不存在: %v", err)}, nil }
	if a.SceneNumber > 0 { existing.SceneNumber = a.SceneNumber }
	if a.Title != "" { existing.Title = a.Title }
	if a.LocationID > 0 { existing.LocationID = &a.LocationID }
	if a.CharacterIDs != "" { existing.CharacterIDs = a.CharacterIDs }
	if a.ArcID > 0 { existing.ArcID = &a.ArcID }
	if a.ArcNodeID > 0 { existing.ArcNodeID = &a.ArcNodeID }
	if a.WordCount > 0 { existing.WordCount = a.WordCount }
	if a.Summary != "" { existing.Summary = a.Summary }
	if err := store.Update(ctx, existing); err != nil { return nil, fmt.Errorf("update scene: %w", err) }
	return &ToolResult{Success: true, Data: map[string]any{"scene_id": existing.ID}}, nil
}

// ── delete_scene ──

type DeleteSceneArgs struct {
	SceneID int64 `json:"scene_id" jsonschema:"required,description=场景ID" validate:"required,min=1"`
}
type DeleteSceneTool struct{}
func (t *DeleteSceneTool) Name() string { return "delete_scene" }
func (t *DeleteSceneTool) Description() string { return "删除场景条目。" }
func (t *DeleteSceneTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *DeleteSceneTool) JSONSchema() json.RawMessage { return SchemaOf(DeleteSceneArgs{}) }
func (t *DeleteSceneTool) ExposeToLLM() bool { return true }
func (t *DeleteSceneTool) NewArgs() any      { return &DeleteSceneArgs{} }
func (t *DeleteSceneTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*DeleteSceneArgs)
	if err := scene.NewStore(tc.DB, slog.Default()).Delete(ctx, a.SceneID, tc.NovelID); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("删除失败: %v", err)}, nil
	}
	return &ToolResult{Success: true, Data: map[string]any{"deleted": true}}, nil
}

func RegisterSceneTools(r *Registry) {
	r.Register(&GetScenesTool{}); r.Register(&CreateSceneTool{})
	r.Register(&UpdateSceneTool{}); r.Register(&DeleteSceneTool{})
}
