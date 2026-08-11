package mcp_tools

import (
	"log/slog"
	"strings"
	"testing"

	"novel/internal/skill"
)

// TestBuildSkillsReminder 验证阶段技能短提醒生成：
// 含技能名 + description 要点，远小于全文，可用于唤起注意。
func TestBuildSkillsReminder(t *testing.T) {
	store, err := skill.NewStore(slog.Default(), "")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	reminder, err := BuildSkillsReminder(store, 0, []string{"main-tech-show-dont-tell", "main-tech-anti-ai-writing"})
	if err != nil {
		t.Fatalf("BuildSkillsReminder: %v", err)
	}
	if !strings.HasPrefix(reminder, "【当前阶段技能提醒】") {
		t.Errorf("提醒缺少前缀标记: %q", reminder)
	}
	if !strings.Contains(reminder, "main-tech-show-dont-tell") || !strings.Contains(reminder, "main-tech-anti-ai-writing") {
		t.Errorf("提醒缺少技能名: %q", reminder)
	}
	if !strings.Contains(reminder, "请按以上技能要点创作") {
		t.Errorf("提醒缺少收尾指令: %q", reminder)
	}
	// 提醒应远小于全文（全文几 KB，提醒几百 token 内）
	full, err := BuildSkillsContent(store, 0, []string{"main-tech-show-dont-tell", "main-tech-anti-ai-writing"})
	if err != nil {
		t.Fatalf("BuildSkillsContent: %v", err)
	}
	if len(reminder) >= len(full) {
		t.Errorf("提醒应远小于全文: reminder=%d full=%d", len(reminder), len(full))
	}
	t.Logf("提醒 %d 字符 / 全文 %d 字符", len(reminder), len(full))
	t.Logf("提醒内容: %s", reminder)
}

// TestBuildSkillsReminderEmptyStore 空 store / 空列表返回空提醒。
func TestBuildSkillsReminderEmpty(t *testing.T) {
	r, err := BuildSkillsReminder(nil, 0, nil)
	if err != nil {
		t.Fatalf("nil store: %v", err)
	}
	if r != "" {
		t.Errorf("nil store 应返回空: %q", r)
	}
	store, err := skill.NewStore(slog.Default(), "")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	r, err = BuildSkillsReminder(store, 0, []string{"main-tech-does-not-exist"})
	if err != nil {
		t.Fatalf("不存在技能: %v", err)
	}
	if r != "" {
		t.Errorf("全部技能不存在应返回空: %q", r)
	}
}
