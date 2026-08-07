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
批量：init → prepare → [outline ⇄ write × N 章] → review → maintain → done → prepare
```

## require 清单（必须成功调用，失败不算）

| 阶段 | require | 说明 |
|------|---------|------|
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
| require_reads | 否 | 必须用 read_required 读取的技能名列表（如 `main-tech-show-dont-tell, main-tech-anti-ai-writing`）。阶段内强制：切换阶段时检查本阶段是否读过，跨阶段读取不算；支持 `*` 通配符 |
| next | 是 | require 满足后可进入的下一阶段（旧字段名 main-cmd-next 已废弃，只解析 next） |
| edit_paths | 否 | edit 工具的路径范围（如 "outlines/*, goink.md"，"*"=不限制） |
| loop | 否 | "true" 表示 batch 模式下可循环（write 可回退 outline，连续多章写作） |

配置存在数据库（phase_gate_config），出厂自动 seed 默认配置（single + batch），用户可在设置面板修改。AI 可用 `get_phase_gate_config` 查看、`update_phase_gate_config` 编辑。各阶段默认必读技能：init 5 个开书技能、prepare common-sense-logic、outline hook-enhanced+title-design、write show-dont-tell+anti-ai-writing、maintain anti-repetition+foreshadow-cycle（用 `read_required(skills="...")` 加载，技能名从门禁配置读取，不硬编码）。

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
- 新会话自动从 prepare 开始（current_phase 为空时强制 prepare），之后由 AI 主动 set_phase 推进
