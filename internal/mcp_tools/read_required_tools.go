package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ── read_required ─────────────────────────────────────────

// ReadRequiredArgs 是 read_required 的参数。
// skills 为逗号分隔的技能名列表（不含 .md 后缀）。
type ReadRequiredArgs struct {
	Skills string `json:"skills" jsonschema:"required,description=逗号分隔的技能名列表，如 main-tech-anti-ai-writing,main-tech-show-dont-tell。门禁 require 阶段必须调用本工具并传入该阶段要求的技能" validate:"required"`
}

// ReadRequiredTool 读取指定技能完整内容。
// 与 read 的区别：专供门禁 require 使用——技能名白名单由调用方（阶段配置）决定，
// 工具本身只做路径校验，读不到即失败（require 只统计成功调用）。
type ReadRequiredTool struct{}

func (t *ReadRequiredTool) Name() string           { return "read_required" }
func (t *ReadRequiredTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *ReadRequiredTool) ExposeToLLM() bool      { return true }

func (t *ReadRequiredTool) Description() string {
	return `读取指定技能的完整内容，返回给当前上下文。

与 read 的区别：本工具是门禁强制的技能加载入口——门禁配置的 require_reads 决定每个阶段必须读哪些技能，本工具只按参数读取，不内置任何技能清单。

用法：进入新阶段后，先调用 get_phase_gate_config 查看当前阶段的 require_reads 必读技能，然后用本工具传入这些技能名；否则 set_phase 会被门禁拦截。

注意：必读技能必须在创作动作（edit/update/create/run_subagent）**之前**加载——技能是创作指导，不是切换阶段的手续。未加载必读技能就执行创作动作会被门禁直接拦截，提示先读技能。若技能内容已被滚动压缩出上下文，必须重新调用本工具加载，不要赌记忆。

参数：
- skills: 逗号分隔的技能名列表（不含 .md），如 "main-tech-anti-ai-writing,main-tech-show-dont-tell"

返回：每个技能的完整 Markdown 内容。技能不存在时返回失败。`
}

func (t *ReadRequiredTool) JSONSchema() json.RawMessage { return SchemaOf(ReadRequiredArgs{}) }
func (t *ReadRequiredTool) NewArgs() any                { return &ReadRequiredArgs{} }

func (t *ReadRequiredTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*ReadRequiredArgs)

	if tc.SkillStore == nil {
		return &ToolResult{Success: false, Error: "skill store 未初始化"}, nil
	}

	names := []string{}
	for _, raw := range strings.Split(a.Skills, ",") {
		name := strings.TrimSpace(raw)
		name = strings.TrimSuffix(name, ".md")
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return &ToolResult{Success: false, Error: "未指定技能名"}, nil
	}

	var b strings.Builder
	for _, name := range names {
		sk, ok := tc.SkillStore.Get(tc.NovelID, name)
		if !ok {
			return &ToolResult{Success: false, Error: fmt.Sprintf("技能 %q 不存在", name)}, nil
		}
		b.WriteString("--- ")
		b.WriteString(sk.Name)
		b.WriteString(" ---\n")
		b.WriteString(sk.RawContent)
		b.WriteString("\n\n")
	}

	return &ToolResult{Success: true, Data: map[string]any{
		"skills":  names,
		"display": fmt.Sprintf("加载技能: %s", strings.Join(names, ", ")),
		"content": strings.TrimSpace(b.String()),
	}}, nil
}

// RegisterReadRequiredTool 注册 read_required 工具。
func RegisterReadRequiredTool(r *Registry) {
	r.Register(&ReadRequiredTool{})
}
