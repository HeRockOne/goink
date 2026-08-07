# 阶段门禁（Phase Gate）

## 设计动机（为什么存在门禁）

门禁不是凭空设计的，来自真实使用 AI 写小说时观察到的三个问题：

1. **AI 写完正文就不维护设定/伏笔**（偷懒跳过 maintain）→ 解决：maintain 15 项强制清单 + require 只统计成功调用
2. **AI 跳过流程直接写正文**（不听阶段指挥）→ 解决：阶段硬拦截，非白名单工具直接拒绝执行，不自动推进
3. **AI 设定前后矛盾**（凭记忆写，不查数据库）→ 解决：prepare 9 项必查 + get_writing_context 全量状态树 + 代码级闭环
**设计脉络**：本项目最初在 Chinese-novel-pipeline（用户自研 skill）里用 Python 状态机（`director.py`）验证了"按阶段推进创作"的理念，属于**建议性**状态机（脚本检测，AI 可能绕过）。Goink 将其升级为**强制性**硬拦截状态机：工具执行前检查（`phase_gate.go` + `registry.Execute` 之前拒绝）、require 成功计数防虚报、write 转出强制字数、edit 路径白名单。

**核心原则**：门禁让 AI 始终遵循 prepare → outline → write → review → maintain 稳定推进，防止它忘了更新设定、漏填设定、虚报过程、不读 skill 直接写、埋头不听指挥。宁可多调用工具，不可漏掉状态维护。

## 概述

阶段门禁是 Goink 的创作流程强制执行系统。它确保 LLM 按照 main-core-writing-kernel.md 定义的阶段顺序执行，不能跳步或跳过必要操作。

**核心特性：**
- 系统级强制：每次对话自动激活，不依赖 LLM 配合
- 硬拦截：门禁检查在工具执行之前，被拦截的工具不会执行
- 主动推进：require 满足后**必须主动调 set_phase 切换阶段**，系统不自动推进
- 跨 turn 持久化：工具调用记录保存在 session 中
- 两种模式：单章（single）和批量（batch）
- 单轮内可回退修正，走完一轮完整流程（回到 prepare）后重置访问记录，防止第二轮任意跳转

## 设计哲学

**prepare 允许 edit**：一般编辑任务（改大纲、改角色设定）在 prepare 阶段自由使用，不受门禁拦截。

**require 触发收紧**：当 LLM 完成 prepare 的 9 项必查（get_writing_context、get_chapter_list、get_characters、get_timeline、get_story_arcs、get_reader_perspective、get_writing_snapshot、get_scenes、get_preferences）时，require 满足，但门禁**不会自动推进**——必须由 LLM 主动调 `set_phase("outline")` 切换，后续流程受控。

**硬拦截**：门禁检查在 `registry.Execute` 之前。被拦截的工具不会执行，LLM 收到错误结果。

**回退修正**：单轮创作内，LLM 可回退到本轮已访问过的阶段（如 write 阶段发现大纲问题，回 outline 修改）。

**循环重置**：完成一轮完整流程（single 的 maintain→prepare，或 batch 的 maintain→done→prepare）后，访问记录重置——第二轮创作不能利用上一轮的访问历史任意跳转。

**字数校验（write 阶段转出）**：`set_phase("review")` 前强制检查：
- 必须调用过 `get_chapter_list`（其返回的 `word_count_ok` 写入门禁状态），未检查则阻塞
- `word_count_ok=false`（低于/高于用户设置的字数范围）则阻塞，AI 需扩写后重新检查
- **进入 write 阶段时重置字数状态**（2026-08-06 修复）：上一章检查通过的 `word_count_ok` 不会带到本章——每章必须用本章的 `get_chapter_list` 结果（旧实现用布尔值跨章，上一章达标会放行本章未达标）

## 工作流程

### 单章模式（mode: single）

```
每次对话开始 → 自动进入 prepare 阶段
  ↓ prepare 允许 edit（一般编辑自由用）
  ↓ 调 9 项必查（get_writing_context 等）
  ↓ require 满足后，LLM 主动调 set_phase("outline")
outline → 写大纲（require: edit）→ set_phase("write")
write → 写正文（require: edit + get_chapter_list）→ 字数校验 → set_phase("review")
review → 审读（require: run_subagent）→ set_phase("maintain")
maintain → 状态维护（require: 13 项清单）→ set_phase("prepare")
  ↓ 回到 prepare（访问记录重置，开始新一轮）
```

### 批量模式（mode: batch）

```
init → prepare → [outline → write] × N 章循环 → review → maintain → done → prepare
```

每章完成后 maintain→done→prepare，访问记录重置后开始下一章。

## 工具白名单

> 下表为简化示意。**精确白名单以数据库配置为准**（出厂时自动写入默认配置，也可在设置面板修改 phase_gate_config，或参考 `门禁配置示例.md`）。

