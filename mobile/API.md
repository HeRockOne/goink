# Goink Mobile API 端点文档

> 基础地址: `http（s）://{IP}:{PORT}` (默认端口 8877)
> 所有响应 Content-Type: `application/json`，除 `/api/chat` 返回 SSE 流。
> CORS: 允许所有来源 (`Access-Control-Allow-Origin: *`)。

---

## 认证

所有 API 请求（除豁免路径外）需要携带认证令牌。

**获取令牌：** 桌面端「设置 → API 认证令牌」中查看或重置。

**方式一：HTTP Header（推荐）**
```
Authorization: Bearer a95f2e1b78b0c01408bc32a477633c3e（当前）
```

**方式二：Query 参数**
```
?token=<token>
```

**豁免路径（无需认证）：**
- `GET /api/health` — 健康检查
- `/mobile/*` — 移动端静态文件

**未认证响应：**
```json
{ "error": "unauthorized" }
```
HTTP 状态码: `401`

---

## 目录

1. [认证](#认证)
2. [系统](#1-系统)
3. [小说](#2-小说)
4. [章节](#3-章节)
5. [角色](#4-角色)
6. [时间线](#5-时间线)
7. [弧线](#6-弧线)
8. [弧线节点](#7-弧线节点)
9. [读者认知](#8-读者认知)
10. [偏好](#9-偏好)
11. [地点](#10-地点)
12. [世界观设定](#11-世界观设定)
13. [物品](#12-物品)
14. [场景](#13-场景)
15. [创作统计](#14-创作统计)
16. [写作快照](#15-写作快照)
17. [对话](#16-对话)
18. [角色关系](#18-角色关系)
19. [地点关系](#19-地点关系)
20. [物品出现记录](#20-物品出现记录)
21. [门禁配置](#21-门禁配置)
22. [书写上下文](#22-书写上下文)
23. [语义搜索](#23-语义搜索)
24. [读取文件](#24-读取文件)
25. [会话](#25-会话)
26. [模型设置](#26-模型设置)
27. [WebSocket](#27-websocket)
28. [静态文件](#28-静态文件)

---

## 1. 系统

### GET /api/health

健康检查。

**响应:**
```json
{ "status": "ok" }
```

### GET /api/info

服务器信息。

**响应:**
```json
{
  "ip": "192.168.1.100",
  "port": 8877,
  "url": "http://192.168.1.100:8877"
}
```

---

## 2. 小说

### GET /api/novels

获取所有小说列表。

**响应:**
```json
{
  "novels": [
    {
      "id": 1,
      "title": "小说标题",
      "description": "简介",
      "genre": "玄幻",
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-07-01T00:00:00Z"
    }
  ],
  "total": 1
}
```

---

## 3. 章节

### GET /api/novels/{novel_id}/chapters

获取指定小说的章节列表，按章节号降序排列。

**路径参数:**
| 参数 | 类型 | 说明 |
|------|------|------|
| novel_id | int | 小说 ID |

**查询参数:**
| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| page | int | 1 | 页码 |
| size | int | 9999 | 每页数量 |

**响应:**
```json
{
  "chapters": [
    {
      "id": 101,
      "novel_id": 1,
      "chapter_number": 75,
      "title": "章节标题",
      "word_count": 3500,
      "created_at": "2026-07-01T00:00:00Z",
      "updated_at": "2026-07-15T00:00:00Z"
    }
  ],
  "total": 75
}
```

### GET /api/chapters/{chapter_id}

获取单个章节的完整内容（含 Markdown 正文）。

**路径参数:**
| 参数 | 类型 | 说明 |
|------|------|------|
| chapter_id | int | 章节 ID |

**响应:**
```json
{
  "id": 101,
  "chapter_number": 75,
  "title": "章节标题",
  "word_count": 3500,
  "content": "# 第七十五章\n\n正文内容...",
  "file_path": "/home/user/Goink/novels/1/chapters/075.md"
}
```

---

## 4. 角色

### GET /api/characters

获取指定小说的所有角色。

**查询参数:**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| novel_id | int | 是 | 小说 ID |

**响应:**
```json
{
  "characters": [
    {
      "id": 1,
      "novel_id": 1,
      "name": "角色名",
      "role": "主角",
      "personality": "{\"勇敢\": \"true\", \"聪明\": \"true\"}",
      "background": "角色背景故事",
      "created_at": "2026-01-01T00:00:00Z"
    }
  ],
  "total": 1
}
```

**字段说明:**
| 字段 | 类型 | 说明 |
|------|------|------|
| name | string | 角色名称 |
| role | string | 角色定位（主角/配角/反派等） |
| personality | string | JSON 格式的性格特征键值对 |
| background | string | 角色背景 |

---

## 5. 时间线

### GET /api/timeline

获取指定小说的时间线条目（伏笔/用户指令）。

**查询参数:**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| novel_id | int | 是 | 小说 ID |
| page | int | 否 | 页码（默认 1） |
| size | int | 否 | 每页数量（默认 9999） |

**响应:**
```json
{
  "entries": {
    "items": [
      {
        "id": 1,
        "novel_id": 1,
        "title": "伏笔标题",
        "category": "伏笔",
        "status": "pending",
        "target_chapter": 50,
        "source_chapter_id": 10,
        "resolved_chapter_id": 0,
        "importance": 4,
        "source": "用户指令",
        "content": "详细内容描述",
        "detail_json": "{\"key\": \"value\"}",
        "created_at": "2026-01-01T00:00:00Z"
      }
    ],
    "total": 1
  }
}
```

**字段说明:**
| 字段 | 类型 | 说明 |
|------|------|------|
| title | string | 伏笔/事件标题 |
| category | string | 分类 |
| status | string | 状态: `resolved`(已解决) / `pending`(待处理) / 其他 |
| target_chapter | int | 目标章节号 |
| source_chapter_id | int | 来源章节 ID |
| resolved_chapter_id | int | 解决章节 ID |
| importance | int | 重要度 (1-5，★表示) |
| source | string | 来源（用户指令/AI 生成等） |
| content | string | 详细内容 |
| detail_json | string | JSON 格式的扩展详情 |

---

## 6. 弧线

### GET /api/arcs

获取指定小说的故事弧线。

**查询参数:**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| novel_id | int | 是 | 小说 ID |
| page | int | 否 | 页码（默认 1） |
| size | int | 否 | 每页数量（默认 9999） |

**响应:**
```json
{
  "arcs": {
    "items": [
      {
        "id": 1,
        "novel_id": 1,
        "name": "弧线名称",
        "arc_type": "主线",
        "status": "active",
        "description": "弧线描述",
        "created_at": "2026-01-01T00:00:00Z"
      }
    ],
    "total": 1
  }
}
```

**字段说明:**
| 字段 | 类型 | 说明 |
|------|------|------|
| name | string | 弧线名称 |
| arc_type | string | 弧线类型（主线/支线/暗线等） |
| status | string | 状态: `active` / `completed` / `paused` |
| description | string | 弧线描述 |

---

## 7. 弧线节点

### GET /api/arc-nodes

获取指定小说的所有弧线节点（章节范围 0-9999）。

**查询参数:**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| novel_id | int | 是 | 小说 ID |

**响应:**
```json
{
  "nodes": [
    {
      "id": 1,
      "novel_id": 1,
      "story_arc_id": 1,
      "title": "节点标题",
      "description": "节点描述",
      "target_chapter": 20,
      "actual_chapter": 22,
      "status": "completed",
      "created_at": "2026-01-01T00:00:00Z"
    }
  ],
  "total": 5
}
```

**字段说明:**
| 字段 | 类型 | 说明 |
|------|------|------|
| story_arc_id | int | 所属弧线 ID |
| title | string | 节点标题 |
| description | string | 节点描述 |
| target_chapter | int | 目标章节号 |
| actual_chapter | int | 实际完成章节号 |
| status | string | 状态: `completed` / `pending` |

---

## 8. 读者认知

### GET /api/reader

获取指定小说的读者视角条目。

**查询参数:**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| novel_id | int | 是 | 小说 ID |
| page | int | 否 | 页码（默认 1） |
| size | int | 否 | 每页数量（默认 9999） |

**响应:**
```json
{
  "entries": {
    "items": [
      {
        "id": 1,
        "novel_id": 1,
        "type": "known",
        "content": "读者已知的信息",
        "planted_chapter": 5,
        "revealed_chapter": 20,
        "related_truth": "关联真相",
        "created_at": "2026-01-01T00:00:00Z"
      }
    ],
    "total": 1
  }
}
```

**字段说明:**
| 字段 | 类型 | 说明 |
|------|------|------|
| type | string | 类型: `known`(已知信息) / `suspense`(悬念) / `misconception`(误解) |
| content | string | 内容描述 |
| planted_chapter | int | 埋设章节号 |
| revealed_chapter | int | 揭示章节号 |
| related_truth | string | 关联真相 |

---

## 9. 偏好

### GET /api/preferences

获取指定小说的创作偏好。

**查询参数:**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| novel_id | int | 是 | 小说 ID |
| page | int | 否 | 页码（默认 1） |
| size | int | 否 | 每页数量（默认 500） |

**响应:**
```json
{
  "preferences": [
    {
      "id": 1,
      "novel_id": 1,
      "is_global": false,
      "category": "写作风格",
      "content": "偏好内容描述"
    }
  ]
}
```

**字段说明:**
| 字段 | 类型 | 说明 |
|------|------|------|
| is_global | bool | `true`=全局偏好，`false`=小说专属 |
| category | string | 偏好分类（自由文本） |
| content | string | 偏好内容 |

---

## 10. 地点

### GET /api/locations

获取指定小说的所有地点。

**查询参数:**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| novel_id | int | 是 | 小说 ID |
| page | int | 否 | 页码（默认 1） |
| size | int | 否 | 每页数量（默认 500） |

**响应:**
```json
{
  "locations": [
    {
      "id": 1,
      "novel_id": 1,
      "name": "地点名称",
      "location_type": "城市",
      "description": "地点描述",
      "detail_json": "{\"population\": \"100万\"}",
      "tags": "繁华,现代",
      "created_at": "2026-01-01T00:00:00Z"
    }
  ]
}
```

**字段说明:**
| 字段 | 类型 | 说明 |
|------|------|------|
| name | string | 地点名称 |
| location_type | string | 地点类型（城市/建筑/自然等） |
| description | string | 地点描述 |
| detail_json | string | JSON 格式的扩展详情 |
| tags | string | 逗号分隔的标签 |

---

## 11. 世界观设定

### GET /api/lore

获取指定小说的世界观设定条目，支持按分类和关键词筛选。

**查询参数:**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| novel_id | int | 是 | 小说 ID |
| category | string | 否 | 分类筛选（力量体系/社会构成/历史事件等） |
| search | string | 否 | 全文搜索关键词 |
| page | int | 否 | 页码（默认 1） |
| size | int | 否 | 每页数量（默认 9999） |

**响应:**
```json
{
  "lore": [
    {
      "id": 1,
      "novel_id": 1,
      "title": "灵脉修炼体系",
      "category": "力量体系",
      "summary": "以灵脉为核心的能量修炼体系",
      "content": "灵脉是天地灵气凝聚而成的能量脉络...",
      "arc_id": 1,
      "reveal_chapter_id": 5,
      "is_public": true,
      "source": "",
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-07-01T00:00:00Z"
    }
  ],
  "total": 1
}
```

**字段说明:**
| 字段 | 类型 | 说明 |
|------|------|------|
| title | string | 设定标题 |
| category | string | 分类 |
| summary | string | 一句话摘要 |
| content | string | 正文内容（Markdown） |
| arc_id | int | 关联的故事弧线 ID（可选） |
| reveal_chapter_id | int | 首次向读者揭露此设定的章节 ID |
| is_public | bool | 是否为公开设定（false 表示仅角色知晓） |
| source | string | 来源说明 |

---

## 12. 物品

### GET /api/items

获取指定小说的物品/法宝列表。

**查询参数:**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| novel_id | int | 是 | 小说 ID |
| type | string | 否 | 物品类型（法宝/丹药/灵药/功法等） |
| status | string | 否 | 状态筛选（active/destroyed/lost等） |
| search | string | 否 | 关键词搜索 |
| page | int | 否 | 页码（默认 1） |
| size | int | 否 | 每页数量（默认 9999） |

**响应:**
```json
{
  "items": [
    {
      "id": 1,
      "novel_id": 1,
      "name": "灵脉玉佩",
      "item_type": "法宝",
      "grade": "天阶",
      "description": "温润的玉佩...",
      "ability": "储存灵气，缓慢恢复持有者法力",
      "lore": "传说为上古大能所炼制",
      "owner_id": 127,
      "status": "active",
      "arc_id": 1,
      "narrative_role": "key_prop",
      "first_chapter_id": 1,
      "status_changed_chapter_id": 15,
      "created_at": "2026-01-01T00:00:00Z"
    }
  ],
  "total": 1
}
```

**字段说明:**
| 字段 | 类型 | 说明 |
|------|------|------|
| name | string | 物品名称 |
| item_type | string | 物品类型 |
| grade | string | 品级 |
| description | string | 外观/功能描述 |
| ability | string | 特殊能力 |
| lore | string | 来历/传说 |
| owner_id | int | 当前持有角色 ID |
| status | string | 状态: `active` / `destroyed` / `lost` |
| arc_id | int | 关联弧线 ID |
| narrative_role | string | 叙事重要性: `key_prop` / `supporting` / `normal` |
| first_chapter_id | int | 首次出现章节 ID |
| status_changed_chapter_id | int | 状态变化章节 ID |

---

## 13. 场景

### GET /api/scenes

获取指定小说/章节的场景列表。

**查询参数:**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| novel_id | int | 是 | 小说 ID |
| chapter_id | int | 否 | 章节 ID（留空返回所有场景） |

**响应:**
```json
{
  "scenes": [
    {
      "id": 1,
      "novel_id": 1,
      "chapter_id": 101,
      "title": "炎羽部落初遇",
      "content": "夜幕降临...",
      "character_ids": "[1,2,3]",
      "location_id": 5,
      "arc_id": 1,
      "arc_node_id": 3,
      "created_at": "2026-01-01T00:00:00Z"
    }
  ]
}
```

**字段说明:**
| 字段 | 类型 | 说明 |
|------|------|------|
| title | string | 场景标题 |
| content | string | 场景内容 |
| chapter_id | int | 所属章节 ID |
| character_ids | string | JSON 数组格式的角色 ID 列表 |
| location_id | int | 地点 ID |
| arc_id | int | 关联弧线 ID |
| arc_node_id | int | 关联弧线节点 ID |

---

## 14. 创作统计

### GET /api/stats

获取指定小说的综合统计数据。

**查询参数:**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| novel_id | int | 是 | 小说 ID |

**响应:**
```json
{
  "stats": {
    "total_chapters": 75,
    "total_words": 350000,
    "avg_chapter_words": 4667,
    "arc_count": 3,
    "arc_completed": 1,
    "foreshadowing_total": 15,
    "foreshadowing_resolved": 8,
    "character_count": 12,
    "location_count": 25,
    "latest_chapter_num": 75,
    "latest_chapter_title": "第七十五章 真相大白"
  }
}
```

**字段说明:**
| 字段 | 类型 | 说明 |
|------|------|------|
| total_chapters | int | 总章节数 |
| total_words | int | 总字数 |
| avg_chapter_words | int | 平均每章字数 |
| arc_count | int | 弧线总数 |
| arc_completed | int | 已完成的弧线数 |
| foreshadowing_total | int | 伏笔总数 |
| foreshadowing_resolved | int | 已回收的伏笔数 |
| character_count | int | 角色数 |
| location_count | int | 地点数 |
| latest_chapter_num | int | 最新章节号 |
| latest_chapter_title | string | 最新章节标题 |

---

## 15. 写作快照

### GET /api/writing-snapshot

获取当前写作进度快照。

**查询参数:**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| novel_id | int | 是 | 小说 ID |

**响应:**
```json
{
  "writing_snapshot": {
    "id": 1,
    "novel_id": 1,
    "last_chapter_id": 101,
    "current_arc_id": 2,
    "current_location_id": 5,
    "current_location_text": "青云宗后山",
    "active_chars": "[1,2,3]",
    "updated_at": "2026-07-15T00:00:00Z"
  }
}
```

**字段说明:**
| 字段 | 类型 | 说明 |
|------|------|------|
| last_chapter_id | int | 最新完成的章节 ID |
| current_arc_id | int | 当前正在推进的弧线 ID |
| current_location_id | int | 当前焦点地点 ID |
| current_location_text | string | 焦点地点文本（冗余） |
| active_chars | string | JSON 数组格式的活跃角色 ID 列表 |

---

## 16. 角色关系

### GET /api/character-relations

获取角色之间的当前有效关系图。

**查询参数:**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| novel_id | int | 是 | 小说 ID |

**响应:**
```json
{
  "relations": [
    {
      "id": 1,
      "novel_id": 11,
      "source_character_id": 127,
      "target_character_id": 128,
      "relation_describe": "同伴、暗中提防",
      "is_current": true,
      "created_at": "2026-07-01T00:00:00Z"
    }
  ]
}
```

---

## 17. 地点关系

### GET /api/location-relations

获取地点之间的空间连通关系。

**查询参数:**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| novel_id | int | 是 | 小说 ID |

**响应:**
```json
{
  "relations": [
    {
      "id": 1,
      "location_a_id": 5,
      "location_b_id": 8,
      "relation_type": "相邻",
      "description": "皇城南门通往皇宫偏殿"
    }
  ]
}
```

---

## 18. 物品出现记录

### GET /api/item-occurrences

获取物品在章节中的出现记录。支持按物品 ID 筛选或返回全部。

**查询参数:**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| novel_id | int | 是 | 小说 ID |
| item_id | int | 否 | 物品 ID（留空返回全部） |

**响应:**
```json
{
  "occurrences": [
    {
      "id": 1,
      "novel_id": 11,
      "item_id": 4,
      "chapter_id": 290,
      "action": "acquired",
      "description": "黎烨获得炽魂石",
      "created_at": "2026-07-01T00:00:00Z"
    }
  ]
}
```

---

## 19. 门禁配置

### GET /api/phase-gate-config

获取当前阶段门禁配置和开关状态。

**响应:**
```json
{
  "config": "<!-- phase-gate-config\nmode: single\nphase: prepare\ntools: ...",
  "enabled": true
}
```

---

## 20. 书写上下文

### GET /api/writing-context

获取树状关联的当前故事状态，用于 prepare 阶段替代多次 get_* 调用。

**查询参数:**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| novel_id | int | 是 | 小说 ID |
| current_chapter | int | 是 | 当前要写的章节号 |

**响应:**
```json
{
  "chapter": { "num": 3, "title": "第3章", "word_count": 2308 },
  "recent_chapters": [
    { "num": 3, "title": "第3章", "summary": "", "key_events": "", "word_cnt": 23 },
    { "num": 2, "title": "荒原暗影", "summary": "三人前往星痕山脉途中...", "key_events": ["炽魂石出现裂纹"], "word_cnt": 3626 }
  ],
  "characters": [
    { "id": 127, "name": "黎烨", "location": { "id": 67, "name": "山脚村落" }, "item_count": 3 }
  ],
  "active_arcs": [
    { "id": 68, "name": "灵脉拯救之路", "type_zh": "主线", "nodes_total": 2, "nodes_done": 2 }
  ],
  "timeline": {
    "pending": [{ "id": 197, "title": "炽魂石的共鸣", "target_chapter": 3, "importance": 4 }],
    "resolved": []
  },
  "reader": { "known": 1, "suspense": 3, "misconception": 0 },
  "writing_snapshot": { "last_chapter_num": 2, "current_arc_id": 68, "active_chars": "[127,128,129]" },
  "stats": { "total_chapters": 3 }
}
```

---

## 21. 语义搜索

### GET /api/search-memory

使用语义搜索在小说内容中查找相关信息。

**查询参数:**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| novel_id | int | 是 | 小说 ID |
| query | string | 是 | 自然语言搜索关键词 |

**响应:**
```json
{
  "results": [
    { "type": "lore", "title": "上古大战与星痕山脉", "relevance": 0 },
    { "type": "rag", "title": "第 2 章", "relevance": 0.57 }
  ]
}
```

---

## 22. 读取文件

### GET /api/read

读取小说仓库中的文件内容（章节正文、大纲、故事状态等）。

**查询参数:**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| novel_id | int | 是 | 小说 ID |
| path | string | 是 | 文件路径，如 `chapters/002.md`、`outlines/002.md`、`goink.md` |

**响应:**
```json
{
  "content": "荒原的风比昨夜更冷了。\n\n黎烨勒住马缰...",
  "path": "chapters/002.md"
}
```

---

## 23. 对话

### POST /api/chat

发送消息并获取 AI 回复（SSE 流式响应）。

> 需要认证令牌。

**请求体:**
```json
{
  "message": "你好，请帮我写一段开头",
  "novel_id": 1,
  "session_id": "可选，续接已有会话",
  "model": "可选，模型ID",
  "provider": "可选，提供商名称"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| message | string | 是 | 用户消息 |
| novel_id | int | 是 | 小说 ID |
| session_id | string | 否 | 会话 ID（留空则创建新会话） |
| model | string | 否 | 模型 ID（如 `gpt-4`） |
| provider | string | 否 | 提供商名称（如 `openai`） |

**响应:** `text/event-stream`

SSE 事件格式（每行 `data: {...}\n\n`）:

| event.type | 说明 | data 字段 |
|------------|------|-----------|
| `started` | 会话已创建 | `session_id`: 新会话 ID |
| `thinking` | AI 思考中 | `data`: 思考内容片段 |
| `content` | AI 回复内容 | `data`: 内容片段 |
| `done` | 回复完成 | `text`: 完整回复文本 |
| `error` | 出错 | `error`: 错误信息 |

**示例 SSE 流:**
```
data: {"type":"started","session_id":"abc123"}

data: {"type":"thinking","data":"让我想想..."}

data: {"type":"content","data":"好的，"}

data: {"type":"content","data":"我来帮你写。"}

data: {"type":"done","text":"好的，我来帮你写。"}
```

### POST /api/chat/cancel

取消正在进行的对话。

**请求体:**
```json
{
  "session_id": "abc123"
}
```

**响应:**
```json
{ "ok": true }
```

---

## 24. 会话

### GET /api/sessions

获取指定小说的会话列表（分页）。

**查询参数:**
| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| novel_id | int | 必填 | 小说 ID |
| page | int | 1 | 页码 |
| size | int | 50 | 每页数量 |

**响应:**
```json
{
  "items": [
    {
      "session_id": "abc123",
      "title": "会话标题",
      "current_phase": "写作",
      "created_at": "2026-07-01T00:00:00Z"
    }
  ],
  "total": 10
}
```

### GET /api/sessions/{session_id}/messages

获取指定会话的消息列表（仅 user 和 assistant 角色）。

**路径参数:**
| 参数 | 类型 | 说明 |
|------|------|------|
| session_id | string | 会话 ID |

**响应:**
```json
{
  "messages": [
    {
      "role": "user",
      "content": "用户消息",
      "thinking_content": "",
      "created_at": "2026-07-01T00:00:00Z"
    },
    {
      "role": "assistant",
      "content": "AI 回复",
      "thinking_content": "思考过程...",
      "created_at": "2026-07-01T00:00:01Z"
    }
  ]
}
```

---

## 25. 模型设置

### GET /api/settings/model

获取当前模型配置和可用模型列表。

**响应:**
```json
{
  "selected_model_key": "openai/gpt-4",
  "reasoning_effort": "medium",
  "models": [
    {
      "key": "openai/gpt-4",
      "name": "GPT-4",
      "provider": "openai",
      "thinking": false
    },
    {
      "key": "anthropic/claude-3-opus",
      "name": "Claude 3 Opus",
      "provider": "anthropic",
      "thinking": true
    }
  ]
}
```

**字段说明:**
| 字段 | 类型 | 说明 |
|------|------|------|
| selected_model_key | string | 当前选中的模型 key（格式: `provider/model_id`） |
| reasoning_effort | string | 推理力度 |
| models[].key | string | 模型唯一标识 |
| models[].name | string | 模型显示名称 |
| models[].provider | string | 提供商名称 |
| models[].thinking | bool | 是否支持思考模式 |

### POST /api/settings/model

切换模型。切换后通过 WebSocket 广播到所有连接的客户端。

**请求体:**
```json
{
  "model_key": "openai/gpt-4",
  "reasoning_effort": "medium"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| model_key | string | 是 | 目标模型 key |
| reasoning_effort | string | 否 | 推理力度 |

**响应:**
```json
{ "ok": true, "model_key": "openai/gpt-4" }
```

---

## 26. WebSocket

### WebSocket /api/ws

实时双向通信端点，用于桌面端和移动端之间的状态同步。

**连接地址:** `ws://{IP}:{PORT}/api/ws?token=<token>`

> WebSocket 需要通过 query 参数传递 token，无法使用 HTTP Header。

**接收事件类型:**
| event.type | 说明 | 数据 |
|------------|------|------|
| `model_changed` | 模型已切换 | `{ model_key, reasoning_effort }` |
| `chat:done` | 对话完成 | - |

---

## 27. 静态文件

### GET /

桌面端 Web 前端（React SPA）。

### GET /mobile/

移动端 Web 前端。访问 `http://{IP}:{PORT}/mobile/` 即可打开移动端界面。

---

## 通用说明

### 错误响应

所有接口在出错时返回:
```json
{
  "error": "错误描述"
}
```
HTTP 状态码为 500（内部错误）或 405（方法不允许）。

### 分页响应格式

部分接口返回分页数据，格式为:
```json
{
  "items": [...],
  "total": 100
}
```
或（特定接口）:
```json
{
  "entries": {
    "items": [...],
    "total": 100
  }
}
```

### 数据提取规则

客户端提取列表数据时建议按以下优先级:
1. 直接检查顶层数组值键（如 `novels`、`characters`、`locations`、`lore`、`items`、`scenes`）
2. 检查嵌套 `items` 数组（如 `entries.items`、`arcs.items`）
3. 回退到空数组
