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
- 主动推进：require 满足后**必须主动调 `set_phase` 切换阶段**，系统不自动推进
- 跨 turn 持久化：当前阶段和已调用工具记录存在 sessions 表，断点续作自动恢复
- 两种模式：单章（single）和批量（batch，支持 outline⇄write 多章循环）
- 回退修正：单轮内可回退到本轮已访问过的阶段（如 write 发现大纲问题回 outline 修改）
- 循环重置：回到 prepare 后访问记录重置，第二轮不能利用上一轮历史任意跳转

## 阶段流程

```
单章：prepare → outline → write → review → maintain → prepare
批量：init → prepare → outline（一次出 N 章大纲）→ write（循环 N 章，每章动笔前 read 本章大纲，每章写后迷你维护，每 3 章轻量自检）→ review（批末审稿覆盖全批）→ maintain → prepare
```

> 批量模式 `[outline ⇄ write × N 章]` 含义：outline 阶段一次性产出全部 N 章大纲（连续 edit outlines/001.md ~ NNN.md），
> 然后 write 阶段循环写 N 章正文。**循环中每章 write 前必须 read outlines/NNN.md（本章大纲，门禁 require 强制）**，
> 把本章大纲锚定在上下文末尾再动笔，防止把别的章的大纲内容串进本章正文。write 阶段配置 `loop: true`，可回退 outline 修改大纲。

> **批量状态实时性（迷你维护）**：每章 write 完成后立即执行"迷你维护"——只写不查，把本章事实写入 DB
> （create_scene + update_character + create_timeline_entry + update_timeline_entry + create_item_occurrence + update_writing_snapshot），
> 不调用 get_*/search_* 查询。这样下一章的 get_writing_context 读到的是最新角色/伏笔/场景状态，
> 避免"整批末尾才 maintain 导致第 N 章读到第 1 章状态"。整批末尾仍保留完整 maintain（13 项清单 + goink.md 指纹）收尾核对。

> **批量质量节奏（每 3 章自检 + 批末全批审稿）**：write 循环每写 3 章停笔自检一次——
> **一致性（重点）**：调 get_characters / get_timeline / get_writing_snapshot 读取状态，对照最近 3 章
> 正文检查设定矛盾（角色状态/时间线/伏笔/章节衔接/重复），发现问题立即 edit 修复；
> **文笔（次重点）**：read main-tech-revision-pass + sub-tech-anti-ai-grade 检查节奏/AI 味。
> 不攒批末、不调 run_subagent 不 set_phase（write 阶段白名单无 run_subagent）。批末 review 阶段
> run_subagent 审读**本批全部 N 章**（子代理 fork 完整主历史可见全部正文），一致性优先，
> 逐章 read 自查 + edit 修复 + 字数复查，不要只审第 1 章。
> **批量 write 显式循环 + 注入去重**：每章写完后调 set_phase("write") 声明下一章边界
> （同阶段切换幂等成功，只产生显式阶段记录，不重置校验状态）。系统自动注入**已去重**：
> 只注入本阶段缺失的必读技能（injectPhaseSkills 按 missingInjections 计算），已注入过或
> LLM 已读过的技能不重复注入——每章 set_phase("write") 不再有重复注入成本。
> 自检/修复/迷你维护不需要 set_phase；**上下文压缩后门禁清空技能记录**（技能已不在上下文），
> 需按需补读（auto_skill_injection 或下次 set_phase 自动注入），不要因"技能读过"而跳过补读。

## require 清单（必须成功调用，失败不算）

| 阶段 | require | 说明 |
|------|---------|------|
| init | get_characters, get_locations, get_story_arcs, get_lore, get_items, get_timeline, get_preferences | 开书：7 项查询确认现状（新书另写 book-outline.md 总纲） |
| prepare | get_writing_context, get_chapter_list, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_writing_snapshot, get_scenes, get_preferences | 9 项必查，全量状态必须加载 |
| outline | edit | 大纲必须写入 outlines/NNN.md |
| write | edit, get_chapter_list | 正文必须写入 chapters/NNN.md + 字数校验前置检查 |
| review | run_subagent | 审稿子 Agent 必须启动 |
| maintain | edit, update_chapter_plan, update_chapter_meta, update_writing_snapshot, search_lore, search_items, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_scenes, get_item_occurrences, get_character_relations | 13 项强制清单，设定/伏笔/关系全量维护 |

