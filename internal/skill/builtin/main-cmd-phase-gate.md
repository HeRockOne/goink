---
name: main-cmd-phase-gate
description: 阶段门禁系统说明文档（阶段流程、require 清单、配置格式、故障排查）。用户询问门禁、阶段、require、被拦截、set_phase 时使用
category: 系统
mode: manual
author: builtin
version: 1
---
# 阶段门禁（Phase Gate）说明

本文是门禁系统的权威说明。docs/architecture/phase-gate.md 可能被清理，本 skill 随 exe 内置分发，防止门禁说明丢失。

## 门禁是什么

Goink 的创作流程强制执行系统。AI 必须按 prepare → outline → write → review → maintain 稳定推进，不能跳步、不能跳过状态维护、不能虚报过程。

核心机制：

- 硬拦截：门禁检查在工具执行之前（`registry.Execute` 之前），被拦截的工具不会执行，AI 收到错误结果
- 自动推进：require 满足后系统在回合收尾时自动推进到下一阶段并注入必读技能；也可主动调 `set_phase` 立即切换
- 跨 turn 持久化：当前阶段和已调用工具记录存在 sessions 表，断点续作自动恢复
- 两种模式：单章（single）和批量（batch，支持 outline⇄write 多章循环）
- 回退修正：单轮内可回退到本轮已访问过的阶段（如 write 发现大纲问题回 outline 修改）
- 循环重置：回到 prepare 后访问记录重置，第二轮不能利用上一轮历史任意跳转

## 阶段流程

```
单章：prepare → outline → write → review → maintain → prepare
批量：init → prepare → outline（一次出 N 章大纲）→ write（循环 N 章，每章动笔前 read 本章大纲，每章写后迷你维护）→ review → maintain → prepare
```

> 批量模式 `[outline ⇄ write × N 章]` 含义：outline 阶段一次性产出全部 N 章大纲（连续 edit outlines/001.md ~ NNN.md），
> 然后 write 阶段循环写 N 章正文。**循环中每章 write 前必须 read outlines/NNN.md（本章大纲，门禁 require 强制）**，
> 把本章大纲锚定在上下文末尾再动笔，防止把别的章的大纲内容串进本章正文。write 阶段配置 `loop: true`，可回退 outline 修改大纲。

> **批量状态实时性（迷你维护）**：每章 write 完成后立即执行"迷你维护"——只写不查，把本章事实写入 DB
> （create_scene + update_character + create_timeline_entry + update_timeline_entry + create_item_occurrence + update_writing_snapshot），
> 不调用 get_*/search_* 查询。这样下一章的 get_writing_context 读到的是最新角色/伏笔/场景状态，
> 避免"整批末尾才 maintain 导致第 N 章读到第 1 章状态"。整批末尾仍保留完整 maintain（14 项 require + 15 项检查清单 + goink.md 指纹）收尾核对。

## require 清单（必须成功调用，失败不算）

| 阶段 | require | 说明 |
|------|---------|------|
| init | get_characters, get_locations, get_story_arcs, get_lore, get_items, get_timeline, get_preferences | 开书：7 项查询确认现状（新书另写 book-outline.md 总纲） |
| prepare | get_writing_context, get_chapter_list, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_writing_snapshot, get_scenes, get_preferences | 9 项必查，全量状态必须加载 |
| outline | edit | 大纲必须写入 outlines/NNN.md |
| write | edit, get_chapter_list | 正文必须写入 chapters/NNN.md + 字数校验前置检查 |
| review | run_subagent | 审稿子 Agent 必须启动 |
| maintain | edit, update_chapter_plan, update_chapter_meta, update_writing_snapshot, search_lore, search_items, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_scenes, get_item_occurrences, get_character_relations, check_story_consistency | 14 项强制清单，设定/伏笔/关系全量维护 + 一致性核对 |

> require 只统计**成功调用**——调了但失败不算，防止蒙混过关。

## 各阶段行为要点

