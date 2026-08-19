package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"novel/internal/outline"
)

// ── get_outline ────────────────────────────────────────────

type GetOutlineArgs struct{}

type GetOutlineTool struct{}

func (t *GetOutlineTool) Name() string { return "get_outline" }
func (t *GetOutlineTool) Description() string {
	return "获取当前小说的全书总纲和大爽点列表。返回核心矛盾、成长弧线、结局方向、篇幅规划，以及大爽点数组（章号+描述）。" +
		"开书阶段写入后基本不再修改。"
}
func (t *GetOutlineTool) Category() ToolCategory { return CategoryConsistencyCheck }
func (t *GetOutlineTool) JSONSchema() json.RawMessage {
	return SchemaOf(GetOutlineArgs{})
}
func (t *GetOutlineTool) ExposeToLLM() bool { return true }
func (t *GetOutlineTool) NewArgs() any      { return &GetOutlineArgs{} }

func (t *GetOutlineTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	store := outline.NewStore(tc.DB)
	o, err := store.GetByNovelID(ctx, tc.NovelID)
	if err != nil {
		return &ToolResult{Success: true, Data: map[string]any{"content": "暂无总纲数据"}}, nil
	}

	beats, err := store.ListBeats(ctx, tc.NovelID)
	if err != nil {
		return nil, fmt.Errorf("list beats: %w", err)
	}

	beatList := make([]map[string]any, 0, len(beats))
	for _, b := range beats {
		beatList = append(beatList, map[string]any{
			"chapter":     b.Chapter,
			"description": b.Description,
			"beat_type":   b.BeatType,
			"importance":  b.Importance,
		})
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"core_conflict":    o.CoreConflict,
			"growth_arc":       o.GrowthArc,
			"ending_direction": o.EndingDirection,
			"theme":            o.Theme,
			"word_count_plan":  o.WordCountPlan,
			"beats":            beatList,
		},
	}, nil
}

// ── update_outline ─────────────────────────────────────────

type UpdateOutlineArgs struct {
	CoreConflict    string `json:"core_conflict" jsonschema:"description=核心矛盾。每行一个要素，格式：> 要素名：内容。例：> 主角：林逸\\n> 反派：陈默\\n> 根本冲突：被至交背叛夺走一切\\n> 赌注：六界存亡"`
	GrowthArc       string `json:"growth_arc" jsonschema:"description=成长弧线。每行一个阶段，格式：> 第起始-结束章 阶段名：阶段描述。例：> 第1-8章 废柴期：被欺压的普通弟子，无特殊能力"`
	EndingDirection string `json:"ending_direction" jsonschema:"description=结局方向。每行一个要素，格式：> 要素名：内容。例：> 类型：逆袭碾压\\n> 基调：从最低谷到碾压巅峰\\n> 收束：清算所有仇敌"`
	Theme           string `json:"theme" jsonschema:"description=主题立意。这本书想表达什么。例：> 核心主题：逆天改命\\n> 深层追问：人的价值由谁定义"`
	WordCountPlan   int    `json:"word_count_plan" jsonschema:"description=篇幅规划（万字）"`
}

type UpdateOutlineTool struct{}

func (t *UpdateOutlineTool) Name() string { return "update_outline" }
func (t *UpdateOutlineTool) Description() string {
	return "更新全书总纲。开书阶段写入，后续基本不再修改。" +
		"参数可选，只更新传入的字段（不传则不更新）。" +
		"总纲是 init_consistency 检查的数据源，确保核心矛盾/成长弧线/结局方向/篇幅规划完整。"
}
func (t *UpdateOutlineTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *UpdateOutlineTool) JSONSchema() json.RawMessage {
	return SchemaOf(UpdateOutlineArgs{})
}
func (t *UpdateOutlineTool) ExposeToLLM() bool { return true }
func (t *UpdateOutlineTool) NewArgs() any      { return &UpdateOutlineArgs{} }

func (t *UpdateOutlineTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpdateOutlineArgs)
	store := outline.NewStore(tc.DB)

	o, err := store.GetByNovelID(ctx, tc.NovelID)
	if err != nil {
		// 不存在则创建
		o = &outline.Outline{NovelID: tc.NovelID}
	}

	if a.CoreConflict != "" {
		o.CoreConflict = a.CoreConflict
	}
	if a.GrowthArc != "" {
		o.GrowthArc = a.GrowthArc
	}
	if a.EndingDirection != "" {
		o.EndingDirection = a.EndingDirection
	}
	if a.Theme != "" {
		o.Theme = a.Theme
	}
	if a.WordCountPlan > 0 {
		o.WordCountPlan = a.WordCountPlan
	}

	if err := store.Upsert(ctx, o); err != nil {
		return nil, fmt.Errorf("upsert outline: %w", err)
	}

	slog.Default().Info("总纲已更新", "novel_id", tc.NovelID)
	return &ToolResult{Success: true, Data: map[string]any{"id": o.ID}}, nil
}

// ── create_outline_beat ────────────────────────────────────

type CreateOutlineBeatArgs struct {
	Chapter     int    `json:"chapter" jsonschema:"required,description=承诺章号" validate:"required,min=1"`
	Description string `json:"description" jsonschema:"required,description=大爽点描述（如'碾压守卫展示实力'）" validate:"required"`
	BeatType    string `json:"beat_type" jsonschema:"description=类型：shuangdian(大爽点)/turning(转折)/climax(高潮)，默认shuangdian"`
	Importance  int    `json:"importance" jsonschema:"description=重要度1-5，默认3"`
}

