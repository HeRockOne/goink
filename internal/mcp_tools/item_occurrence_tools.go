package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"novel/internal/itemoccurrence"
)

// ── get_item_occurrences ────────────────────────────────

type GetItemOccurrencesArgs struct {
	ItemID int64 `json:"item_id" jsonschema:"required,description=物品ID" validate:"required,min=1"`
}

type GetItemOccurrencesTool struct{}

func (t *GetItemOccurrencesTool) Name() string { return "get_item_occurrences" }
func (t *GetItemOccurrencesTool) Description() string {
	return "获取某物品在所有章节中的出现/流转记录（最近 50 条，按章倒序）。返回格式：{occurrences: [{id, item_id, chapter_id, action, description, created_at}]}。action 取值：acquired/used/lost/destroyed/mentioned。每次物品易主、使用、丢失时用 create_item_occurrence 记录。" +
		"【关联场景】写作前调用此工具查物品历史，可避免物品位置/持有人前后矛盾。"
}
func (t *GetItemOccurrencesTool) Category() ToolCategory { return CategoryMemoryRetrieval }
func (t *GetItemOccurrencesTool) JSONSchema() json.RawMessage { return SchemaOf(GetItemOccurrencesArgs{}) }
func (t *GetItemOccurrencesTool) ExposeToLLM() bool           { return true }
func (t *GetItemOccurrencesTool) NewArgs() any                { return &GetItemOccurrencesArgs{} }

func (t *GetItemOccurrencesTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*GetItemOccurrencesArgs)
	store := itemoccurrence.NewStore(tc.DB, slog.Default())
	items, err := store.ListByItem(ctx, tc.NovelID, a.ItemID)
	if err != nil {
		return nil, fmt.Errorf("get item occurrences: %w", err)
	}
	return &ToolResult{Success: true, Data: map[string]any{"occurrences": items}}, nil
}

// ── create_item_occurrence ─────────────────────────────

type CreateItemOccurrenceArgs struct {
	ItemID      int64  `json:"item_id"      jsonschema:"required,description=物品ID"                     validate:"required,min=1"`
	ChapterID   int64  `json:"chapter_id"   jsonschema:"required,description=章节ID"                    validate:"required,min=1"`
	Action      string `json:"action"       jsonschema:"required,description=动作类型: acquired/used/lost/destroyed/mentioned"`
	Description string `json:"description"  jsonschema:"description=该章节中物品的具体表现或状态描述"`
}

type CreateItemOccurrenceTool struct{}

func (t *CreateItemOccurrenceTool) Name() string { return "create_item_occurrence" }
func (t *CreateItemOccurrenceTool) Description() string {
	return "记录物品在指定章节中的出现或状态变化。每次物品易主、使用、丢失、销毁时都应记录，便于 AI 追踪物品流向。" +
		"【关联场景】每次更新 item 的 owner_id 时，应同时用此工具记录一条 action=acquired/lost 的记录。" +
		"action 必填：acquired（获得）、used（使用）、lost（丢失）、destroyed（销毁）、mentioned（提及）。"
}
func (t *CreateItemOccurrenceTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *CreateItemOccurrenceTool) JSONSchema() json.RawMessage {
	return SchemaOf(CreateItemOccurrenceArgs{})
}
func (t *CreateItemOccurrenceTool) ExposeToLLM() bool { return true }
func (t *CreateItemOccurrenceTool) NewArgs() any      { return &CreateItemOccurrenceArgs{} }

func (t *CreateItemOccurrenceTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*CreateItemOccurrenceArgs)
	action := a.Action
	if action == "" {
		action = "mentioned"
	}
	o := &itemoccurrence.ItemOccurrence{
		NovelID:     tc.NovelID,
		ItemID:      a.ItemID,
		ChapterID:   a.ChapterID,
		Action:      action,
		Description: a.Description,
	}
	store := itemoccurrence.NewStore(tc.DB, slog.Default())
	if err := store.Create(ctx, o); err != nil {
		return nil, fmt.Errorf("create item occurrence: %w", err)
	}
	return &ToolResult{Success: true, Data: map[string]any{"id": o.ID}}, nil
}

// ── 注册 ──────────────────────────────────────────────

func RegisterItemOccurrenceTools(r *Registry) {
	r.Register(&GetItemOccurrencesTool{})
	r.Register(&CreateItemOccurrenceTool{})
}
