package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"

	"novel/internal/chapter"
)

// ── update_chapter_meta ──────────────────────────────────

type UpdateChapterMetaArgs struct {
	ChapterID    int64   `json:"chapter_id"    jsonschema:"required,description=章节ID" validate:"required,min=1"`
	Summary      string  `json:"summary"       jsonschema:"required,description=章节摘要/概述，50-200字，让下一章 AI 快速了解本章发生了什么"`
	KeyEvents    string  `json:"key_events"    jsonschema:"required,description=JSON数组，本章关键事件列表，如[\"发现祭坛\",\"激活封印\"]"`
	CharactersIn string  `json:"characters_in" jsonschema:"required,description=JSON数组，本章出场角色ID列表，如[127,128,129]"`
	ArcIDs       string  `json:"arc_ids"       jsonschema:"required,description=JSON数组，本章涉及的弧线ID列表，如[68]"`
}

type UpdateChapterMetaTool struct{}

func (t *UpdateChapterMetaTool) Name() string { return "update_chapter_meta" }
func (t *UpdateChapterMetaTool) Description() string {
	return "更新章节的元数据（摘要、关键事件、出场角色、关联弧线）。" +
		"每次写完一章后，必须调用此工具更新 summary 和 key_events，" +
		"否则下一章 get_writing_context 的 recent_chapters 中看不到本章摘要。" +
		"【关联场景】写完正文后，在 maintain 阶段调用此工具，让后续创作知道本章发生了什么。"
}
func (t *UpdateChapterMetaTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *UpdateChapterMetaTool) JSONSchema() json.RawMessage {
	return SchemaOf(UpdateChapterMetaArgs{})
}
func (t *UpdateChapterMetaTool) ExposeToLLM() bool { return true }
func (t *UpdateChapterMetaTool) NewArgs() any      { return &UpdateChapterMetaArgs{} }

func (t *UpdateChapterMetaTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpdateChapterMetaArgs)

	var ch chapter.Chapter
	if err := tc.DB.WithContext(ctx).Where("id = ? AND novel_id = ?", a.ChapterID, tc.NovelID).First(&ch).Error; err != nil {
		return nil, fmt.Errorf("chapter not found: %w", err)
	}

	updates := map[string]any{}
	if a.Summary != "" {
		updates["summary"] = a.Summary
	}
	if a.KeyEvents != "" {
		updates["key_events"] = a.KeyEvents
	}
	if a.CharactersIn != "" {
		updates["characters_in"] = a.CharactersIn
	}
	if a.ArcIDs != "" {
		updates["arc_ids"] = a.ArcIDs
	}

	if len(updates) == 0 {
		return &ToolResult{Success: false, Error: "至少需要提供一个要更新的字段"}, nil
	}

	if err := tc.DB.WithContext(ctx).Model(&ch).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("update chapter meta: %w", err)
	}

	return &ToolResult{Success: true, Data: map[string]any{"updated": true}}, nil
}

func RegisterChapterMetaTool(r *Registry) {
	r.Register(&UpdateChapterMetaTool{})
}
