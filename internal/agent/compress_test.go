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
	// NS 不再作为消息落库（动态尾部注入，见缓存协议），persistCompression 不接收 novelState
	_, err = a.persistCompression(context.Background(), opts,
		"identity-content", "", "", "摘要内容", retained)
	if err != nil {
		t.Fatal(err)
	}

	var msgs []session.Message
	if err := db.Where("session_id = ? AND version = ?", "s1", 2).
		Order("id ASC").Find(&msgs).Error; err != nil {
		t.Fatal(err)
	}

	// 期望顺序: [identity][reminder][summary][retained(user)][marker]，无 NS 消息
	if len(msgs) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(msgs))
	}
	for _, m := range msgs {
		if m.Content == novelState {
			t.Error("NS snapshot must not be persisted as a message")
		}
	}
	if msgs[0].Role != "system" || msgs[0].Content != "identity-content" {
		t.Errorf("first message should be identity, got role=%s", msgs[0].Role)
	}
}