> require 只统计**成功调用**——调了但失败不算，防止蒙混过关。

## 各阶段行为要点

- **prepare**：允许 edit（一般编辑自由用）；9 项必查满足后主动 `set_phase("outline")`
- **write 转出字数校验**：`set_phase("review")` 前必须调用过 `get_chapter_list`（返回 `word_count_ok`），字数不在用户设定范围会被阻塞，需扩写后重新检查；进入 write 阶段时字数状态重置，每章独立校验
- **maintain**：每章必做 15 项检查（查询 A-G + 更新 1-15，见 main-core-writing-kernel.md），宁多调工具不可漏维护

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
| auto_skill_injection | 否 | 该阶段必读技能名列表（如 `main-tech-show-dont-tell, main-tech-anti-ai-writing`）。**系统在 set_phase 进入该阶段时自动注入**，模型无需手动调 auto_skill_injection 工具。支持 `*` 通配符（展开为技能库实际存在的技能再注入）。注入去重：已注入过或已读过的技能不重复注入 |
| next | 是 | require 满足后可进入的下一阶段（旧字段名 main-cmd-next 已废弃，只解析 next） |
| edit_paths | 否 | edit 工具的路径范围（如 "outlines/*, goink.md"，"*"=不限制） |
| loop | 否 | "true" 表示 batch 模式下可循环（write 可回退 outline，连续多章写作） |
| inject | 否 | 进入该阶段是否自动注入必读技能（默认 true；false=需手动 auto_skill_injection） |
| inject_dedup | 否 | 同阶段重复进入是否去重注入（默认 true，已注入过不重复；false=每次全量注入） |
| same_phase | 否 | 同阶段 set_phase 是否幂等成功（默认 true，批量"每章声明边界"语义；false=禁止同阶段重复 set_phase） |
| word_count_check | 否 | 转出该阶段是否强制 get_chapter_list 字数校验（缺省=仅 write 阶段强制；true=任何阶段都强制；false=不强制） |
| word_count_reset | 否 | 进入该阶段是否重置字数状态（缺省=仅进 write 时重置；true/false 显式覆盖） |
| mutating_guard | 否 | 事前技能强制：必读技能未加载前禁止创作/维护动作（默认 true；false=放行） |

配置存在数据库（phase_gate_config），出厂自动 seed 默认配置（single + batch），用户可在设置面板修改。AI 可用 `get_phase_gate_config` 查看、`update_phase_gate_config` 编辑。各阶段默认必读技能：init 5 个开书技能、prepare common-sense-logic、outline hook-enhanced+title-design、write show-dont-tell+anti-ai-writing+pov-purity+info-density、maintain anti-repetition+foreshadow-cycle（系统在 set_phase 进入该阶段时自动注入，技能名从门禁配置读取，不硬编码）。

> **行为开关缺省即 legacy 行为**：存量配置不写新字段时，行为与历史版本完全一致（字数校验仍只对 write 阶段生效）。新字段的意义是显式化——自定义阶段名、按阶段关闭注入/强制等场景不再需要改代码。

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
| maintain | edit, update_chapter_plan, update_chapter_meta, update_writing_snapshot, search_lore, search_items, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_scenes, get_item_occurrences, get_character_relations |

### auto_skill_injection 设计（该阶段核心方法论，对照 main-core-writing-kernel 阶段技能表）

| 阶段 | auto_skill_injection 建议 |
|------|-------------------|
| init | main-core-init-phase, main-tech-genre-templates, main-tech-book-outline, main-tech-character-design, main-tech-world-building-system |
| prepare | main-tech-common-sense-logic |
| outline | main-tech-chapter-hook-enhanced, main-tech-chapter-title-design |
| write | main-tech-show-dont-tell, main-tech-anti-ai-writing, main-tech-pov-purity, main-tech-info-density |
| review | 空（sub-* 技能由系统自动注入子代理） |
| maintain | main-tech-anti-repetition, main-tech-foreshadow-cycle |

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
