# 阶段门禁（Phase Gate）

## 概述

阶段门禁是 Goink 的创作流程强制执行系统。它确保 LLM 按照 writing-kernel.md 定义的阶段顺序执行，不能跳步或跳过必要操作。

**核心特性：**
- 系统级强制：每次对话自动激活，不依赖 LLM 配合
- 硬拦截：门禁检查在工具执行之前，被拦截的工具不会执行
- 主动推进：require 满足后**必须主动调 set_phase 切换阶段**，系统不自动推进
- 跨 turn 持久化：工具调用记录保存在 session 中
- 两种模式：单章（single）和批量（batch）
- 单轮内可回退修正，走完一轮完整流程（回到 prepare）后重置访问记录，防止第二轮任意跳转

## 设计哲学

**prepare 允许 edit**：一般编辑任务（改大纲、改角色设定）在 prepare 阶段自由使用，不受门禁拦截。

**require 触发收紧**：当 LLM 调用 get_chapter_list + get_characters + get_timeline（五门检查）时，require 满足，但门禁**不会自动推进**——必须由 LLM 主动调 `set_phase("outline")` 切换，后续流程受控。

**硬拦截**：门禁检查在 `registry.Execute` 之前。被拦截的工具不会执行，LLM 收到错误结果。

**回退修正**：单轮创作内，LLM 可回退到本轮已访问过的阶段（如 write 阶段发现大纲问题，回 outline 修改）。

**循环重置**：完成一轮完整流程（single 的 maintain→prepare，或 batch 的 maintain→done→prepare）后，访问记录重置——第二轮创作不能利用上一轮的访问历史任意跳转。

## 工作流程

### 单章模式（mode: single）

```
每次对话开始 → 自动进入 init/prepare 阶段
  ↓ prepare 允许 edit（一般编辑自由用）
  ↓ 调 get_chapter_list + get_characters + get_timeline（五门检查）
  ↓ require 满足后，LLM 主动调 set_phase("outline")
outline → 写大纲（require: edit）→ set_phase("write")
write → 写正文（require: edit）→ 字数校验 → set_phase("review")
review → 审读（require: run_subagent）→ set_phase("maintain")
maintain → 状态维护（require: update_chapter_plan, edit）→ set_phase("prepare")
  ↓ 回到 prepare（访问记录重置，开始新一轮）
```

### 批量模式（mode: batch）

```
init → prepare → [outline → write] × N 章循环 → review → maintain → done → prepare
```

每章完成后 maintain→done→prepare，访问记录重置后开始下一章。

## 工具白名单

> 下表为简化示意。**精确白名单以数据库配置为准**（`门禁配置示例.md` 或设置面板中的 phase_gate_config）。

