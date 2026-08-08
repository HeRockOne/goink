package app

import (
	"log/slog"
	"strings"

	"gorm.io/gorm"

	"novel/internal/config"
)

// defaultPhaseGateConfig 是出厂默认阶段门禁配置，与项目根目录 门禁配置示例.md 保持同步。
// 首次启动时写入 app_config，用户可在设置页修改或清空（清空后恢复默认）。
const defaultPhaseGateConfig = `<!-- phase-gate-config
mode: single
phase: init
tools: read_required, create_location, create_character, create_story_arc, create_arc_node, create_lore, create_item, create_timeline_entry, create_preference, get_characters, get_locations, get_story_arcs, get_lore, get_items, get_timeline, get_preferences, get_writing_context, set_phase
require: get_characters, get_locations, get_story_arcs, get_lore, get_items, get_timeline, get_preferences
require_reads: main-core-init-phase, main-tech-genre-templates, main-tech-book-outline, main-tech-character-design, main-tech-world-building-system
next: prepare
-->
<!-- phase-gate-config
mode: single
phase: prepare
tools: get_writing_context, get_chapter_list, read, read_required, get_characters, get_character_relations, get_timeline, get_story_arcs, get_locations, get_reader_perspective, get_preferences, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, search_story_memory, web_search, web_fetch, set_phase
require: get_writing_context, get_chapter_list, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_writing_snapshot, get_scenes, get_preferences
require_reads: main-tech-common-sense-logic
next: outline
-->
<!-- phase-gate-config
mode: single
phase: outline
tools: read, read_required, edit, get_chapter_list, get_characters, get_character_relations, get_timeline, get_story_arcs, get_locations, get_reader_perspective, get_preferences, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, get_writing_context, search_story_memory, web_search, web_fetch, set_phase
edit_paths: outlines/*, goink.md, book-outline.md, skills/*
require: edit
require_reads: main-tech-chapter-hook-enhanced, main-tech-chapter-title-design
next: write
-->
<!-- phase-gate-config
mode: single
phase: write
tools: read, read_required, edit, search_story_memory, get_characters, get_character_relations, get_timeline, get_story_arcs, get_reader_perspective, get_preferences, get_chapter_list, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, get_writing_context, web_search, web_fetch, set_phase
edit_paths: chapters/*
require: edit, get_chapter_list, read, read_required
require_reads: main-tech-show-dont-tell, main-tech-anti-ai-writing
next: review
-->
<!-- phase-gate-config
mode: single
phase: review
tools: read, read_required, edit, run_subagent, get_chapter_list, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_context, search_story_memory, web_search, web_fetch, set_phase
edit_paths: chapters/*
require: run_subagent
next: maintain
-->
<!-- phase-gate-config
mode: single
phase: maintain
tools: read, read_required, edit, update_character, update_character_relationship, create_lore, update_lore, search_lore, create_item, update_item, search_items, get_item_occurrences, create_item_occurrence, create_scene, update_scene, delete_lore, delete_item, delete_scene, create_timeline_entry, update_timeline_entry, update_chapter_plan, create_arc_node, update_arc_node, create_reader_perspective_entry, update_reader_perspective_entry, create_character, update_location, create_location, create_location_relation, update_location_relation, create_story_arc, update_story_arc, create_preference, update_preference, delete_record, get_chapter_list, get_characters, get_character_relations, get_timeline, get_story_arcs, get_locations, get_reader_perspective, get_preferences, get_lore, get_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, update_writing_snapshot, get_writing_context, update_chapter_meta, set_phase
edit_paths: goink.md, chapters/*, outlines/*, skills/*
require: edit, update_chapter_plan, update_chapter_meta, update_writing_snapshot, search_lore, search_items, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_scenes, get_item_occurrences, get_character_relations
require_reads: main-tech-anti-repetition, main-tech-foreshadow-cycle
next: prepare
-->
<!-- phase-gate-config
mode: batch
phase: init
tools: read_required, create_location, create_character, create_story_arc, create_arc_node, create_lore, create_item, create_timeline_entry, create_preference, get_characters, get_locations, get_story_arcs, get_lore, get_items, get_timeline, get_preferences, get_writing_context, set_phase
require: get_characters, get_locations, get_story_arcs, get_lore, get_items, get_timeline, get_preferences
require_reads: main-core-init-phase, main-tech-genre-templates, main-tech-book-outline, main-tech-character-design, main-tech-world-building-system
next: prepare
-->
<!-- phase-gate-config
mode: batch
phase: prepare
tools: get_writing_context, get_chapter_list, read, read_required, get_characters, get_character_relations, get_timeline, get_story_arcs, get_locations, get_reader_perspective, get_preferences, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, search_story_memory, web_search, web_fetch, set_phase
require: get_writing_context, get_chapter_list, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_writing_snapshot, get_scenes, get_preferences
require_reads: main-tech-common-sense-logic
next: outline
-->
<!-- phase-gate-config
mode: batch
phase: outline
tools: read, read_required, edit, get_chapter_list, get_characters, get_character_relations, get_timeline, get_story_arcs, get_locations, get_reader_perspective, get_preferences, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, get_writing_context, search_story_memory, web_search, web_fetch, set_phase
edit_paths: outlines/*, goink.md, book-outline.md, skills/*
require: edit
require_reads: main-tech-chapter-hook-enhanced, main-tech-chapter-title-design
next: write
-->
<!-- phase-gate-config
mode: batch
phase: write
tools: read, read_required, edit, search_story_memory, get_characters, get_character_relations, get_timeline, get_story_arcs, get_reader_perspective, get_preferences, get_chapter_list, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, get_writing_context, web_search, web_fetch, set_phase, create_scene, update_character, create_timeline_entry, update_timeline_entry, create_item_occurrence, update_writing_snapshot
edit_paths: chapters/*
require: edit, get_chapter_list, read, read_required, create_scene, update_character, create_timeline_entry, update_timeline_entry, create_item_occurrence, update_writing_snapshot
require_reads: main-tech-show-dont-tell, main-tech-anti-ai-writing
next: review
loop: true
-->
<!-- phase-gate-config
mode: batch
phase: review
tools: read, read_required, edit, run_subagent, get_chapter_list, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_context, search_story_memory, web_search, web_fetch, set_phase
edit_paths: chapters/*
require: run_subagent
next: maintain
-->
<!-- phase-gate-config
mode: batch
phase: maintain
tools: read, read_required, edit, update_character, update_character_relationship, create_lore, update_lore, search_lore, create_item, update_item, search_items, get_item_occurrences, create_item_occurrence, create_scene, update_scene, delete_lore, delete_item, delete_scene, create_timeline_entry, update_timeline_entry, update_chapter_plan, create_arc_node, update_arc_node, create_reader_perspective_entry, update_reader_perspective_entry, create_character, update_location, create_location, create_location_relation, update_location_relation, create_story_arc, update_story_arc, create_preference, update_preference, delete_record, get_chapter_list, get_characters, get_character_relations, get_timeline, get_story_arcs, get_locations, get_reader_perspective, get_preferences, get_lore, get_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, update_writing_snapshot, get_writing_context, update_chapter_meta, set_phase
edit_paths: goink.md, chapters/*, outlines/*, skills/*
require: edit, update_chapter_plan, update_chapter_meta, update_writing_snapshot, search_lore, search_items, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_scenes, get_item_occurrences, get_character_relations
require_reads: main-tech-anti-repetition, main-tech-foreshadow-cycle
next: done
-->
<!-- phase-gate-config
mode: batch
phase: done
tools: read
next: prepare
-->`

