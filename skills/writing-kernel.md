---
name: writing-kernel
description: 小说核心写作调度。按阶段工作，禁止跳过。阶段步骤用 set_phase 推进。
category: 核心系统
mode: always
---

# 写作核心系统

## skill目录（加载顺序，避免浪费 read 调用）

- 内置 skill（41 个通用 + 类型 skill）位于 `/builtin/skills/<name>.md`（只读），auto 模式按需 read
- 同名 skill 优先级：小说级 > 用户级 > 内置（`/builtin/skills/`）
- always 模式 skill 已在会话开头注入，无需再 read

## 流程

单章：prepare → outline → write → review → maintain → prepare
批量：prepare → outline ⇄ write（循环N章）→ review → maintain → done

## 阶段指令

### prepare

1. **get_writing_context**（required）— 一次获取树状全量状态
2. **get_chapter_list**（required）— 确认章节编号连续
3. **get_characters**（required）— 确认角色阵容
4. **get_timeline**（required）— 确认伏笔状态
5. **get_story_arcs**（required）— 确认弧线进度
6. **get_reader_perspective**（required）— 确认读者认知
7. **get_writing_snapshot**（required）— 确认写作进度
8. **get_scenes**（required）— 确认本章场景
9. **get_preferences**（required）— 确认创作偏好
10. 五门检查（字数/段数/情绪/节奏/禁止项）→ **set_phase("outline")**

### outline

1. 加载技能（emotion-injection, chapter-hook-enhanced, maliang-method, dialogue-subtext, chapter-title-hooks；新书首次 outline 加 golden-three-chapters、golden-finger-design，以及对应类型的专精 skill：xuanhuan-cultivation/urban-martial-arts/post-apocalyptic-survival/suspense-rule-horror/historical-time-travel）
2. **edit**(outlines/NNN.md)（required）— 写大纲（标题/基调/场景/关键事件/重点角色/伏笔操作/章末钩子）
3. 等待用户审批 → **set_phase("write")**

### write

1. 加载技能（show-dont-tell, info-density, pov-purity, anti-ai-writing）
2. **edit**(chapters/NNN.md)（required）— 写正文
3. 校验字数（2500-4000）
4. 记录关键物品出现 → create_item_occurrence
5. **set_phase("review")**

### review

1. **run_subagent**(agent_type="review")（required）— 启动审稿
2. 根据意见修复
3. **set_phase("maintain")**

### maintain（逐项检查清单，每章必做）

**每章必做的状态查询**（门禁 require 强制，宁可多调用不可漏维护）：

| # | 查询项 | 工具 | 确认目的 |
|---|--------|------|---------|
| A | 查角色状态 | get_characters | 是否有角色需要更新 |
| B | 查伏笔状态 | get_timeline | 是否有伏笔要回收/校准 |
| C | 查弧线进度 | get_story_arcs | 是否有节点要推进 |
| D | 查读者认知 | get_reader_perspective | 是否有悬念要维护 |
| E | 查场景状态 | get_scenes | 是否有场景要创建/更新 |
| F | 查物品流转 | get_item_occurrences | 是否有物品易主未记录 |
| G | 查角色关系 | get_character_relations | 是否有关系变化未记录 |

> 每章 maintain 必须完成以上 7 项状态查询 + 下方更新动作，门禁 require 才放行。查到有变化就执行对应更新工具（6-14 项）。

| # | 检查项 | 条件 | 工具 |
|---|--------|------|------|
| 1 | **写章节元数据** | 每章必做 | update_chapter_meta（summary + key_events + characters_in + arc_ids 全部 required） |
| 2 | **更新写作快照** | 每章必做 | update_writing_snapshot（summary required） |
| 3 | **搜索设定防遗忘** | 每章必做 | search_lore |
| 4 | **搜索物品防断裂** | 每章必做 | search_items |
| 5 | **更新章节计划** | 每章必做 | update_chapter_plan（next/near/far） |
| 6 | **创建场景条目** | 查询 E 发现有变化时 | create_scene（title + summary required） |
| 7 | **记录物品流转** | 查询 F 发现有变化时 | create_item_occurrence（item_id + chapter_id + action 全部 required） |
| 8 | **更新角色状态** | 查询 A 发现有变化时 | update_character |
| 9 | **更新角色关系** | 查询 G 发现有变化时 | update_character_relationship（relation_describe required） |
| 10 | **推进弧线节点** | 查询 C 发现有变化时 | update_arc_node |
| 11 | **新伏笔/悬念** | 有新伏笔时 | create_timeline_entry（title + category + target_chapter 全部 required） |
| 12 | **回收伏笔** | 查询 B 发现有要回收的时 | update_timeline_entry（resolved_chapter_id） |
| 13 | **更新读者认知** | 查询 D 发现有悬念变化时 | create_reader_perspective_entry / update_reader_perspective_entry |
| 14 | **更新故事状态** | 有重大进展时 | edit(goink.md) |
| 15 | **阶段切换** | 全部完成后 | set_phase("prepare") |

## 阶段技能表（33 个内置 skill 全量调度）

> 每阶段先加载对应技能再执行。manual 模式（collect/memory/next/review）由用户 `/` 触发，不在此表。

| 阶段 | 技能 |
|------|------|
| **init（开书）** | init-phase（开书流程）, genre-templates（12类型）, book-outline（总纲）, character-design（角色设计）, world-building-system（世界观） |
| **prepare（准备）** | common-sense-logic（一致性）, genre-templates, book-outline（卷纲）, brainstorm-composer（卡情节时构思） |
| **outline（大纲）** | book-outline（章节蓝图）, chapter-opening（每章开头）, chapter-hook-enhanced（章末钩子）, maliang-method（打脸/金手指节奏）, dialogue-subtext（对白设计）, emotional-arc（情感弧线）, opening-chapter（第一章开篇） |
| **write（正文）** | show-dont-tell（展示）, info-density（信息密度）, pov-purity（视角）, anti-ai-writing（八条铁律）, shuangdian-pacing（爽点节奏）, climax-scene（战斗章）, foreshadow-cycle（埋伏笔）, pacing-control（节奏控制）, scene-beats（场景节拍）, emotion-injection（情绪注入）, word-count-calibration（字数校准） |
| **write后（自审）** | revision-pass（修改润色）, anti-ai-grade（用词级反AI） |
| **review（审稿）** | run_subagent(agent_type="review") → review-standards（16项判定） |
| **maintain（维护）** | anti-repetition（去重）, foreshadow-cycle（回收伏笔） |
| **完结** | book-completion（完本清单） |

## 硬约束

- 每章至少1个情绪锚点；情绪浓度高时禁止讲述句
- 每章至少1次快慢节奏切换；关键场景必须现场描写≥300字
- 每章至少1个爽点（对照 shuangdian-pacing）；章末必有钩子且类型不与前2章重复
- 禁止功能报告体（连续3段无感官描写）；禁止散文体
- 每章35-55段，每段60-80字，总字数2500-4000
- 字数不足禁止转阶段
- 大战之间必须插非战斗章（对照 climax-scene）
- 完结前检查伏笔回收率≥90%（对照 foreshadow-cycle / book-completion）

## 工具使用规则

- read：用start_line/end_line只读需要的行
- edit：优先search_replace
- **关联规则**：更新物品持有者时同步create_item_occurrence；创建角色时传location_id；写完章节后update_writing_snapshot
