# StoryArc 设计文档

## 概述

StoryArc 是叙事弧线系统，处于 ChapterPlan（战术层）和 NovelCreativeProfile（整体大纲）之间的战略层。由两张表组成：`story_arcs`（弧线容器）和 `arc_nodes`（弧线内有序链节）。

## 结构

```
复仇弧线 (main):
  ○ 发现仇人身份  →  ○ 获得关键武器  →  ○ 首战失败  →  ○ 终复仇
    (第10章)          (第25章)           (第40章)       (第50章)

感情线 (sub):
  ○ 相遇  →  ○ 表白  →  ○ 修成正果
   (第15章)    (第30章)    (第60章)
```

弧线之间没有结构化边表——一个小说通常 3-5 条弧线，AI 读全貌即可理解弧线间关系，不需要图查询。

## 与 Python 版本的差异

### 表拆分（Python 1 张 → Go 2 张）

Python 将弧线和弧线内节点混在自然语言 description 中。Go 拆分为：
- **story_arcs**：弧线容器，名称/类型/状态
- **arc_nodes**：弧线内有序链节，承接 Python plot_node 的职责

### 字段精简

Python 11 字段 → Go story_arcs 8 字段。砍掉：
- `start_chapter` / `end_chapter`：从首末节点的 target_chapter 推导
- `extra_metadata`：MCP add 工具参数未暴露，死字段

### arc_type 和 status 保持约束枚举

arc_type 有明确叙事学含义（主线/支线/角色线/背景线），status 驱动活跃窗口过滤。均保持枚举，不同于 location_type 的自由文本设计。

## 与 TimelineEntry 的边界

| | ArcNode | TimelineEntry(foreshadowing) |
|---|---|---|
| 视角 | 作者大纲，"什么时候该发生什么" | 读者感知，"故事里埋了什么钩子" |
| 创建者 | 作者规划，写作前设定 | AI 写作过程中发现 |
| 消费方式 | 注入上下文，提醒 AI 节奏 | MCP 查询，按需检索 |
| 结构 | 弧线内有序链 | 全局无序列表 |

同一事件可能同时产生 ArcNode 和 TimelineEntry——"获得武器"是弧线里程碑，也是伏笔的回收。

## MCP 工具（4 个）

| 工具 | 功能 |
|------|------|
| `get_story_arcs` | 返回弧线列表，含按 sequence 排序的节点链 |
| `create_story_arc` | 新建弧线容器（name 必填） |
| `update_story_arc` | 更新弧线元数据含 status |
| `update_arc_node` | UPSERT 节点：不传 node_id → INSERT（sequence 自动取 max+1）；传 node_id → UPDATE。可标记 completed |

没有 delete 工具，后续统一提供。

## 排序规则

- **story_arcs 列表**：importance DESC, created_at ASC
- **arc_nodes 链**：sequence ASC（联合唯一约束保证同弧线内 sequence 不重复）
