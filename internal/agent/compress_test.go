package agent

import (
	"context"
	"log/slog"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"novel/internal/agentcfg"
	"novel/internal/session"
)

func TestRetainMessages_ExcludesNovelState(t *testing.T) {
	messages := []map[string]any{
		{"role": "system", "content": "identity"},
		{"role": "user", "content": "第一轮"},
		{"role": "system", "content": agentcfg.NovelStatePrefix + "书名：测试"},
		{"role": "assistant", "content": "回复1"},
		{"role": "user", "content": "第二轮"},
		{"role": "system", "content": agentcfg.NovelStatePrefix + "书名：测试2"},
		{"role": "assistant", "content": "回复2"},
	}

	retained := retainMessages(messages)
	for _, m := range retained {
		if role, _ := m["role"].(string); role == "system" {
			if content, ok := m["content"].(string); ok && content != "identity" {
				t.Errorf("system message should be excluded from retained, got: %q", content)
			}
		}
	}
	if len(retained) != 4 {
		t.Errorf("expected 4 retained messages (identity not included, NS excluded), got %d", len(retained))
	}
}

func TestPersistCompression_NovelStateAtEnd(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&session.Session{}, &session.Message{}); err != nil {
		t.Fatal(err)
	}
	db.Create(&session.Session{SessionID: "s1", NovelID: 1, ActiveVersion: 1})

	logger := slog.New(slog.DiscardHandler)
	a := &Agent{db: db, logger: logger}
	opts := &RunOptions{SessionID: "s1", TurnID: 1}

	novelState := agentcfg.NovelStatePrefix + "书名：测试"
	retained := []map[string]any{
		{"role": "user", "content": "历史消息"},
	}
	// 压缩后最新 NS 作为消息落库到新版本末尾（缓存协议：NS 进历史、永不清理）
	_, err = a.persistCompression(context.Background(), opts,
		"identity-content", "", "", novelState, "摘要内容", retained)
	if err != nil {
		t.Fatal(err)
	}

	var msgs []session.Message
	if err := db.Where("session_id = ? AND version = ?", "s1", 2).
		Order("id ASC").Find(&msgs).Error; err != nil {
		t.Fatal(err)
	}

	// 期望顺序: [identity][reminder][summary][retained(user)][NS][marker]
	if len(msgs) != 6 {
		t.Fatalf("expected 6 messages, got %d", len(msgs))
	}
	ns := msgs[len(msgs)-2]
	if ns.Role != "system" || ns.Content != novelState {
		t.Errorf("NS snapshot should be second-to-last, got role=%s content=%q", ns.Role, ns.Content)
	}
	if ns.ToFrontend {
		t.Error("NS snapshot should not be visible to frontend")
	}
	if !ns.ToAPI {
		t.Error("NS snapshot should be sent to API")
	}
	if msgs[0].Role != "system" || msgs[0].Content != "identity-content" {
		t.Errorf("first message should be identity, got role=%s", msgs[0].Role)
	}
	for i, m := range msgs {
		if i == len(msgs)-2 {
			continue
		}
		if m.Content == novelState {
			t.Errorf("duplicate NS in system area at index %d", i)
		}
	}
}