| 阶段 | 允许的工具（简化） | 阻止的工具（简化） |
|------|-------------------|-------------------|
| init | create_*, get_*, set_phase | edit, update_*, delete_*, run_subagent |
| prepare | get_*, read, search_story_memory, web_search, web_fetch, set_phase | edit, update_*, create_*, delete_*, run_subagent |
| outline | read, edit(get: outlines/*, goink.md, skills/*), get_*, set_phase | update_*, create_*, delete_*, run_subagent |
| write | read, edit(get: chapters/*), search_story_memory, get_*, set_phase | update_*, create_*, delete_*, run_subagent |
| review | read, edit(get: chapters/*), run_subagent, get_*, set_phase | update_*, create_*, delete_* |
| maintain | read, edit(goink.md, chapters/*, outlines/*, skills/*), update_*, create_*, delete_*, get_*, set_phase | run_subagent |

> **注意**：get_lore、get_items、get_scenes、get_stats、get_writing_snapshot 属于 get_*，在全部阶段可用。
> create_lore、create_item、create_scene、update_lore、update_item、update_scene、delete_lore、delete_item、delete_scene、update_writing_snapshot 属于 create_*/update_*/delete_*，仅在 init 和 maintain 阶段可用（即新建/修改设定的操作集中在开书与维护阶段）。
> set_phase 在所有阶段始终可用（它是阶段切换的唯一入口）。

## require 完成条件

| 阶段 | require | 说明 |
|------|---------|------|
| prepare | get_chapter_list, get_characters, get_timeline | 五门检查必须做 |
| outline | edit | 大纲必须写入文件 |
| write | edit | 正文必须写入文件 |
| review | run_subagent | Review agent 必须启动 |
| maintain | update_chapter_plan, edit | 章节计划和 goink.md 必须更新 |

> require 只统计**成功调用**（`phase_gate.go` `successfulTools`）——失败不算，防止"调了但没做成"蒙混过关。

## 跨 Turn 持久化

门禁状态保存在 `sessions` 表：
- `current_phase`：当前阶段名
- `called_tools`：已调用工具的 JSON 计数

每次 `agent.Run()` 结束时自动保存，下次对话时自动恢复。

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
| next | 是 | require 满足后可进入的下一阶段 |
| edit_paths | 否 | edit 工具的路径范围（如 "outlines/*, goink.md"，"*"=不限制） |
| loop | 否 | "true" 表示 batch 模式下可循环 |

## 故障排查

| 现象 | 原因 | 解决 |
|------|------|------|
| 工具被拦截 | 当前阶段不允许该工具 | 完成当前阶段 require 后，主动调 `set_phase` 切换到目标阶段 |
| 阶段不推进 | require 未满足，或未调 set_phase | 先调用 require 列表中的工具，再主动 `set_phase` 切换 |
| 切换被拒 | 目标阶段不在 next 链，也不在本轮 visited | 只能推进到 next 或回退到本轮已访问过的阶段 |
| 第二轮可任意跳转（旧 bug） | visited 永久累积 | 已修复：回到 prepare 时重置访问记录 |
| 批量模式不循环 | write 阶段没有 `loop: true` | 检查 writing-kernel.md 配置 |
| 门禁未激活 | session 的 current_phase 为空 | 每次对话自动激活，检查 DB |

## 设置开关

阶段门禁可在桌面端「设置」中开启/关闭：

- 开启时：AI 严格按照阶段顺序执行，工具调用受白名单限制
- 关闭时：AI 可自由调用所有工具，无阶段限制

默认开启。

## API 访问

通过 HTTP API 进行对话时，阶段门禁同样生效：

- `POST /api/chat` 发送消息后，Agent 按当前 session 的阶段执行
- 门禁状态持久化在 `sessions` 表的 `current_phase` 字段
- 新会话自动从配置的第一个阶段（init，若有）开始，之后由 LLM 主动 set_phase 推进


## 示例门禁配置

以下是完整的阶段门禁配置，可直接复制到设置中使用（每个 `<!-- phase-gate-config -->` 块是一个阶段的规则）：

```yaml
# prepare 阶段：只读上下文搜集
mode: single
phase: prepare
tools: get_writing_context, get_chapter_list, read, get_characters, ...
require: get_writing_context, get_chapter_list, get_characters, ...
next: outline

# outline 阶段：写大纲
mode: single
phase: outline
tools: read, edit, ...
require: edit
next: write

# write 阶段：写正文
mode: single
phase: write
tools: read, edit, ...
require: edit, get_chapter_list
next: review

# review 阶段：审稿
mode: single
phase: review
tools: read, edit, run_subagent, ...
require: run_subagent
next: maintain

# maintain 阶段：状态维护
mode: single
phase: maintain
tools: read, edit, update_*, create_*, ...
require: edit, update_chapter_plan, update_chapter_meta, update_writing_snapshot, search_lore, search_items, ...
next: prepare
```

完整配置见下方（HTML 注释块，系统解析用）：

<!-- phase-gate-config
mode: single
phase: init
tools: read, create_location, create_character, create_story_arc, create_arc_node, create_lore, create_item, create_timeline_entry, create_preference, get_characters, get_locations, get_story_arcs, get_lore, get_items, get_timeline, get_preferences, get_writing_context, set_phase
require: get_characters, get_locations, get_story_arcs, get_lore, get_items, get_timeline, get_preferences
next: prepare
-->
<!-- phase-gate-config
mode: single
phase: prepare
tools: get_writing_context, get_chapter_list, read, get_characters, get_character_relations, get_timeline, get_story_arcs, get_locations, get_reader_perspective, get_preferences, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, get_phase_gate_config, search_story_memory, web_search, web_fetch, set_phase
require: get_writing_context, get_chapter_list, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_writing_snapshot, get_scenes, get_preferences
next: outline
-->
<!-- phase-gate-config
mode: single
phase: outline
tools: read, edit, get_chapter_list, get_characters, get_character_relations, get_timeline, get_story_arcs, get_locations, get_reader_perspective, get_preferences, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, get_writing_context, get_phase_gate_config, search_story_memory, web_search, web_fetch, set_phase
edit_paths: outlines/*, goink.md, skills/*
require: edit
next: write
-->
<!-- phase-gate-config
mode: single
phase: write
tools: read, edit, search_story_memory, get_characters, get_character_relations, get_timeline, get_story_arcs, get_reader_perspective, get_preferences, get_chapter_list, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, get_writing_context, get_phase_gate_config, web_search, web_fetch, set_phase
edit_paths: chapters/*
require: edit, get_chapter_list
next: review
-->
<!-- phase-gate-config
mode: single
phase: review
tools: read, edit, run_subagent, get_chapter_list, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_context, get_phase_gate_config, search_story_memory, web_search, web_fetch, set_phase
edit_paths: chapters/*
require: run_subagent
next: maintain
-->
<!-- phase-gate-config
mode: single
phase: maintain
tools: read, edit, update_character, update_character_relationship, create_lore, update_lore, search_lore, create_item, update_item, search_items, get_item_occurrences, create_item_occurrence, create_scene, update_scene, delete_lore, delete_item, delete_scene, create_timeline_entry, update_timeline_entry, update_chapter_plan, create_arc_node, update_arc_node, create_reader_perspective_entry, update_reader_perspective_entry, create_character, update_location, create_location, create_location_relation, update_location_relation, create_story_arc, update_story_arc, create_preference, update_preference, delete_record, get_chapter_list, get_characters, get_character_relations, get_timeline, get_story_arcs, get_locations, get_reader_perspective, get_preferences, get_lore, get_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, update_writing_snapshot, get_writing_context, get_phase_gate_config, update_phase_gate_config, update_chapter_meta, set_phase
edit_paths: goink.md, chapters/*, outlines/*, skills/*
require: edit, update_chapter_plan, update_chapter_meta, update_writing_snapshot, search_lore, search_items, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_scenes, get_item_occurrences, get_character_relations
next: prepare
-->
<!-- phase-gate-config
mode: batch
phase: init
tools: read, create_location, create_character, create_story_arc, create_arc_node, create_lore, create_item, create_timeline_entry, create_preference, get_characters, get_locations, get_story_arcs, get_lore, get_items, get_timeline, get_preferences, get_writing_context, set_phase
require: get_characters, get_locations, get_story_arcs, get_lore, get_items, get_timeline, get_preferences
next: prepare
-->
<!-- phase-gate-config
mode: batch
phase: prepare
tools: get_writing_context, get_chapter_list, read, get_characters, get_character_relations, get_timeline, get_story_arcs, get_locations, get_reader_perspective, get_preferences, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, get_phase_gate_config, search_story_memory, web_search, web_fetch, set_phase
require: get_writing_context, get_chapter_list, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_writing_snapshot, get_scenes, get_preferences
next: outline
-->
<!-- phase-gate-config
mode: batch
phase: outline
tools: read, edit, get_chapter_list, get_characters, get_character_relations, get_timeline, get_story_arcs, get_locations, get_reader_perspective, get_preferences, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, get_writing_context, get_phase_gate_config, search_story_memory, web_search, web_fetch, set_phase
edit_paths: outlines/*, goink.md, skills/*
require: edit
next: write
-->
<!-- phase-gate-config
mode: batch
phase: write
tools: read, edit, search_story_memory, get_characters, get_character_relations, get_timeline, get_story_arcs, get_reader_perspective, get_preferences, get_chapter_list, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, get_writing_context, get_phase_gate_config, web_search, web_fetch, set_phase
edit_paths: chapters/*
require: edit, get_chapter_list
next: review
-->
<!-- phase-gate-config
mode: batch
phase: review
tools: read, edit, run_subagent, get_chapter_list, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_lore, search_lore, get_items, search_items, get_scenes, get_item_occurrences, get_stats, get_writing_context, get_phase_gate_config, search_story_memory, web_search, web_fetch, set_phase
edit_paths: chapters/*
require: run_subagent
next: maintain
-->
<!-- phase-gate-config
mode: batch
phase: maintain
tools: read, edit, update_character, update_character_relationship, create_lore, update_lore, search_lore, create_item, update_item, search_items, get_item_occurrences, create_item_occurrence, create_scene, update_scene, delete_lore, delete_item, delete_scene, create_timeline_entry, update_timeline_entry, update_chapter_plan, create_arc_node, update_arc_node, create_reader_perspective_entry, update_reader_perspective_entry, create_character, update_location, create_location, create_location_relation, update_location_relation, create_story_arc, update_story_arc, create_preference, update_preference, delete_record, get_chapter_list, get_characters, get_character_relations, get_timeline, get_story_arcs, get_locations, get_reader_perspective, get_preferences, get_lore, get_items, get_scenes, get_item_occurrences, get_stats, get_writing_snapshot, update_writing_snapshot, get_writing_context, get_phase_gate_config, update_phase_gate_config, update_chapter_meta, set_phase
edit_paths: goink.md, chapters/*, outlines/*, skills/*
require: edit, update_chapter_plan, update_chapter_meta, update_writing_snapshot, search_lore, search_items, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_scenes, get_item_occurrences, get_character_relations
next: done
-->
<!-- phase-gate-config
mode: batch
phase: done
tools: read
next: prepare
-->
