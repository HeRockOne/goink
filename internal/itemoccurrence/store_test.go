package itemoccurrence

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"novel/internal/chapter"
)

func openOccDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&ItemOccurrence{}, &chapter.Chapter{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func occLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestListByItemRange(t *testing.T) {
	db := openOccDB(t)
	s := NewStore(db, occLogger())
	ctx := context.Background()

	// chapters 表（chapter_number 与 id 不同序，验证 join 用章号过滤）
	db.Create(&chapter.Chapter{NovelID: 1, ChapterNumber: 2, Title: "第2章"})
	db.Create(&chapter.Chapter{NovelID: 1, ChapterNumber: 8, Title: "第8章"})
	db.Create(&chapter.Chapter{NovelID: 1, ChapterNumber: 15, Title: "第15章"})
	db.Create(&ItemOccurrence{NovelID: 1, ItemID: 3, ChapterID: 1}) // 章号2
	db.Create(&ItemOccurrence{NovelID: 1, ItemID: 3, ChapterID: 2}) // 章号8
	db.Create(&ItemOccurrence{NovelID: 1, ItemID: 3, ChapterID: 3}) // 章号15
	db.Create(&ItemOccurrence{NovelID: 1, ItemID: 9, ChapterID: 2}) // 其他物品

	got, err := s.ListByItemRange(ctx, 1, 3, 5, 10)
	if err != nil {
		t.Fatalf("range query: %v", err)
	}
	if len(got) != 1 || got[0].ChapterID != 2 {
		t.Errorf("range 5-10: expected 1 (chapter 8), got %+v", got)
	}
	// 单边界
	got, _ = s.ListByItemRange(ctx, 1, 3, 15, 0)
	if len(got) != 1 || got[0].ChapterID != 3 {
		t.Errorf("from 15: expected 1 (chapter 15), got %+v", got)
	}
	// 无匹配
	got, _ = s.ListByItemRange(ctx, 1, 3, 20, 30)
	if len(got) != 0 {
		t.Errorf("range 20-30: expected 0, got %d", len(got))
	}
}
