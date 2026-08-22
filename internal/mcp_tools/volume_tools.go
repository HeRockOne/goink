package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"novel/internal/volume"
)

// ── get_volumes ────────────────────────────────────────────

type GetVolumesArgs struct{}

type GetVolumesTool struct{}

func (t *GetVolumesTool) Name() string { return "get_volumes" }
func (t *GetVolumesTool) Description() string {
	return "获取当前小说的所有卷（按章节顺序排列）。返回卷名、章节范围、描述、结构化卷纲。"
}
func (t *GetVolumesTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *GetVolumesTool) JSONSchema() json.RawMessage {
	return SchemaOf(GetVolumesArgs{})
}
func (t *GetVolumesTool) ExposeToLLM() bool { return true }
func (t *GetVolumesTool) NewArgs() any      { return &GetVolumesArgs{} }

func (t *GetVolumesTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	store := volume.NewStore(tc.DB)
	volumes, err := store.ListByNovel(ctx, tc.NovelID)
	if err != nil {
		return nil, fmt.Errorf("list volumes: %w", err)
	}
	if len(volumes) == 0 {
		return &ToolResult{Success: true, Data: map[string]any{"content": "暂无卷数据"}}, nil
	}

	list := make([]map[string]any, 0, len(volumes))
	for _, v := range volumes {
		item := map[string]any{
			"id":            v.ID,
			"name":          v.Name,
			"description":   v.Description,
			"start_chapter": v.StartChapter,
			"end_chapter":   v.EndChapter,
			"sort_order":    v.SortOrder,
		}
		if v.DetailJSON != "" {
			var detail map[string]any
			if err := json.Unmarshal([]byte(v.DetailJSON), &detail); err == nil {
				item["detail"] = detail
			} else {
				// 非 JSON 格式（如 > 键：值 行格式），原样返回
				item["detail_raw"] = v.DetailJSON
			}
		}
		list = append(list, item)
	}

	return &ToolResult{Success: true, Data: map[string]any{"volumes": list}}, nil
}

// ── create_volume ──────────────────────────────────────────

type CreateVolumeArgs struct {
	Name         string `json:"name" jsonschema:"required,description=卷名（如'第一卷·崛起'）" validate:"required"`
	Description  string `json:"description" jsonschema:"description=卷纲概述（一句话）"`
	StartChapter int    `json:"start_chapter" jsonschema:"required,description=起始章节号" validate:"required,min=1"`
	EndChapter   int    `json:"end_chapter" jsonschema:"required,description=结束章节号" validate:"required,min=1"`
	DetailJSON   string `json:"detail_json" jsonschema:"description=结构化卷纲。每行一个要素，格式：> 要素名：内容。例：> 核心事件：宗门大比夺魁\\n> 主角变化：废柴→筑基→金丹\\n> 收尾钩子：反派第一层身份揭晓\\n> 爽点分布：Ch.5 碾压守卫 / Ch.12 身份曝光"`
}

type CreateVolumeTool struct{}

func (t *CreateVolumeTool) Name() string { return "create_volume" }
func (t *CreateVolumeTool) Description() string {
	return "创建一卷。开书阶段规划卷结构时使用，必须填写起始/结束章节号。" +
		"detail_json 用 > 行格式写结构化卷纲（核心事件/主角变化/收尾钩子/爽点分布）。"
}
func (t *CreateVolumeTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *CreateVolumeTool) JSONSchema() json.RawMessage {
	return SchemaOf(CreateVolumeArgs{})
}
func (t *CreateVolumeTool) ExposeToLLM() bool { return true }
func (t *CreateVolumeTool) NewArgs() any      { return &CreateVolumeArgs{} }

func (t *CreateVolumeTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*CreateVolumeArgs)
	if a.EndChapter < a.StartChapter {
		return nil, fmt.Errorf("结束章节号不能小于起始章节号")
	}

	// 计算 sort_order（当前最大值+1）
	store := volume.NewStore(tc.DB)
	existing, _ := store.ListByNovel(ctx, tc.NovelID)
	sortOrder := len(existing) + 1

	v := volume.Volume{
		NovelID:      tc.NovelID,
		Name:         a.Name,
		Description:  a.Description,
		StartChapter: a.StartChapter,
		EndChapter:   a.EndChapter,
		DetailJSON:   a.DetailJSON,
		SortOrder:    sortOrder,
	}
	if err := store.Create(ctx, &v); err != nil {
		return nil, fmt.Errorf("create volume: %w", err)
	}

	slog.Default().Info("卷已创建", "novel_id", tc.NovelID, "name", a.Name)
	return &ToolResult{Success: true, Data: map[string]any{"id": v.ID}}, nil
}

// ── update_volume ──────────────────────────────────────────

