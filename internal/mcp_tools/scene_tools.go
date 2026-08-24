package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"novel/internal/scene"
	"gorm.io/gorm"
)

// ── get_scenes ──

type GetScenesArgs struct {
		ChapterID int64 `json:"chapter_id" jsonschema:"description=章节ID（留空返回全部场景）" validate:"omitempty,min=1"`
		Brief     bool  `json:"brief"      jsonschema:"description=true=只返回id/title/chapter_id（省token）；false=返回完整数据"`
	}
	type GetScenesTool struct{}

	func (t *GetScenesTool) Name() string { return "get_scenes" }
	func (t *GetScenesTool) Description() string { return "获取场景列表。传入chapter_id按章节查（推荐，最省token）；不传只返回最近100个场景（按章倒序，旧场景用 get_writing_context 的卷实体索引查）。brief=true 只返回id/title/chapter_id。" }
	func (t *GetScenesTool) Category() ToolCategory { return CategoryNovelManagement }
	func (t *GetScenesTool) JSONSchema() json.RawMessage { return SchemaOf(GetScenesArgs{}) }
	func (t *GetScenesTool) ExposeToLLM() bool { return true }
	func (t *GetScenesTool) NewArgs() any      { return &GetScenesArgs{} }
	func (t *GetScenesTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
		a := args.(*GetScenesArgs)
		store := scene.NewStore(tc.DB, slog.Default())
		var items []scene.Scene
		var err error
		if a.ChapterID > 0 {
			items, err = store.ListByChapter(ctx, tc.NovelID, a.ChapterID)
		} else {
			items, err = store.ListByNovel(ctx, tc.NovelID)
		}
		if err != nil { return nil, fmt.Errorf("list scenes: %w", err) }
		if a.Brief {
			type briefScene struct {
				ID        int64  `json:"id"`
				Title     string `json:"title"`
				ChapterID *int64 `json:"chapter_id"`
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

// CreateSceneItem 是 create_scene 的单条参数。
type CreateSceneItem struct {
		ChapterID    int64  `json:"chapter_id" jsonschema:"description=章节ID。规划阶段可不填，写完后回填" validate:"omitempty,min=1"`
		SceneNumber  int    `json:"scene_number" jsonschema:"required,description=场景序号（从1开始）,minimum=1" validate:"required,min=1"`
		Title        string `json:"title" jsonschema:"required,description=场景标题"`
		LocationID   int64  `json:"location_id" jsonschema:"required,description=场景地点ID"`
		CharacterIDs string `json:"character_ids" jsonschema:"required,description=出场角色ID数组JSON，如[127,128,129]"`
		ArcID        int64  `json:"arc_id" jsonschema:"description=所属弧线ID"`
		ArcNodeID    int64  `json:"arc_node_id" jsonschema:"description=对应弧线节点ID"`
		WordCount    int    `json:"word_count" jsonschema:"description=场景字数"`
		Summary      string `json:"summary" jsonschema:"required,description=场景摘要，50-100字概述本场景发生什么" validate:"required"`
	}

// CreateSceneArgs 是 create_scene 的参数（批量，1-5条）。
type CreateSceneArgs struct {
		Scenes []CreateSceneItem `json:"scenes" jsonschema:"required,description=要创建的场景列表（1-5个），每条包含 scene_number/title/location_id/character_ids/summary 等字段" validate:"required,min=1,max=5,dive"`
	}
	type CreateSceneTool struct{}

	func (t *CreateSceneTool) Name() string { return "create_scene" }
	func (t *CreateSceneTool) Description() string { return "批量创建场景（1-5条）。保证原子性，失败时返回具体条目原因。场景 = 章节内的叙事单元，含地点/出场角色/字数/摘要。规划阶段 chapter_id 可不填，写完后用 update_scene 回填。" +
		"每条需传入 scene_number/title/location_id/character_ids/summary。" +
		"【使用时机】大纲/细纲规划本章场景时建条目；写作完成回填 chapter_id 与字数。" +
		"【注意】character_ids 是出场角色 ID 数组——场景是 get_writing_context 推断本章出场角色的数据源，漏填会导致写作上下文缺角色。" }
	func (t *CreateSceneTool) Category() ToolCategory { return CategoryWritingAssistant }
	func (t *CreateSceneTool) JSONSchema() json.RawMessage { return SchemaOf(CreateSceneArgs{}) }
	func (t *CreateSceneTool) ExposeToLLM() bool { return true }
	func (t *CreateSceneTool) NewArgs() any      { return &CreateSceneArgs{} }
	func (t *CreateSceneTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
		a := args.(*CreateSceneArgs)
		var ids []int64
		var failedTitle string
		var failedErr error
		err := tc.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			for _, item := range a.Scenes {
				sc := &scene.Scene{
					NovelID: tc.NovelID, SceneNumber: item.SceneNumber,
					Title: item.Title, CharacterIDs: item.CharacterIDs, WordCount: item.WordCount, Summary: item.Summary,
				}
				if item.ChapterID > 0 { sc.ChapterID = &item.ChapterID }
				if item.LocationID > 0 { sc.LocationID = &item.LocationID }
				if item.ArcID > 0 { sc.ArcID = &item.ArcID }
				if item.ArcNodeID > 0 { sc.ArcNodeID = &item.ArcNodeID }
				if err := scene.NewStore(tx, slog.Default()).Create(ctx, sc); err != nil {
					failedTitle = item.Title
					failedErr = err
					return err
				}
				ids = append(ids, sc.ID)
			}
			return nil
		})
		if err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("创建场景 [%s] 失败: %s", failedTitle, failedErr)}, nil
		}
		return &ToolResult{Success: true, Data: map[string]any{"ids": ids, "count": len(ids)}}, nil
	}

// ── update_scene ──

type UpdateSceneArgs struct {
		SceneID      int64  `json:"scene_id" jsonschema:"required,description=场景ID" validate:"required,min=1"`
		ChapterID    int64  `json:"chapter_id" jsonschema:"description=章节ID（写完后回填，规划场景转为正式场景）"`
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
	func (t *UpdateSceneTool) Description() string { return "更新场景信息。PATCH 语义。写完后用 chapter_id 回填规划场景；章节写作中场景发生变化（换地点/换出场角色）时同步。" }
	func (t *UpdateSceneTool) Category() ToolCategory { return CategoryWritingAssistant }
	func (t *UpdateSceneTool) JSONSchema() json.RawMessage { return SchemaOf(UpdateSceneArgs{}) }
	func (t *UpdateSceneTool) ExposeToLLM() bool { return true }
	func (t *UpdateSceneTool) NewArgs() any      { return &UpdateSceneArgs{} }
	func (t *UpdateSceneTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
		a := args.(*UpdateSceneArgs)
		store := scene.NewStore(tc.DB, slog.Default())
		existing, err := store.GetByID(ctx, a.SceneID, tc.NovelID)
		if err != nil { return &ToolResult{Success: false, Error: fmt.Sprintf("场景不存在: %v", err)}, nil }
		if a.ChapterID > 0 { existing.ChapterID = &a.ChapterID }
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
func (t *DeleteSceneTool) Description() string { return "删除场景条目（不可恢复）。【注意】删除前确认场景不是本章情节的关键节点（场景承载角色出场/事件推进）——误删会导致该章场景信息缺失，后续写作无法核对出场角色。" }
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
