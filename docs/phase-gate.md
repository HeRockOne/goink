# 阶段门禁（Phase Gate）

## 概述

阶段门禁是 Goink 的创作流程强制执行系统。它确保 LLM 按照 writing-kernel.md 定义的阶段顺序执行，不能跳步或跳过必要操作。

**核心特性：**
- 系统级强制：每次对话自动激活，不依赖 LLM 配合
- 硬拦截：门禁检查在工具执行之前，被拦截的工具不会执行
- 自动推进：require 满足后自动进入下一阶段
- 跨 turn 持久化：工具调用记录保存在 session 中
- 两种模式：单章（single）和批量（batch）

## 设计哲学

**prepare 允许 edit**：一般编辑任务（改大纲、改角色设定）在 prepare 阶段自由使用，不受门禁拦截。

**require 触发收紧**：当 LLM 调用 get_chapter_list + get_characters + get_timeline（五门检查）时，require 满足，门禁自动推进到 outline 阶段，后续流程受控。

**硬拦截**：门禁检查在 `registry.Execute` 之前。被拦截的工具不会执行，LLM 收到错误结果。

## 工作流程

### 单章模式（mode: single）

```
每次对话开始 → 自动进入 prepare 阶段
  ↓ prepare 允许 edit（一般编辑自由用）
  ↓ 调 get_chapter_list + get_characters + get_timeline（五门检查）
  ↓ require 满足，自动推进
outline → 写大纲（require: edit）
  ↓ require 满足，自动推进
write → 写正文（require: edit）
  ↓ require 满足，自动推进
review → 审读（require: run_subagent）
  ↓ require 满足，自动推进
maintain → 状态维护（require: update_chapter_plan, edit）
  ↓ 完成
回到 prepare
```

### 批量模式（mode: batch）

```
prepare → [outline → write] × N 章循环 → review → maintain → done
```

## 工具白名单

| 阶段 | 允许的工具 | 阻止的工具 |
|------|-----------|-----------|
| prepare | get_*, read, edit, search_story_memory | update_*, create_*, run_subagent |
| outline | read, edit, get_*, search_story_memory | update_*, create_*, run_subagent |
| write | read, edit, search_story_memory, get_* | update_*, create_*, run_subagent |
| review | read, run_subagent, get_* | edit, update_*, create_* |
| maintain | read, edit, update_*, create_* | run_subagent |

## require 完成条件

| 阶段 | require | 说明 |
|------|---------|------|
| prepare | get_chapter_list, get_characters, get_timeline | 五门检查必须做 |
| outline | edit | 大纲必须写入文件 |
| write | edit | 正文必须写入文件 |
| review | run_subagent | Review agent 必须启动 |
| maintain | update_chapter_plan, edit | 章节计划和 goink.md 必须更新 |

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
| next | 是 | require 满足后进入的下一阶段 |
| loop | 否 | "true" 表示 batch 模式下可循环 |

## 故障排查

| 现象 | 原因 | 解决 |
|------|------|------|
| 工具被拦截 | 当前阶段不允许该工具 | 完成当前阶段 require 后自动解锁 |
| 阶段不推进 | require 未满足 | 调用 require 列表中的工具 |
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
- 新会话自动从 prepare 阶段开始