type CreateOutlineBeatTool struct{}

func (t *CreateOutlineBeatTool) Name() string { return "create_outline_beat" }
func (t *CreateOutlineBeatTool) Description() string {
	return "创建大爽点/关键节点。开书阶段写入，每个大爽点标注承诺章号和描述。" +
		"init_consistency 会检查这些承诺是否兑现（promise_fulfillment）。"
}
func (t *CreateOutlineBeatTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *CreateOutlineBeatTool) JSONSchema() json.RawMessage {
	return SchemaOf(CreateOutlineBeatArgs{})
}
func (t *CreateOutlineBeatTool) ExposeToLLM() bool { return true }
func (t *CreateOutlineBeatTool) NewArgs() any      { return &CreateOutlineBeatArgs{} }

func (t *CreateOutlineBeatTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*CreateOutlineBeatArgs)
	store := outline.NewStore(tc.DB)

	beatType := a.BeatType
	if beatType == "" {
		beatType = "shuangdian"
	}
	importance := a.Importance
	if importance <= 0 {
		importance = 3
	}

	b := &outline.OutlineBeat{
		NovelID:     tc.NovelID,
		Chapter:     a.Chapter,
		Description: a.Description,
		BeatType:    beatType,
		Importance:  importance,
	}

	if err := store.CreateBeat(ctx, b); err != nil {
		return nil, fmt.Errorf("create beat: %w", err)
	}

	slog.Default().Info("大爽点已创建", "novel_id", tc.NovelID, "chapter", a.Chapter)
	return &ToolResult{Success: true, Data: map[string]any{"id": b.ID}}, nil
}

// ── update_outline_beat ────────────────────────────────────

type UpdateOutlineBeatArgs struct {
	ID          int64  `json:"id" jsonschema:"required,description=大爽点ID" validate:"required,min=1"`
	Chapter     int    `json:"chapter" jsonschema:"description=承诺章号"`
	Description string `json:"description" jsonschema:"description=大爽点描述"`
	BeatType    string `json:"beat_type" jsonschema:"description=类型：shuangdian/turning/climax"`
	Importance  int    `json:"importance" jsonschema:"description=重要度1-5"`
}

type UpdateOutlineBeatTool struct{}

func (t *UpdateOutlineBeatTool) Name() string { return "update_outline_beat" }
func (t *UpdateOutlineBeatTool) Description() string {
	return "更新大爽点/关键节点。只更新传入的字段。"
}
func (t *UpdateOutlineBeatTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *UpdateOutlineBeatTool) JSONSchema() json.RawMessage {
	return SchemaOf(UpdateOutlineBeatArgs{})
}
func (t *UpdateOutlineBeatTool) ExposeToLLM() bool { return true }
func (t *UpdateOutlineBeatTool) NewArgs() any      { return &UpdateOutlineBeatArgs{} }

func (t *UpdateOutlineBeatTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpdateOutlineBeatArgs)
	store := outline.NewStore(tc.DB)

	beats, err := store.ListBeats(ctx, tc.NovelID)
	if err != nil {
		return nil, fmt.Errorf("list beats: %w", err)
	}

	var beat *outline.OutlineBeat
	for i := range beats {
		if beats[i].ID == a.ID {
			beat = &beats[i]
			break
		}
	}
	if beat == nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("大爽点 ID %d 不存在", a.ID)}, nil
	}

	if a.Chapter > 0 {
		beat.Chapter = a.Chapter
	}
	if a.Description != "" {
		beat.Description = a.Description
	}
	if a.BeatType != "" {
		beat.BeatType = a.BeatType
	}
	if a.Importance > 0 {
		beat.Importance = a.Importance
	}

	if err := store.UpdateBeat(ctx, beat); err != nil {
		return nil, fmt.Errorf("update beat: %w", err)
	}

	return &ToolResult{Success: true, Data: map[string]any{"id": beat.ID}}, nil
}

// ── delete_outline_beat ────────────────────────────────────

type DeleteOutlineBeatArgs struct {
	ID int64 `json:"id" jsonschema:"required,description=大爽点ID" validate:"required,min=1"`
}

type DeleteOutlineBeatTool struct{}

func (t *DeleteOutlineBeatTool) Name() string { return "delete_outline_beat" }
func (t *DeleteOutlineBeatTool) Description() string {
	return "删除大爽点/关键节点。"
}
func (t *DeleteOutlineBeatTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *DeleteOutlineBeatTool) JSONSchema() json.RawMessage {
	return SchemaOf(DeleteOutlineBeatArgs{})
}
func (t *DeleteOutlineBeatTool) ExposeToLLM() bool { return true }
func (t *DeleteOutlineBeatTool) NewArgs() any      { return &DeleteOutlineBeatArgs{} }

func (t *DeleteOutlineBeatTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*DeleteOutlineBeatArgs)
	store := outline.NewStore(tc.DB)

	if err := store.DeleteBeat(ctx, a.ID); err != nil {
		return nil, fmt.Errorf("delete beat: %w", err)
	}

	return &ToolResult{Success: true, Data: map[string]any{"deleted": a.ID}}, nil
}

// ── 注册 ──────────────────────────────────────────────────

func RegisterOutlineTools(r *Registry) {
	r.Register(&GetOutlineTool{})
	r.Register(&UpdateOutlineTool{})
	r.Register(&CreateOutlineBeatTool{})
	r.Register(&UpdateOutlineBeatTool{})
	r.Register(&DeleteOutlineBeatTool{})
}
