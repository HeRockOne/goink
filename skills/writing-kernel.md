---
name: writing-kernel
description: 小说核心写作调度。按阶段工作，禁止跳过。阶段步骤用 set_phase 推进。
category: 核心系统
mode: always
---

# 写作核心系统

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

1. 加载技能（emotion-injection, chapter-hook-enhanced, maliang-method, dialogue-subtext）
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

| # | 检查项 | 条件 | 工具 |
|---|--------|------|------|
| 1 | **写章节元数据** | 每章必做 | update_chapter_meta（summary + key_events + characters_in + arc_ids 全部 required） |
| 2 | **更新写作快照** | 每章必做 | update_writing_snapshot（summary required） |
| 3 | **搜索设定防遗忘** | 每章必做 | search_lore |
| 4 | **搜索物品防断裂** | 每章必做 | search_items |
| 5 | **更新章节计划** | 每章必做 | update_chapter_plan（next/near/far） |
| 6 | **创建场景条目** | 本章有场景切换时 | create_scene（title + summary required） |
| 7 | **记录物品流转** | 物品持有者变化时 | create_item_occurrence（item_id + chapter_id + action 全部 required） |
| 8 | **更新角色状态** | 角色设定变化时 | update_character |
| 9 | **更新角色关系** | 关系变化时 | update_character_relationship（relation_describe required） |
| 10 | **推进弧线节点** | 节点完成时 | update_arc_node |
| 11 | **新伏笔/悬念** | 有新伏笔时 | create_timeline_entry（title + category + target_chapter 全部 required） |
| 12 | **回收伏笔** | 伏笔回收时 | update_timeline_entry（resolved_chapter_id） |
| 13 | **更新读者认知** | 新悬念/回收旧悬念时 | create_reader_perspective_entry / update_reader_perspective_entry |
| 14 | **更新故事状态** | 有重大进展时 | edit(goink.md) |
| 15 | **阶段切换** | 全部完成后 | set_phase("prepare") |

## 阶段技能表

| 阶段 | 技能 |
|------|------|
| prepare | common-sense-logic, genre-templates |
| outline | emotion-injection, chapter-hook-enhanced, maliang-method, dialogue-subtext |
| write | show-dont-tell, info-density, pov-purity, anti-ai-writing |
| write后 | revision-pass |
| review | run_subagent(agent_type="review") |
| maintain | anti-repetition |

## 硬约束

- 每章至少1个情绪锚点；情绪浓度高时禁止讲述句
- 每章至少1次快慢节奏切换；关键场景必须现场描写≥300字
- 禁止功能报告体（连续3段无感官描写）；禁止散文体
- 每章35-55段，每段60-80字，总字数2500-4000
- 字数不足禁止转阶段

## 工具使用规则

- read：用start_line/end_line只读需要的行
- edit：优先search_replace
- **关联规则**：更新物品持有者时同步create_item_occurrence；创建角色时传location_id；写完章节后update_writing_snapshot