| 阶段 | 允许的工具（简化） | 阻止的工具（简化） |
|------|-------------------|-------------------|
| init | create_*, get_*, set_phase | edit, update_*, delete_*, run_subagent |
| prepare | get_*, read, search_story_memory, web_search, web_fetch, set_phase | edit, update_*, create_*, delete_*, run_subagent |
| outline | read, edit(get: outlines/*, goink.md, book-outline.md, skills/*), get_*, set_phase | update_*, create_*, delete_*, run_subagent |
| write | read, edit(get: chapters/*), search_story_memory, get_*, set_phase | update_*, create_*, delete_*, run_subagent |
| review | read, edit(get: chapters/*), run_subagent, get_*, set_phase | update_*, create_*, delete_* |
| maintain | read, edit(goink.md, chapters/*, outlines/*, skills/*), update_*, create_*, delete_*, get_*, set_phase | run_subagent |

> **注意**：get_lore、get_items、get_scenes、get_stats、get_writing_snapshot 属于 get_*，在全部阶段可用。
> create_lore、create_item、create_scene、update_lore、update_item、update_scene、delete_lore、delete_item、delete_scene、update_writing_snapshot 属于 create_*/update_*/delete_*，仅在 init 和 maintain 阶段可用（即新建/修改设定的操作集中在开书与维护阶段）。
> set_phase 在所有阶段始终可用（它是阶段切换的唯一入口）。

## require 完成条件

| 阶段 | require | 说明 |
|------|---------|------|
| prepare | get_writing_context, get_chapter_list, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_writing_snapshot, get_scenes, get_preferences | 9 项必查（全量状态必须加载） |
| outline | edit | 大纲必须写入文件 |
| write | edit, get_chapter_list | 正文必须写入文件 + 字数校验前置检查 |
| review | run_subagent | Review agent 必须启动 |
| maintain | edit, update_chapter_plan, update_chapter_meta, update_writing_snapshot, search_lore, search_items, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_scenes, get_item_occurrences, get_character_relations | 13 项强制清单（设定/伏笔/关系全量维护） |
> require 只统计**成功调用**（`phase_gate.go` `successfulTools`）——失败不算，防止"调了但没做成"蒙混过关。

## 跨 Turn 持久化

门禁状态保存在 `sessions` 表：
- `current_phase`：当前阶段名
- `called_tools`：已调用工具的 JSON 计数（含 visited 访问记录，新格式 `{"tools":{...},"visited":[...]}`，兼容旧格式）

每次 `agent.Run()` 结束时自动保存，下次对话时自动恢复。visited 随状态持久化，保证断点续作后仍可回退到已访问过的阶段。

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
| require_reads | 否 | 必须用 read_required 读取的技能名列表（如 `main-tech-show-dont-tell, main-tech-anti-ai-writing`）。阶段内强制：切换阶段时检查本阶段是否读过，跨阶段读取不算；支持 `*` 通配符（如 `main-tech-*`） |
| next | 是 | require 满足后可进入的下一阶段 |
| edit_paths | 否 | edit 工具的路径范围（如 "outlines/*, goink.md"，"*"=不限制） |
| loop | 否 | "true" 表示 batch 模式下可循环（write 阶段可回退到上一阶段 outline，连续多章写作） |

> 批量模式循环：默认配置中 batch 的 write 阶段带 `loop: true`，配合 visited 回退机制实现「outline ⇄ write × N 章」。

## 故障排查

| 现象 | 原因 | 解决 |
|------|------|------|
| 工具被拦截 | 当前阶段不允许该工具 | 完成当前阶段 require 后，主动调 `set_phase` 切换到目标阶段 |
| 阶段不推进 | require 未满足，或未调 set_phase | 先调用 require 列表中的工具，再主动 `set_phase` 切换 |
| 切换被拒 | 目标阶段不在 next 链，也不在本轮 visited | 只能推进到 next 或回退到本轮已访问过的阶段 |
| 第二轮可任意跳转（旧 bug） | visited 永久累积 | 已修复：回到 prepare 时重置访问记录 |
| 批量模式不循环 | write 阶段没有 `loop: true` | 默认配置已带；自定义配置需在 batch 的 write 阶段加 `loop: true` |
| 门禁未激活 | phase_gate_config 为空 | 出厂首次启动自动 seed 默认配置（single + batch）；已配置过则不会被覆盖 |

## 设置开关

阶段门禁可在桌面端「设置」中开启/关闭，也可点窗口标题栏的盾牌图标（Shield/ShieldOff）快捷切换：

- 开启时：AI 严格按照阶段顺序执行，工具调用受白名单限制
- 关闭时：AI 可自由调用所有工具，无阶段限制

默认开启。开关状态存 `app_config.phase_gate_enabled`，标题栏按钮与设置页共用同一状态。

> 本文档内容已冗余到内置 skill `main-cmd-phase-gate`（manual 模式，`/` 触发）——docs 可能被清理，内置 skill 随 exe 分发，防止门禁说明丢失。

## API 访问

通过 HTTP API 进行对话时，阶段门禁同样生效：

- `POST /api/chat` 发送消息后，Agent 按当前 session 的阶段执行
- 门禁状态持久化在 `sessions` 表的 `current_phase` 字段
- 新会话自动从 prepare 开始（`current_phase` 为空时强制设为 `"prepare"`，见 `app/chat.go`），之后由 LLM 主动 set_phase 推进


## 示例门禁配置

完整配置见 [`门禁配置示例.md`](../../门禁配置示例.md)。**首次启动时系统自动将默认配置（与示例一致）写入数据库并启用，无需手动配置**；用户可在设置面板修改或清空（清空后下次启动恢复默认）。

> 配置字段名为 `next`（`internal/agent/phase_gate.go` 只解析 `next` / `fail_next`）。
