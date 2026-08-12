package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"novel/internal/skill"
)

// ── auto_skill_injection ─────────────────────────────────

// AutoSkillInjectionArgs 是 auto_skill_injection 的参数。
// skills 为逗号分隔的技能名列表（不含 .md 后缀）。
type AutoSkillInjectionArgs struct {
	Skills string `json:"skills" jsonschema:"required,description=逗号分隔的技能名列表，如 main-tech-anti-ai-writing,main-tech-show-dont-tell。门禁 auto_skill_injection 阶段系统会在 set_phase 时自动注入，本工具仅需手动刷新时调用" validate:"required"`
}

// AutoSkillInjectionTool 读取指定技能完整内容（手动刷新入口）。
// 各阶段必读技能已由系统在 set_phase 时自动注入，正常情况下无需手动调用。
type AutoSkillInjectionTool struct{}

func (t *AutoSkillInjectionTool) Name() string           { return "auto_skill_injection" }
func (t *AutoSkillInjectionTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *AutoSkillInjectionTool) ExposeToLLM() bool      { return true }

func (t *AutoSkillInjectionTool) Description() string {
	return `读取指定技能的完整内容，返回给当前上下文。

各阶段必读技能（auto_skill_injection）已由系统在 set_phase 时自动注入，正常情况下无需手动调用。仅当技能内容被滚动压缩出上下文、或需要手动刷新时使用。

参数：
- skills: 逗号分隔的技能名列表（不含 .md），如 "main-tech-anti-ai-writing,main-tech-show-dont-tell"

返回：每个技能的完整 Markdown 内容。技能不存在时返回失败。

【注意】技能是创作指导（世界观分类/伏笔节奏/悬念设计等方法论），压缩后必须补读再动笔——不要因为"读过一次"就跳过补读；门禁会在创作动作前强制检查。`
}

func (t *AutoSkillInjectionTool) JSONSchema() json.RawMessage { return SchemaOf(AutoSkillInjectionArgs{}) }
func (t *AutoSkillInjectionTool) NewArgs() any                { return &AutoSkillInjectionArgs{} }

func (t *AutoSkillInjectionTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*AutoSkillInjectionArgs)
	if tc.SkillStore == nil {
		return &ToolResult{Success: false, Error: "skill store 未初始化"}, nil
	}
	names := parseSkillNames(a.Skills)
	if len(names) == 0 {
		return &ToolResult{Success: false, Error: "未指定技能名"}, nil
	}
	content, err := BuildSkillsContent(tc.SkillStore, tc.NovelID, names)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}
	return &ToolResult{Success: true, Data: map[string]any{
		"skills":  names,
		"display": fmt.Sprintf("加载技能: %s", strings.Join(names, ", ")),
		"content": content,
	}}, nil
}

// RegisterAutoSkillInjectionTool 注册 auto_skill_injection 工具。
func RegisterAutoSkillInjectionTool(r *Registry) {
	r.Register(&AutoSkillInjectionTool{})
}

// ── 共享函数（auto-inject 和 auto_skill_injection 工具共用） ──

// parseSkillNames 解析逗号分隔的技能名，去空格、去 .md 后缀。
func parseSkillNames(raw string) []string {
	var names []string
	for _, s := range strings.Split(raw, ",") {
		name := strings.TrimSpace(s)
		name = strings.TrimSuffix(name, ".md")
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

// BuildSkillsContent 从 skill store 读取技能内容并拼接成全文。
// 被 auto-inject（agent.injectPhaseSkills）和 auto_skill_injection 工具共享，
// 避免读取逻辑重复。
// 防御性设计（read_required 时代 buildRequiredSkillsContent 语义）：
// store 为 nil 或技能缺失时跳过该项而非整体失败——注入不能因单个技能
// 缺失而整个中断（否则 reads 不标记 → set_phase 失败/工具拦截连锁断点）。
func BuildSkillsContent(store *skill.Store, novelID int64, names []string) (string, error) {
	if store == nil {
		return "", nil
	}
	var b strings.Builder
	for _, name := range names {
		sk, ok := store.Get(novelID, name)
		if !ok {
			continue
		}
		b.WriteString("--- ")
		b.WriteString(sk.Name)
		b.WriteString(" ---\n")
		b.WriteString(sk.RawContent)
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String()), nil
}

// BuildSkillsReminder 生成阶段技能短提醒（技能名 + description 要点，~百 token）。
// 用于技能全文已在上下文（首次注入常驻历史）时，每次进入同阶段追加的"唤起注意"消息：
// 解决 Lost in the Middle——全文保证可见，短提醒紧跟请求尾部保证被注意。
// 与 BuildSkillsContent 共享技能读取逻辑；description 缺失时回退技能名。
func BuildSkillsReminder(store *skill.Store, novelID int64, names []string) (string, error) {
	if store == nil || len(names) == 0 {
		return "", nil
	}
	var b strings.Builder
	b.WriteString("【当前阶段技能提醒】")
	for i, name := range names {
		sk, ok := store.Get(novelID, name)
		if !ok {
			continue
		}
		if i > 0 {
			b.WriteString("；")
		}
		b.WriteString(sk.Name)
		if sk.Description != "" {
			b.WriteString("（")
			b.WriteString(sk.Description)
			b.WriteString("）")
		}
	}
	if b.Len() <= len("【当前阶段技能提醒】") {
		return "", nil
	}
	b.WriteString("。请按以上技能要点创作，勿偏离。")
	return b.String(), nil
}
