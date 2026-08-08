package app

import (
	"strings"
	"testing"
)

// 旧配置（无迷你维护工具）升级后,batch write 阶段 tools 应包含迷你维护工具。
func TestUpgradeBatchWriteTools(t *testing.T) {
	oldCfg := `<!-- phase-gate-config
mode: single
phase: write
tools: read, edit
next: review
-->
<!-- phase-gate-config
mode: batch
phase: write
tools: read, read_required, edit, get_chapter_list
edit_paths: chapters/*
require: edit, get_chapter_list, read, read_required
next: review
loop: true
-->`

	upgraded := upgradeBatchWriteTools(oldCfg)
	if !strings.Contains(upgraded, "create_scene") {
		t.Fatal("batch write tools 应包含 create_scene")
	}
	if !strings.Contains(upgraded, "update_writing_snapshot") {
		t.Fatal("batch write tools 应包含 update_writing_snapshot")
	}
	// single 阶段不应被改
	if strings.Contains(upgraded, "tools: read, edit, create_scene") {
		t.Fatal("single 阶段不应被追加迷你维护工具")
	}
	// 幂等:再次升级无变化
	again := upgradeBatchWriteTools(upgraded)
	if again != upgraded {
		t.Fatal("升级应幂等")
	}
}

// 批量意图检测。
func TestIsBatchCreationIntent(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"批量写5章，先出大纲", true},
		{"连写3章", true},
		{"一口气写十章", true},
		{"批量创作多章", true},
		{"帮我写一章", false},
		{"看看设定", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isBatchCreationIntent(c.msg); got != c.want {
			t.Errorf("isBatchCreationIntent(%q) = %v, want %v", c.msg, got, c.want)
		}
	}
}
