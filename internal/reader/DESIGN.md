# Reader 设计文档

## 概述

ReaderPerspective 追踪读者认知状态，帮助 AI 控制信息揭露节奏。属于上下文注入 Layer 2（STABLE 层），每章写作前自动注入。

## 三种类型

| type | 含义 | 查询过滤 | 格式化段 |
|------|------|---------|---------|
| `known` | 读者已知的信息 | 全量返回 | 已知信息 |
| `suspense` | 读者等待解答的悬念 | revealed_chapter=0 | 活跃悬念 |
| `misconception` | 读者误以为的情况 | revealed_chapter=0 | 读者误知 |

## 与 Python 版本的差异

related_truth 从"仅 misconception 可用"扩展为所有类型可选。suspense 条目现在也能记录真实答案——"谁杀了村长"是悬念，作者心里应该有数。

其余字段直接平移，无删减。

## 注入格式

```
## 读者认知

### 已知信息
- 张三是个孤儿 [第3章起]
- 魔法学院位于北境 [第5章起]

### 活跃悬念
- 谁杀了村长？（第5章种下 → 真相：王大锤是凶手）
- 密室里有什么？（第8章种下）

### 读者误知
- 李四背叛了主角（第6章种下 → 实际：李四是卧底）
```

## MCP 工具（3 个）

| 工具 | 功能 |
|------|------|
| `get_reader_perspective` | 按 type 分段返回格式化文本 |
| `create_reader_perspective_entry` | 创建条目（type/content/planted_chapter/related_truth 可选） |
| `update_reader_perspective_entry` | 更新：last_mentioned_chapter、revealed_chapter、content、related_truth |