- **prepare**：允许 edit（一般编辑自由用）；9 项必查满足后主动 `set_phase("outline")`
- **write 转出字数校验**：`set_phase("review")` 前必须调用过 `get_chapter_list`（返回 `word_count_ok`），字数不在用户设定范围会被阻塞，需扩写后重新检查；进入 write 阶段时字数状态重置，每章独立校验
- **maintain**：每章必做 15 项检查（查询 A-G + 更新 1-15，含内容校准，见 main-core-writing-kernel.md 与 main-tech-data-hygiene），宁多调工具不可漏维护

## 配置格式

```markdown
<!-- phase-gate-config
mode: single
phase: prepare
tools: get_chapter_list, read, edit, get_characters, get_timeline
require: get_chapter_list, get_characters, get_timeline
next: outline
-->
```

| 字段 | 必填 | 说明 |
|------|------|------|
| mode | 否 | "single" 或 "batch"，空=两种模式都适用 |
| phase | 是 | 阶段名称 |
| tools | 是 | 该阶段允许使用的工具列表 |
| require | 是 | 必须调用过的工具列表 |
| auto_skill_injection | 否 | 该阶段必读技能名列表（如 `main-tech-show-dont-tell, main-tech-anti-ai-writing`）。**系统在 set_phase 进入该阶段时自动注入**，模型无需手动调 auto_skill_injection 工具。支持 `*` 通配符 |
| next | 是 | require 满足后可进入的下一阶段（旧字段名 main-cmd-next 已废弃，只解析 next） |
| edit_paths | 否 | edit 工具的路径范围（如 "outlines/*, goink.md"，"*"=不限制） |
| loop | 否 | "true" 表示 batch 模式下可循环（write 可回退 outline，连续多章写作） |

配置存在数据库（phase_gate_config），出厂自动 seed 默认配置（single + batch），用户可在设置面板修改。AI 可用 `get_phase_gate_config` 查看、`update_phase_gate_config` 编辑。各阶段默认必读技能：init 5 个开书技能、prepare common-sense-logic、outline hook-enhanced+title-design、write show-dont-tell+anti-ai-writing+pov-purity+info-density+word-count-calibration、maintain anti-repetition+foreshadow-cycle+data-hygiene（系统在 set_phase 进入该阶段时自动注入，技能名从门禁配置读取，不硬编码）。

## 配置设计指南（怎么配一套门禁）

### 工具分阶段分配（60 个工具按角色分组）

