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

本工具是门禁必读技能的加载入口。各阶段必读技能（require_reads）已由系统在 set_phase 时自动注入为 system 消息，正常情况下无需手动调用本工具。仅当技能内容被滚动压缩出上下文、或需要手动刷新时使用。

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
