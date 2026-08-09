package app

import (
	"gorm.io/gorm"

	"novel/internal/config"
)

// defaultPhaseGateConfig 是出厂默认阶段门禁配置，与项目根目录 门禁配置示例.md 保持同步。
// 首次启动时写入 app_config，用户可在设置页修改或清空（清空后恢复默认）。
const defaultPhaseGateConfig = `<!-- phase-gate-config
mode: single
phase: init
tools: auto_skill_injection, edit, create_location, create_character, create_story_arc, create_arc_node, create_lore, create_item, create_timeline_entry, create_preference, get_characters, get_locations, get_story_arcs, get_lore, get_items, get_timeline, get_preferences, get_writing_context, set_phase
edit_paths: book-outline.md, goink.md
require: get_characters, get_locations, get_story_arcs, get_lore, get_items, get_timeline, get_preferences
require_reads: main-core-init-phase, main-tech-genre-templates, main-tech-book-outline, main-tech-character-design, main-tech-world-building-system
next: prepare
-->
<!-- phase-gate-config
mode: single
phase: prepare
tools: get_writing_context, get_chapter_list, read, auto_skill_injection, get_characters, get_character_relations, get_timeline, get_story_arcs, get_locations, get_reader_perspective, get_preferences, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, search_story_memory, web_search, web_fetch, set_phase
require: get_writing_context, get_chapter_list, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_writing_snapshot, get_scenes, get_preferences
require_reads: main-tech-common-sense-logic
next: outline
-->
<!-- phase-gate-config
mode: single
phase: outline
tools: read, auto_skill_injection, edit, get_chapter_list, get_characters, get_character_relations, get_timeline, get_story_arcs, get_locations, get_reader_perspective, get_preferences, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, get_writing_context, search_story_memory, web_search, web_fetch, set_phase
edit_paths: outlines/*, goink.md, book-outline.md
require: edit
require_reads: main-tech-chapter-hook-enhanced, main-tech-chapter-title-design
next: write
-->
<!-- phase-gate-config
mode: single
phase: write
tools: read, auto_skill_injection, edit, search_story_memory, get_characters, get_character_relations, get_timeline, get_story_arcs, get_reader_perspective, get_preferences, get_chapter_list, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, get_writing_context, web_search, web_fetch, set_phase, create_item_occurrence, update_writing_snapshot
edit_paths: chapters/*
require: edit, get_chapter_list, read
require_reads: main-tech-show-dont-tell, main-tech-anti-ai-writing, main-tech-pov-purity, main-tech-info-density
next: review
-->
<!-- phase-gate-config
mode: single
phase: review
tools: read, auto_skill_injection, edit, run_subagent, get_chapter_list, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_context, search_story_memory, web_search, web_fetch, set_phase
edit_paths: chapters/*
require: run_subagent
next: maintain
-->
<!-- phase-gate-config
mode: single
phase: maintain
tools: read, auto_skill_injection, edit, update_character, update_character_relationship, create_lore, update_lore, search_lore, create_item, update_item, search_items, get_item_occurrences, create_item_occurrence, create_scene, update_scene, delete_lore, delete_item, delete_scene, create_timeline_entry, update_timeline_entry, update_chapter_plan, create_arc_node, update_arc_node, create_reader_perspective_entry, update_reader_perspective_entry, create_character, update_location, create_location, create_location_relation, update_location_relation, create_story_arc, update_story_arc, create_preference, update_preference, delete_record, get_chapter_list, get_characters, get_character_relations, get_timeline, get_story_arcs, get_locations, get_reader_perspective, get_preferences, get_lore, get_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, update_writing_snapshot, get_writing_context, update_chapter_meta, set_phase
edit_paths: goink.md, chapters/*, outlines/*
require: edit, update_chapter_plan, update_chapter_meta, update_writing_snapshot, search_lore, search_items, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_scenes, get_item_occurrences, get_character_relations
require_reads: main-tech-anti-repetition, main-tech-foreshadow-cycle
next: prepare
-->
<!-- phase-gate-config
mode: batch
phase: init
tools: auto_skill_injection, edit, create_location, create_character, create_story_arc, create_arc_node, create_lore, create_item, create_timeline_entry, create_preference, get_characters, get_locations, get_story_arcs, get_lore, get_items, get_timeline, get_preferences, get_writing_context, set_phase
edit_paths: book-outline.md, goink.md
require: get_characters, get_locations, get_story_arcs, get_lore, get_items, get_timeline, get_preferences
require_reads: main-core-init-phase, main-tech-genre-templates, main-tech-book-outline, main-tech-character-design, main-tech-world-building-system
next: prepare
-->
<!-- phase-gate-config
mode: batch
phase: prepare
tools: get_writing_context, get_chapter_list, read, auto_skill_injection, get_characters, get_character_relations, get_timeline, get_story_arcs, get_locations, get_reader_perspective, get_preferences, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, search_story_memory, web_search, web_fetch, set_phase
require: get_writing_context, get_chapter_list, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_writing_snapshot, get_scenes, get_preferences
require_reads: main-tech-common-sense-logic
next: outline
-->
<!-- phase-gate-config
mode: batch
phase: outline
tools: read, auto_skill_injection, edit, get_chapter_list, get_characters, get_character_relations, get_timeline, get_story_arcs, get_locations, get_reader_perspective, get_preferences, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, get_writing_context, search_story_memory, web_search, web_fetch, set_phase
edit_paths: outlines/*, goink.md, book-outline.md
require: edit
require_reads: main-tech-chapter-hook-enhanced, main-tech-chapter-title-design
next: write
-->
<!-- phase-gate-config
mode: batch
phase: write
tools: read, auto_skill_injection, edit, search_story_memory, get_characters, get_character_relations, get_timeline, get_story_arcs, get_reader_perspective, get_preferences, get_chapter_list, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, get_writing_context, web_search, web_fetch, set_phase, create_scene, update_character, create_timeline_entry, update_timeline_entry, create_item_occurrence, update_writing_snapshot
edit_paths: chapters/*
require: edit, get_chapter_list, read, create_scene, update_character, create_timeline_entry, update_timeline_entry, create_item_occurrence, update_writing_snapshot
require_reads: main-tech-show-dont-tell, main-tech-anti-ai-writing, main-tech-pov-purity, main-tech-info-density
next: review
loop: true
-->
<!-- phase-gate-config
mode: batch
phase: review
tools: read, auto_skill_injection, edit, run_subagent, get_chapter_list, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_context, search_story_memory, web_search, web_fetch, set_phase
edit_paths: chapters/*
require: run_subagent
next: maintain
-->
<!-- phase-gate-config
mode: batch
phase: maintain
tools: read, auto_skill_injection, edit, update_character, update_character_relationship, create_lore, update_lore, search_lore, create_item, update_item, search_items, get_item_occurrences, create_item_occurrence, create_scene, update_scene, delete_lore, delete_item, delete_scene, create_timeline_entry, update_timeline_entry, update_chapter_plan, create_arc_node, update_arc_node, create_reader_perspective_entry, update_reader_perspective_entry, create_character, update_location, create_location, create_location_relation, update_location_relation, create_story_arc, update_story_arc, create_preference, update_preference, delete_record, get_chapter_list, get_characters, get_character_relations, get_timeline, get_story_arcs, get_locations, get_reader_perspective, get_preferences, get_lore, get_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, update_writing_snapshot, get_writing_context, update_chapter_meta, set_phase
edit_paths: goink.md, chapters/*, outlines/*
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
func EnsurePhaseGateConfigSeeded(db *gorm.DB) (*config.AppSettings, error) {
	s, err := config.LoadSettings(db)
	if err != nil {
		return nil, err
	}
	if s.PhaseGateConfig != "" {
		return s, nil
	}
	s.PhaseGateConfig = defaultPhaseGateConfig
	if err := config.SaveSettings(db, s); err != nil {
		return nil, err
	}
	return s, nil
}
