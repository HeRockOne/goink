package mcp_tools

import (
	"context"
	"encoding/json"
	"log/slog"

	"novel/internal/stats"
)

type GetStatsArgs struct {
	IncludeCharacters bool `json:"include_characters" jsonschema:"description=是否包含角色出场统计，默认false"`
	IncludeLocations  bool `json:"include_locations" jsonschema:"description=是否包含地点使用统计，默认false"`
}

type GetStatsTool struct{}

func (t *GetStatsTool) Name() string { return "get_stats" }
func (t *GetStatsTool) Description() string {
	return "获取当前小说的综合统计信息：总章数、总字数、平均每章字数、最新章节、弧线进度、伏笔回收率、角色/地点数量。可选包含角色出场和地点使用频率统计。"
}
func (t *GetStatsTool) Category() ToolCategory { return CategoryNovelManagement }
func (t *GetStatsTool) JSONSchema() json.RawMessage { return SchemaOf(GetStatsArgs{}) }
func (t *GetStatsTool) ExposeToLLM() bool           { return true }
func (t *GetStatsTool) NewArgs() any                { return &GetStatsArgs{} }
func (t *GetStatsTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	statsData, err := stats.NewStore(tc.DB, slog.Default()).GetNovelStats(ctx, tc.NovelID)
	if err != nil {
		return &ToolResult{Success: false, Error: "获取统计失败: " + err.Error()}, nil
	}
	return &ToolResult{Success: true, Data: map[string]any{"stats": statsData}}, nil
}

func RegisterStatsTools(r *Registry) {
	r.Register(&GetStatsTool{})
}
