package migrate

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"novel/internal/chapter"
	"novel/internal/character"
	"novel/internal/config"
	"novel/internal/item"
	"novel/internal/itemoccurrence"
	"novel/internal/location"
	"novel/internal/lore"
	"novel/internal/novel"
	"novel/internal/outline"
	"novel/internal/reader"
	"novel/internal/rollback"
	"novel/internal/scene"
	"novel/internal/session"
	"novel/internal/storage"
	"novel/internal/storyarc"
	"novel/internal/timeline"
	"novel/internal/volume"
	"novel/internal/style"
	"novel/internal/writing"
)

// Run 自动创建/更新全部数据表，幂等安全。
func Run(db *gorm.DB, log *slog.Logger) error {
// 移除旧 novels 表的 dir_path 列（该字段从未被读取过）。幂等：列不存在时报错忽略。
		if err := db.Exec("ALTER TABLE novels DROP COLUMN dir_path").Error; err != nil {
			log.Warn("迁移：删除 novels.dir_path 列失败（如列已不存在则无害）", "err", err)
		}

		// scenes.chapter_id 改为 nullable（支持规划场景）。GORM AutoMigrate 不处理 NOT NULL 约束变更。
		if err := db.Exec("ALTER TABLE scenes ALTER COLUMN chapter_id DROP NOT NULL").Error; err != nil {
			log.Warn("迁移：scenes.chapter_id 改为 nullable 失败（SQLite 可能不支持 ALTER COLUMN，表已重建则无害）", "err", err)
		}

	models := []any{
		&config.AppSettings{},
		&novel.Novel{},
		&novel.PreferenceItem{},
		&chapter.Chapter{},
		&character.Character{},
		&character.CharacterRelation{},
		&timeline.TimelineEntry{},
		&storyarc.StoryArc{},
		&storyarc.ArcNode{},
		&location.Location{},
		&location.LocationRelation{},
		&reader.ReaderPerspective{},
		&session.Session{},
		&session.Message{},
		&session.ModelUsage{},
		&storage.OperationLogRecord{},
		&rollback.TurnCommit{},
		&style.Sample{},
		&writing.WritingLog{},
		&writing.WritingSnapshot{},
		&lore.LoreEntry{},
		&item.Item{},
		&scene.Scene{},
		&chapter.ChapterArc{},
		&itemoccurrence.ItemOccurrence{},
		&outline.Outline{},
		&outline.OutlineBeat{},
		&volume.Volume{},
	}

	for _, m := range models {
		if err := db.AutoMigrate(m); err != nil {
			return fmt.Errorf("migrate: %T: %w", m, err)
		}
	}

	// 数据迁移：从 story_arcs WHERE arc_type='volume' 迁移到 volumes 表
	migrateVolumeData(db, log)

	log.Info("数据库迁移完成", "tables", len(models))
	return nil
}

// migrateVolumeData 将 story_arcs 中 arc_type='volume' 的数据迁移到 volumes 表。
// 幂等：如果 volumes 表已有数据则跳过。
func migrateVolumeData(db *gorm.DB, log *slog.Logger) {
	// 检查 volumes 表是否已有数据
	var count int64
	db.Model(&volume.Volume{}).Count(&count)
	if count > 0 {
		return // 已迁移，跳过
	}

	// 检查 story_arcs 中是否有 volume 类型数据
	var arcs []storyarc.StoryArc
	if err := db.Where("arc_type = 'volume'").Find(&arcs).Error; err != nil {
		log.Warn("迁移卷数据：查询 story_arcs 失败", "err", err)
		return
	}
	if len(arcs) == 0 {
		return
	}

	// 迁移数据
	for i, arc := range arcs {
		v := volume.Volume{
			NovelID:      arc.NovelID,
			Name:         arc.Name,
			Description:  arc.Description,
			StartChapter: arc.StartChapter,
			EndChapter:   arc.EndChapter,
			DetailJSON:   arc.DetailJSON,
			SortOrder:    i + 1,
		}
		if err := db.Create(&v).Error; err != nil {
			log.Warn("迁移卷数据：创建 volume 失败", "name", arc.Name, "err", err)
		}
	}
	log.Info("迁移卷数据完成", "count", len(arcs))
}
