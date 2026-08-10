package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"

	"novel/internal/character"
	"novel/internal/writing"
)

// ── get_writing_snapshot ──

type GetWritingSnapshotArgs struct{}

type GetWritingSnapshotTool struct{}

func (t *GetWritingSnapshotTool) Name() string { return "get_writing_snapshot" }
func (t *GetWritingSnapshotTool) Description() string {
	return "获取当前写作进度快照：最新章节号、当前弧线、活跃角色、待处理剧情线索、一句话状态摘要。在开始写新章节前调用，快速了解当前进展。" +
		"【注意】这是轻量进度卡——需要完整创作上下文（角色状态/伏笔/场景）用 get_writing_context，不要用本工具替代。"
}
func (t *GetWritingSnapshotTool) Category() ToolCategory { return CategoryMemoryRetrieval }
func (t *GetWritingSnapshotTool) JSONSchema() json.RawMessage {
	return SchemaOf(GetWritingSnapshotArgs{})
}
func (t *GetWritingSnapshotTool) ExposeToLLM() bool { return true }
func (t *GetWritingSnapshotTool) NewArgs() any      { return &GetWritingSnapshotArgs{} }
func (t *GetWritingSnapshotTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	snap, err := writing.NewSnapshotStore(tc.DB, nil).Get(ctx, tc.NovelID)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("暂无写作快照，请先写第一章: %v", err)}, nil
	}
	return &ToolResult{Success: true, Data: map[string]any{
		"last_chapter_num": snap.LastChapterNum,
		"last_chapter_id":  snap.LastChapterID,
		"current_arc_id":   snap.CurrentArcID,
		"current_location": snap.CurrentLocation,
		"active_chars":     snap.ActiveChars,
		"pending_threads":  snap.PendingThreads,
		"summary":          snap.Summary,
	}}, nil
}

// ── update_writing_snapshot ──

type UpdateWritingSnapshotArgs struct {
	LastChapterID   int64  `json:"last_chapter_id" jsonschema:"description=最新章节ID"`
	LastChapterNum  int    `json:"last_chapter_num" jsonschema:"description=最新章节号,minimum=0"`
	CurrentArcID    int64  `json:"current_arc_id" jsonschema:"description=当前弧线ID"`
	CurrentLocation string `json:"current_location" jsonschema:"description=当前焦点地点"`
	ActiveChars     string `json:"active_chars" jsonschema:"description=活跃角色ID数组JSON"`
	PendingThreads  string `json:"pending_threads" jsonschema:"description=待处理剧情线索"`
	Summary         string `json:"summary" jsonschema:"required,description=一句话状态摘要"`
	DetailedState   string `json:"detailed_state" jsonschema:"description=详细状态（Markdown）"`
}

type UpdateWritingSnapshotTool struct{}

func (t *UpdateWritingSnapshotTool) Name() string { return "update_writing_snapshot" }
func (t *UpdateWritingSnapshotTool) Description() string {
	return "更新写作进度快照。PATCH 语义。写完一章后调用此工具更新进度（last_chapter_num/summary/active_chars 等）。" +
		"【使用时机】每章完成（含批量每章迷你维护）必须更新——下一章 get_writing_context 读快照判断进度，漏更新会导致连续写错章节号。"
}
func (t *UpdateWritingSnapshotTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *UpdateWritingSnapshotTool) JSONSchema() json.RawMessage {
	return SchemaOf(UpdateWritingSnapshotArgs{})
}
func (t *UpdateWritingSnapshotTool) ExposeToLLM() bool { return true }
func (t *UpdateWritingSnapshotTool) NewArgs() any      { return &UpdateWritingSnapshotArgs{} }
func (t *UpdateWritingSnapshotTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpdateWritingSnapshotArgs)
	snap := &writing.WritingSnapshot{NovelID: tc.NovelID}
	if a.LastChapterID > 0 {
		snap.LastChapterID = a.LastChapterID
	}
	if a.LastChapterNum > 0 {
		snap.LastChapterNum = a.LastChapterNum
	}
	if a.CurrentArcID > 0 {
		snap.CurrentArcID = &a.CurrentArcID
	}
	if a.CurrentLocation != "" {
		snap.CurrentLocation = a.CurrentLocation
	}
	if a.ActiveChars != "" {
		snap.ActiveChars = a.ActiveChars
	}
	if a.PendingThreads != "" {
		snap.PendingThreads = a.PendingThreads
	}
	if a.Summary != "" {
		snap.Summary = a.Summary
	}
	if a.DetailedState != "" {
		snap.DetailedState = a.DetailedState
	}

	// 活跃角色校验：ID 必须属于当前小说且状态非 dead，防止快照与 characters 表脱节
	if a.ActiveChars != "" {
		if err := validateActiveChars(ctx, tc, a.ActiveChars); err != nil {
			return &ToolResult{Success: false, Error: err.Error()}, nil
		}
	}

	store := writing.NewSnapshotStore(tc.DB, nil)
	// 先读现有数据，合并更新
	if existing, err := store.Get(ctx, tc.NovelID); err == nil {
		if a.LastChapterID == 0 {
			snap.LastChapterID = existing.LastChapterID
		}
		if a.LastChapterNum == 0 {
			snap.LastChapterNum = existing.LastChapterNum
		}
		if a.CurrentArcID == 0 && existing.CurrentArcID != nil {
			snap.CurrentArcID = existing.CurrentArcID
		}
		if a.CurrentLocation == "" {
			snap.CurrentLocation = existing.CurrentLocation
		}
		if a.ActiveChars == "" {
			snap.ActiveChars = existing.ActiveChars
		}
		if a.PendingThreads == "" {
			snap.PendingThreads = existing.PendingThreads
		}
		if a.Summary == "" {
			snap.Summary = existing.Summary
		}
		if a.DetailedState == "" {
			snap.DetailedState = existing.DetailedState
		}
	}
	if err := store.Upsert(ctx, snap); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("更新快照失败: %v", err)}, nil
	}
	return &ToolResult{Success: true, Data: map[string]any{"ok": true}}, nil
}

// validateActiveChars 校验活跃角色 ID 数组：必须属于当前小说且状态为 alive/missing/unknown。
func validateActiveChars(ctx context.Context, tc ToolContext, activeChars string) error {
	ids := parseJSONInt64Array(activeChars)
	if len(ids) == 0 {
		return nil
	}
	var chars []character.Character
	if err := tc.DB.WithContext(ctx).Where("novel_id = ? AND id IN ?", tc.NovelID, ids).Find(&chars).Error; err != nil {
		return fmt.Errorf("校验活跃角色失败: %v", err)
	}
	found := map[int64]bool{}
	var invalid []string
	for _, c := range chars {
		found[c.ID] = true
		if c.Status == "dead" {
			invalid = append(invalid, fmt.Sprintf("%s(已死亡)", c.Name))
		}
	}
	for _, id := range ids {
		if !found[id] {
			invalid = append(invalid, fmt.Sprintf("ID %d 不存在", id))
		}
	}
	if len(invalid) > 0 {
		return fmt.Errorf("活跃角色列表包含无效条目: %v（请修正后再保存）", invalid)
	}
	return nil
}

func RegisterSnapshotTools(r *Registry) {
	r.Register(&GetWritingSnapshotTool{})
	r.Register(&UpdateWritingSnapshotTool{})
}
