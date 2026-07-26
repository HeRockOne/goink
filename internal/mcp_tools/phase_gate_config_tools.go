package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"

	"novel/internal/config"
)

// ── get_phase_gate_config ───────────────────────────────

type GetPhaseGateConfigTool struct{}

func (t *GetPhaseGateConfigTool) Name() string { return "get_phase_gate_config" }
func (t *GetPhaseGateConfigTool) Description() string {
	return "读取当前阶段门禁配置。返回完整的 <!-- phase-gate-config --> 配置内容和当前阶段状态。" +
		"如果未配置门禁，返回空。需要查看门禁规则时使用此工具，不要猜测。"
}
func (t *GetPhaseGateConfigTool) Category() ToolCategory { return CategoryMemoryRetrieval }
func (t *GetPhaseGateConfigTool) JSONSchema() json.RawMessage { return SchemaOf(struct{}{}) }
func (t *GetPhaseGateConfigTool) ExposeToLLM() bool           { return true }
func (t *GetPhaseGateConfigTool) NewArgs() any                { return &struct{}{} }

func (t *GetPhaseGateConfigTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	s, err := config.LoadSettings(tc.DB)
	if err != nil {
		return nil, fmt.Errorf("load settings: %w", err)
	}
	return &ToolResult{Success: true, Data: map[string]any{
		"config":        s.PhaseGateConfig,
		"has_config":    s.PhaseGateConfig != "",
		"gate_enabled":  s.PhaseGateEnabled != nil && *s.PhaseGateEnabled,
	}}, nil
}

// ── update_phase_gate_config ────────────────────────────

type UpdatePhaseGateConfigArgs struct {
	Config string `json:"config" jsonschema:"required,description=完整的门禁配置，包含 <!-- phase-gate-config --> 注释块。传递空字符串可清空配置"`
}

type UpdatePhaseGateConfigTool struct{}

func (t *UpdatePhaseGateConfigTool) Name() string { return "update_phase_gate_config" }
func (t *UpdatePhaseGateConfigTool) Description() string {
	return "更新阶段门禁配置。此配置存储在数据库中，仅门禁代码读取，不占用 AI 上下文 token。" +
		"格式为 <!-- phase-gate-config --> 注释块，包含 tools/require/edit_paths/next 等字段。" +
		"传入完整的配置内容（含所有阶段的 config 块）。传入空字符串清空配置。" +
		"修改前建议先调用 get_phase_gate_config 查看当前配置。"
}
func (t *UpdatePhaseGateConfigTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *UpdatePhaseGateConfigTool) JSONSchema() json.RawMessage {
	return SchemaOf(UpdatePhaseGateConfigArgs{})
}
func (t *UpdatePhaseGateConfigTool) ExposeToLLM() bool { return true }
func (t *UpdatePhaseGateConfigTool) NewArgs() any      { return &UpdatePhaseGateConfigArgs{} }

func (t *UpdatePhaseGateConfigTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpdatePhaseGateConfigArgs)
	s, err := config.LoadSettings(tc.DB)
	if err != nil {
		return nil, fmt.Errorf("load settings: %w", err)
	}
	s.PhaseGateConfig = a.Config
	if err := config.SaveSettings(tc.DB, s); err != nil {
		return nil, fmt.Errorf("save settings: %w", err)
	}
	return &ToolResult{Success: true, Data: map[string]any{"saved": true}}, nil
}

// ── 注册 ──────────────────────────────────────────────

func RegisterPhaseGateConfigTool(r *Registry) {
	r.Register(&GetPhaseGateConfigTool{})
	r.Register(&UpdatePhaseGateConfigTool{})
}
