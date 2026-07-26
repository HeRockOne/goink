package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"

	"novel/internal/writing"
)

// ── get_writing_snapshot ──

type GetWritingSnapshotArgs struct{}

type GetWritingSnapshotTool struct{}

func (t *GetWritingSnapshotTool) Name() string { return "get_writing_snapshot" }
func (t *GetWritingSnapshotTool) Description() string {
	return "获取当前写作进度快照：最新章节号、当前弧线、活跃角色、待处理剧情线索、一句话状态摘要。在开始写新章节前调用，快速了解当前进展。"
}
func (t *GetWritingSnapshotTool) Category() ToolCategory { return CategoryMemoryRetrieval }
func (t *GetWritingSnapshotTool) JSONSchema() json.RawMessage { return SchemaOf(GetWritingSnapshotArgs{}) }
func (t *GetWritingSnapshotTool) ExposeToLLM() bool           { return true }
func (t *GetWritingSnapshotTool) NewArgs() any                { return &GetWritingSnapshotArgs{} }
func (t *GetWritingSnapshotTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	snap, err := writing.NewSnapshotStore(tc.DB, nil).Get(ctx, tc.NovelID)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("暂无写作快照，请先写第一章: %v", err)}, nil
	}
	return &ToolResult{Success: true, Data: map[string]any{
		"last_chapter_num":  snap.LastChapterNum,
		"last_chapter_id":   snap.LastChapterID,
		"current_arc_id":    snap.CurrentArcID,
		"current_location":  snap.CurrentLocation,
		"active_chars":      snap.ActiveChars,
		"pending_threads":   snap.PendingThreads,
		"summary":           snap.Summary,
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
	return "更新写作进度快照。PATCH 语义。写完一章后调用此工具更新进度。"
}
func (t *UpdateWritingSnapshotTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *UpdateWritingSnapshotTool) JSONSchema() json.RawMessage { return SchemaOf(UpdateWritingSnapshotArgs{}) }
func (t *UpdateWritingSnapshotTool) ExposeToLLM() bool           { return true }
func (t *UpdateWritingSnapshotTool) NewArgs() any                { return &UpdateWritingSnapshotArgs{} }
func (t *UpdateWritingSnapshotTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpdateWritingSnapshotArgs)
	snap := &writing.WritingSnapshot{NovelID: tc.NovelID}
	if a.LastChapterID > 0 { snap.LastChapterID = a.LastChapterID }
	if a.LastChapterNum > 0 { snap.LastChapterNum = a.LastChapterNum }
	if a.CurrentArcID > 0 { snap.CurrentArcID = &a.CurrentArcID }
	if a.CurrentLocation != "" { snap.CurrentLocation = a.CurrentLocation }
	if a.ActiveChars != "" { snap.ActiveChars = a.ActiveChars }
	if a.PendingThreads != "" { snap.PendingThreads = a.PendingThreads }
	if a.Summary != "" { snap.Summary = a.Summary }
	if a.DetailedState != "" { snap.DetailedState = a.DetailedState }

	store := writing.NewSnapshotStore(tc.DB, nil)
	// 先读现有数据，合并更新
	if existing, err := store.Get(ctx, tc.NovelID); err == nil {
		if a.LastChapterID == 0 { snap.LastChapterID = existing.LastChapterID }
		if a.LastChapterNum == 0 { snap.LastChapterNum = existing.LastChapterNum }
		if a.CurrentArcID == 0 && existing.CurrentArcID != nil { snap.CurrentArcID = existing.CurrentArcID }
		if a.CurrentLocation == "" { snap.CurrentLocation = existing.CurrentLocation }
		if a.ActiveChars == "" { snap.ActiveChars = existing.ActiveChars }
		if a.PendingThreads == "" { snap.PendingThreads = existing.PendingThreads }
		if a.Summary == "" { snap.Summary = existing.Summary }
		if a.DetailedState == "" { snap.DetailedState = existing.DetailedState }
	}
	if err := store.Upsert(ctx, snap); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("更新快照失败: %v", err)}, nil
	}
	return &ToolResult{Success: true, Data: map[string]any{"ok": true}}, nil
}

func RegisterSnapshotTools(r *Registry) {
	r.Register(&GetWritingSnapshotTool{})
	r.Register(&UpdateWritingSnapshotTool{})
}
