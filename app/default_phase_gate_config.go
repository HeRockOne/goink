package app

import (
	"gorm.io/gorm"

	"novel/internal/config"
)

// defaultPhaseGateConfig 是出厂默认阶段门禁配置，与项目根目录 门禁配置示例.md 保持同步。
// 首次启动时写入 app_config，用户可在设置页修改或清空（清空后恢复默认）。
// tools: 支持类别名（get/create/update/delete/search/remove）自动展开为对应前缀工具。
const defaultPhaseGateConfig = `<!-- phase-gate-config
mode: single
phase: init
tools: get, create, edit, auto_skill_injection, set_phase, update_outline, update_outline_beat, delete_outline_beat
edit_paths: goink.md
require: get_characters, get_locations, get_story_arcs, get_lore, get_items, get_timeline, get_preferences
auto_skill_injection: main-core-init-phase, main-tech-genre-templates, main-tech-book-outline, main-tech-character-design, main-tech-world-building-system
next: prepare
note: 先写全书总纲（update_outline + create_outline_beat），未写总纲禁止切换 prepare
-->
<!-- phase-gate-config
mode: single
phase: prepare
tools: get, search, read, auto_skill_injection, set_phase, web_search, web_fetch, update_writing_snapshot, update_chapter_plan, create_story_arc
require: get_writing_context, get_chapter_list, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_writing_snapshot, get_scenes, get_preferences
auto_skill_injection: main-tech-common-sense-logic
next: outline
note: 平行发出 9 项必查。只读为主，禁止 edit 和正文/大纲写入
-->
<!-- phase-gate-config
mode: single
phase: outline
tools: get, search, read, edit, auto_skill_injection, set_phase, web_search, web_fetch
edit_paths: outlines/*, goink.md
require: edit
auto_skill_injection: main-tech-chapter-hook-enhanced, main-tech-chapter-title-design
next: write
note: edit(outlines/NNN.md) 写大纲，不得超出本卷范围
-->
<!-- phase-gate-config
mode: single
phase: write
tools: get, search, read, edit, auto_skill_injection, set_phase, web_search, web_fetch, check_story_consistency, create_item_occurrence, update_writing_snapshot
edit_paths: chapters/*
require: edit, get_chapter_list, read, check_story_consistency
auto_skill_injection: main-tech-show-dont-tell, main-tech-anti-ai-writing, main-tech-pov-purity, main-tech-info-density, main-tech-word-count-calibration
next: review
note: edit(chapters/NNN.md) 写正文，read 本章大纲锚定后再动笔。禁止正文外的维护工具
-->
<!-- phase-gate-config
mode: single
phase: review
tools: get, search, read, edit, auto_skill_injection, set_phase, web_search, web_fetch, check_story_consistency, run_subagent
edit_paths: chapters/*
require: run_subagent, check_story_consistency
next: maintain
note: run_subagent 启动审稿，审完 edit 修正文。禁止维护工具
-->
<!-- phase-gate-config
mode: single
phase: maintain
tools: get, create, update, delete, search, read, edit, auto_skill_injection, set_phase, check_story_consistency
edit_paths: goink.md, chapters/*, outlines/*
require: edit, update_chapter_plan, update_chapter_meta, update_writing_snapshot, search_lore, search_items, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_scenes, get_item_occurrences, get_character_relations, check_story_consistency
auto_skill_injection: main-tech-anti-repetition, main-tech-foreshadow-cycle, main-tech-data-hygiene
next: done
note: 一轮内完成全部 14 项维护，不留待办
-->
<!-- phase-gate-config
mode: single
phase: done
tools: get, set_phase
next:
note: 本轮创作结束，等待用户发起新一轮
-->
<!-- phase-gate-config
mode: batch
phase: init
tools: get, create, edit, auto_skill_injection, set_phase, update_outline, update_outline_beat, delete_outline_beat
edit_paths: goink.md
require: get_characters, get_locations, get_story_arcs, get_lore, get_items, get_timeline, get_preferences
auto_skill_injection: main-core-init-phase, main-tech-genre-templates, main-tech-book-outline, main-tech-character-design, main-tech-world-building-system
next: prepare
note: 先写全书总纲（update_outline + create_outline_beat），未写总纲禁止切换 prepare
-->
<!-- phase-gate-config
mode: batch
phase: prepare
tools: get, search, read, auto_skill_injection, set_phase, web_search, web_fetch, update_writing_snapshot, update_chapter_plan, create_story_arc
require: get_writing_context, get_chapter_list, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_writing_snapshot, get_scenes, get_preferences
auto_skill_injection: main-tech-common-sense-logic
next: outline
note: 平行发出 9 项必查。只读为主，禁止 edit 和正文/大纲写入
-->
<!-- phase-gate-config
mode: batch
phase: outline
tools: get, search, read, edit, auto_skill_injection, set_phase, web_search, web_fetch
edit_paths: outlines/*, goink.md
require: edit
auto_skill_injection: main-tech-chapter-hook-enhanced, main-tech-chapter-title-design
next: write
note: edit(outlines/NNN.md) 写大纲，不得超出本卷范围
-->
<!-- phase-gate-config
mode: batch
phase: write
tools: get, search, read, edit, auto_skill_injection, set_phase, web_search, web_fetch, check_story_consistency, create_item_occurrence, update_writing_snapshot, create_scene, update_character, create_timeline_entry, update_timeline_entry
edit_paths: chapters/*
require: edit, get_chapter_list, read, create_scene, update_character, create_timeline_entry, update_timeline_entry, create_item_occurrence, update_writing_snapshot, check_story_consistency
auto_skill_injection: main-tech-show-dont-tell, main-tech-anti-ai-writing, main-tech-pov-purity, main-tech-info-density, main-tech-word-count-calibration
next: review
loop: true
note: edit(chapters/NNN.md) 写正文，read 本章大纲锚定后再动笔。禁止正文外的维护工具
-->
<!-- phase-gate-config
mode: batch
phase: review
tools: get, search, read, edit, auto_skill_injection, set_phase, web_search, web_fetch, check_story_consistency, run_subagent
edit_paths: chapters/*
require: run_subagent, check_story_consistency
next: maintain
note: run_subagent 启动审稿，审完 edit 修正文。禁止维护工具
-->
<!-- phase-gate-config
mode: batch
phase: maintain
tools: get, create, update, delete, search, read, edit, auto_skill_injection, set_phase, check_story_consistency
edit_paths: goink.md, chapters/*, outlines/*
require: edit, update_chapter_plan, update_chapter_meta, update_writing_snapshot, search_lore, search_items, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_scenes, get_item_occurrences, get_character_relations, check_story_consistency
auto_skill_injection: main-tech-anti-repetition, main-tech-foreshadow-cycle, main-tech-data-hygiene
next: done
note: 一轮内完成全部 14 项维护，不留待办
-->
<!-- phase-gate-config
mode: batch
phase: done
tools: get, set_phase
next:
note: 本轮创作结束，等待用户发起新一轮
-->
`

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