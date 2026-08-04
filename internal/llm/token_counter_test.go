package llm

import "testing"

// 验证 CountSystemInjection：固定前缀的精确计数（与 tokencount 工具口径一致）。
func TestCountSystemInjection(t *testing.T) {
	identity := "你是小说创作系统的主创作助手。"
	always := "main-core-writing-kernel 创作调度：\n1. prepare → outline → write → review → maintain\n2. 每章至少一个情绪锚点"
	catalog := "技能目录：show_dont_tell；anti-ai-writing；chapter-hook-enhanced"
	tools := []map[string]any{
		{"type": "function", "function": map[string]any{
			"name":        "get_characters",
			"description": "获取角色列表",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
		}},
	}

	sys, toolsTokens := CountSystemInjection(identity, always, catalog, tools)
	if sys <= 0 {
		t.Fatalf("system tokens should be > 0, got %d", sys)
	}
	if toolsTokens <= 0 {
		t.Fatalf("tools tokens should be > 0, got %d", toolsTokens)
	}

	// 交叉验证：单条计数之和 = 分组计数
	n1, _ := CountTokens(identity)
	n2, _ := CountTokens(always)
	n3, _ := CountTokens(catalog)
	if sys != n1+n2+n3 {
		t.Errorf("system mismatch: %d vs %d", sys, n1+n2+n3)
	}
}

// 验证空输入不 panic。
func TestCountSystemInjection_Empty(t *testing.T) {
	sys, toolsTokens := CountSystemInjection("", "", "", nil)
	if sys != 0 || toolsTokens != 0 {
		t.Errorf("empty input should return 0, got sys=%d tools=%d", sys, toolsTokens)
	}
}