| 工具角色 | 工具名 | 放哪些阶段 |
|---------|--------|-----------|
| 技能加载 | auto_skill_injection | 有必读技能的阶段 |
| 文件读取 | read | outline / write / review / maintain |
| 查询 | get_characters, get_character_relations, get_locations, get_location_relations, get_timeline, get_story_arcs, get_arc_nodes, get_reader_perspective, get_preferences, get_lore, get_items, get_item_occurrences, get_scenes, get_stats, get_writing_snapshot, get_writing_context, get_chapter_list, get_entity_appearances | 所有阶段（随时查状态） |
| 搜索 | search_lore, search_items, search_story_memory, check_story_consistency | 所有阶段 |
| 网络 | web_search, web_fetch | prepare / outline（查资料考据） |
| 文件写入 | edit | 配合 edit_paths：init 写总纲（book-outline.md, goink.md）、outline 写大纲（outlines/*）、write 写正文（chapters/*）、review 修正文（chapters/*）、maintain 写指纹（goink.md） |
| 创建 | create_character, create_location, create_location_relation, create_story_arc, create_arc_node, create_lore, create_item, create_item_occurrence, create_scene, create_timeline_entry, create_reader_perspective_entry, create_preference | 只在 init（开书）和 maintain（补录） |
| 更新 | update_character, update_character_relationship, update_location, update_location_relation, update_story_arc, update_arc_node, update_lore, update_item, update_scene, update_timeline_entry, update_chapter_plan, update_reader_perspective_entry, update_preference, update_writing_snapshot, update_chapter_meta | 只在 maintain；batch write 额外放迷你维护 6 个（create_scene, update_character, create_timeline_entry, update_timeline_entry, create_item_occurrence, update_writing_snapshot） |
| 删除 | delete_lore, delete_item, delete_scene, delete_record | 只在 maintain |
| 审稿 | run_subagent | 只在 review |
| 门禁管理 | get_phase_gate_config, update_phase_gate_config, set_phase | 永远放行，可不写进 tools |

### require 设计（该阶段不完成就出事故的动作）

| 阶段 | require 建议 |
|------|-------------|
| init | 7 查询（characters/locations/story_arcs/lore/items/timeline/preferences） |
| prepare | 9 项必查（writing_context/chapter_list/characters/timeline/story_arcs/reader_perspective/writing_snapshot/scenes/preferences） |
| outline | edit |
| write | edit, get_chapter_list, read（字数校验转出时自动强制，无需配置） |
| review | run_subagent |
| maintain | edit, update_chapter_plan, update_chapter_meta, update_writing_snapshot, search_lore, search_items, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_scenes, get_item_occurrences, get_character_relations, check_story_consistency |

### auto_skill_injection 设计（该阶段核心方法论，对照 main-core-writing-kernel 阶段技能表）

| 阶段 | auto_skill_injection 建议 |
|------|-------------------|
| init | main-core-init-phase, main-tech-genre-templates, main-tech-book-outline, main-tech-character-design, main-tech-world-building-system |
| prepare | main-tech-common-sense-logic |
| outline | main-tech-chapter-hook-enhanced, main-tech-chapter-title-design |
| write | main-tech-show-dont-tell, main-tech-anti-ai-writing, main-tech-pov-purity, main-tech-info-density, main-tech-word-count-calibration |
| review | 空（sub-* 技能由系统自动注入子代理） |
| maintain | main-tech-anti-repetition, main-tech-foreshadow-cycle, main-tech-data-hygiene |

情景类技能（climax-scene/shuangdian-pacing/foreshadow-cycle/emotion-injection/pacing-control/scene-beats 等）不进 auto_skill_injection，由 kernel 按需引导。

### 常见设计错误（改配置后可用设置页「校验配置」按钮检查）

| 错误 | 后果 |
|------|------|
| require 引用了 tools 里没有的工具 | set_phase 永远被拦，流程卡死 |
| next 指向不存在的阶段名 | 切换时报"未知阶段" |
| 两个 mode 都空的同名阶段 | 只生效第一个（同名阶段必须写 mode 区分） |
| edit_paths 不含 require 需要的路径 | edit 被拦，require 永不满足 |
| auto_skill_injection 引用不存在的技能 | 自动注入跳过该技能（best-effort），需手动确认 |

## 故障排查

| 现象 | 原因 | 解决 |
|------|------|------|
| 工具被拦截 | 当前阶段不允许该工具 | 完成当前阶段 require 后，主动调 `set_phase` 切换到目标阶段 |
| 阶段不推进 | require 未满足，或未调 set_phase | 先调用 require 列表中的工具，再主动 `set_phase` 切换 |
| 切换被拒 | 目标阶段不在 next 链，也不在本轮 visited | 只能推进到 next 或回退到本轮已访问过的阶段 |
| 批量模式不循环 | write 阶段没有 `loop: true` | 默认配置已带；自定义配置需在 batch 的 write 阶段加 `loop: true` |
| 门禁未激活 | phase_gate_config 为空 | 出厂首次启动自动 seed；已配置过则不会被覆盖 |

## 开关

- 设置页或标题栏盾牌按钮（Shield/ShieldOff）可开关门禁
- 开启时：严格按阶段执行；关闭时：AI 可自由调用所有工具
- 新会话自动从配置的第一个阶段开始（默认 init，开书流程；已有小说的会话快速查 7 项确认现状后切 prepare），之后由 AI 主动 set_phase 推进