type UpdateVolumeArgs struct {
	ID           int64  `json:"id" jsonschema:"required,description=卷ID" validate:"required,min=1"`
	Name         string `json:"name" jsonschema:"description=卷名"`
	Description  string `json:"description" jsonschema:"description=卷纲概述（一句话）"`
	StartChapter int    `json:"start_chapter" jsonschema:"description=起始章节号"`
	EndChapter   int    `json:"end_chapter" jsonschema:"description=结束章节号"`
	DetailJSON   string `json:"detail_json" jsonschema:"description=结构化卷纲。每行一个要素，格式：> 要素名：内容"`
	SortOrder    int    `json:"sort_order" jsonschema:"description=排序"`
}

type UpdateVolumeTool struct{}

func (t *UpdateVolumeTool) Name() string { return "update_volume" }
func (t *UpdateVolumeTool) Description() string {
	return "更新一卷的属性。只更新传入的非空字段。" +
		"detail_json 用 > 行格式写结构化卷纲。"
}
func (t *UpdateVolumeTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *UpdateVolumeTool) JSONSchema() json.RawMessage {
	return SchemaOf(UpdateVolumeArgs{})
}
func (t *UpdateVolumeTool) ExposeToLLM() bool { return true }
func (t *UpdateVolumeTool) NewArgs() any      { return &UpdateVolumeArgs{} }

func (t *UpdateVolumeTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpdateVolumeArgs)
	store := volume.NewStore(tc.DB)
	v, err := store.GetByID(ctx, a.ID)
	if err != nil {
		return nil, fmt.Errorf("volume not found: %d", a.ID)
	}

	if a.Name != "" {
		v.Name = a.Name
	}
	if a.Description != "" {
		v.Description = a.Description
	}
	if a.StartChapter > 0 {
		v.StartChapter = a.StartChapter
	}
	if a.EndChapter > 0 {
		v.EndChapter = a.EndChapter
	}
	if a.DetailJSON != "" {
		v.DetailJSON = a.DetailJSON
	}
	if a.SortOrder > 0 {
		v.SortOrder = a.SortOrder
	}

	if err := store.Update(ctx, v); err != nil {
		return nil, fmt.Errorf("update volume: %w", err)
	}

	return &ToolResult{Success: true, Data: map[string]any{"id": v.ID}}, nil
}

// ── delete_volume ──────────────────────────────────────────

type DeleteVolumeArgs struct {
	ID int64 `json:"id" jsonschema:"required,description=卷ID" validate:"required,min=1"`
}

type DeleteVolumeTool struct{}

func (t *DeleteVolumeTool) Name() string { return "delete_volume" }
func (t *DeleteVolumeTool) Description() string {
	return "删除一卷。"
}
func (t *DeleteVolumeTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *DeleteVolumeTool) JSONSchema() json.RawMessage {
	return SchemaOf(DeleteVolumeArgs{})
}
func (t *DeleteVolumeTool) ExposeToLLM() bool { return true }
func (t *DeleteVolumeTool) NewArgs() any      { return &DeleteVolumeArgs{} }

func (t *DeleteVolumeTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*DeleteVolumeArgs)
	store := volume.NewStore(tc.DB)

	// 查询卷信息（用于审批和级联）
	v, err := store.GetByID(ctx, a.ID)
	if err != nil {
		return nil, fmt.Errorf("volume not found: %d", a.ID)
	}

	// 审批
	meta := map[string]any{"id": v.ID, "name": v.Name, "start_chapter": v.StartChapter, "end_chapter": v.EndChapter, "type": "volume"}
	injects, result, err := requestDeleteApproval(ctx, tc, map[string]any{
		"table": "volume", "id": a.ID, "deleted": meta,
	})
	if err != nil || result != nil {
		return result, err
	}

	// 提醒：卷删除后，原卷范围内章节仍保留（chapter 表无 volume_id 字段，章节通过 chapter_number 隐式关联卷）。
	// 章节不会被删除，但卷纲引用的 start_chapter/end_chapter 范围内章节将成为孤儿。
	slog.Info("volume deleted", "volume_id", v.ID, "range", fmt.Sprintf("%d-%d", v.StartChapter, v.EndChapter))

	if err := store.Delete(ctx, a.ID); err != nil {
		return nil, fmt.Errorf("delete volume: %w", err)
	}
	return &ToolResult{Success: true, Data: map[string]any{"ok": true}, Inject: injects}, nil
}

// ── 注册 ──────────────────────────────────────────────────

func RegisterVolumeTools(r *Registry) {
	r.Register(&GetVolumesTool{})
	r.Register(&CreateVolumeTool{})
	r.Register(&UpdateVolumeTool{})
	r.Register(&DeleteVolumeTool{})
}
