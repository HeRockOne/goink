package mcp_tools

import (
	"context"
	"encoding/json"
)

// ── set_phase 工具 ─────────────────────────────────────────

// SetPhaseTool 切换当前创作阶段。
// 阶段门禁由 always-mode skill 中的 <!-- phase-gate-config --> 配置定义。
// 如果没有门禁配置，此工具无操作。
type SetPhaseTool struct{}

func (t *SetPhaseTool) Name() string        { return "set_phase" }
func (t *SetPhaseTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *SetPhaseTool) ExposeToLLM() bool    { return true }

func (t *SetPhaseTool) Description() string {
	return `切换当前创作阶段。

门禁始终激活，当前阶段有 require 列表，必须调用过列表中的所有工具后才能切换。

参数：
- phase: 目标阶段名称（如 "prepare", "outline", "write", "review", "maintain"）

切换成功：返回 success=true 和当前阶段状态。
require 未满足：返回 success=false，提示缺少哪些工具调用。
未知阶段名：返回 success=false。

如果没有门禁配置，此工具无操作。`
}

func (t *SetPhaseTool) NewArgs() any {
	return &SetPhaseArgs{}
}

func (t *SetPhaseTool) JSONSchema() json.RawMessage {
	return SchemaOf(SetPhaseArgs{})
}

func (t *SetPhaseTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	// set_phase 的实际逻辑在 agent loop 中处理（agent/agent.go 的 set_phase 特殊分支）
	// 这里只是为了让 LLM 看到这个工具并能调用它
	// 实际执行在 agent loop 中拦截，不会走到这里
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"message": "set_phase 已在 agent loop 中处理",
		},
	}, nil
}

// SetPhaseArgs set_phase 工具参数。
type SetPhaseArgs struct {
	Phase string `json:"phase" validate:"required" jsonschema:"description=目标阶段名称"`
}

// RegisterPhaseGateTools 注册阶段门禁工具。
func RegisterPhaseGateTools(r *Registry) {
	r.Register(&SetPhaseTool{})
}