// EnsurePhaseGateConfigSeeded 首次启动时写入默认门禁配置，返回最新的设置对象。
// 已存在配置（用户改过）则跳过，避免覆盖用户自定义。
// 对旧版本配置做增量升级：batch write 阶段缺少迷你维护工具时自动合并（不覆盖用户其他修改）。
func EnsurePhaseGateConfigSeeded(db *gorm.DB) (*config.AppSettings, error) {
	s, err := config.LoadSettings(db)
	if err != nil {
		return nil, err
	}
	if s.PhaseGateConfig == "" {
		s.PhaseGateConfig = defaultPhaseGateConfig
		if err := config.SaveSettings(db, s); err != nil {
			return nil, err
		}
		return s, nil
	}
	// 增量升级：旧配置的 batch write 阶段缺迷你维护工具（tools 或 require）时补上
	if !strings.Contains(s.PhaseGateConfig, "create_item_occurrence, update_writing_snapshot") ||
		!strings.Contains(s.PhaseGateConfig, "require: edit, get_chapter_list, read, read_required, create_scene") {
		// 仅在 batch write 阶段块中补迷你维护工具（定位 "mode: batch" 后的 write 块）
		updated := upgradeBatchWriteTools(s.PhaseGateConfig)
		if updated != s.PhaseGateConfig {
			s.PhaseGateConfig = updated
			if err := config.SaveSettings(db, s); err != nil {
				return nil, err
			}
			slog.Info("门禁配置已增量升级：batch write 阶段补充迷你维护工具")
		}
	}
	return s, nil
}

// upgradeBatchWriteTools 在 batch 的 write 阶段 tools 行追加迷你维护工具（幂等）。
// 同时把迷你维护工具加入 require（阶段内累计：整批 write 循环中必须调用过这些工具，
// 未调用不能转 review——保证状态实时结算不会整批漏掉）。
func upgradeBatchWriteTools(cfg string) string {
	const miniTools = "create_scene, update_character, create_timeline_entry, update_timeline_entry, create_item_occurrence, update_writing_snapshot"
	lines := strings.Split(cfg, "\n")
	out := make([]string, 0, len(lines))
	inBatch := false
	inWrite := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "mode: batch") {
			inBatch = true
		}
		if strings.HasPrefix(trimmed, "mode:") && !strings.HasPrefix(trimmed, "mode: batch") {
			inBatch = false
			inWrite = false
		}
		if inBatch {
			if strings.HasPrefix(trimmed, "phase: write") {
				inWrite = true
			} else if strings.HasPrefix(trimmed, "phase:") {
				inWrite = false
			}
		}
		// batch write 块内的 tools 行：追加迷你维护工具
		if inBatch && inWrite && strings.HasPrefix(trimmed, "tools:") {
			if !strings.Contains(line, "create_item_occurrence") {
				out = append(out, line+", "+miniTools)
				continue
			}
		}
		// batch write 块内的 require 行：追加迷你维护工具
		if inBatch && inWrite && strings.HasPrefix(trimmed, "require:") {
			if !strings.Contains(line, "create_item_occurrence") {
				out = append(out, line+", "+miniTools)
				continue
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
