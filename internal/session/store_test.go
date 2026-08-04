package session

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGetMessagesForAPI_OrderById(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Session{}, &Message{}); err != nil {
		t.Fatal(err)
	}
	db.Create(&Session{SessionID: "s1", NovelID: 1, ActiveVersion: 1})

	// 同一纳秒批量插入多条消息，created_at 完全相同，顺序只能靠 id
	ts := time.Now().Truncate(time.Nanosecond)
	msgs := []Message{
		{SessionID: "s1", TurnID: 1, Role: "system", Content: "A", Version: 1, ToAPI: true, CreatedAt: ts},
		{SessionID: "s1", TurnID: 1, Role: "user", Content: "B", Version: 1, ToAPI: true, CreatedAt: ts},
		{SessionID: "s1", TurnID: 1, Role: "system", Content: "NS", Version: 1, ToAPI: true, CreatedAt: ts},
		{SessionID: "s1", TurnID: 1, Role: "assistant", Content: "C", Version: 1, ToAPI: true, CreatedAt: ts},
	}
	if err := db.Create(&msgs).Error; err != nil {
		t.Fatal(err)
	}

	store := NewStore(db, slog.New(slog.DiscardHandler))
	got, err := store.GetMessagesForAPI(context.Background(), "s1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(got))
	}
	want := []string{"A", "B", "NS", "C"}
	for i, m := range got {
		if m.Content != want[i] {
			t.Errorf("position %d: expected %q, got %q (order must follow id ASC)", i, want[i], m.Content)
		}
	}
}
