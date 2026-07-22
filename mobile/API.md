# Goink Mobile API 端点文档

> 基础地址: `http://{IP}:{PORT}` (默认端口 8877)
> 所有响应 Content-Type: `application/json`，除 `/api/chat` 返回 SSE 流。
> CORS: 允许所有来源 (`Access-Control-Allow-Origin: *`)。

---

## 认证

所有 API 请求（除豁免路径外）需要携带认证令牌。

**获取令牌：** 桌面端「设置 → API 认证令牌」中查看或重置。

**方式一：HTTP Header（推荐）**
```
Authorization: Bearer <token>
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
12. [对话](#11-对话)
13. [会话](#12-会话)
14. [模型设置](#13-模型设置)
15. [WebSocket](#14-websocket)
16. [静态文件](#15-静态文件)

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

## 11. 对话

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

## 12. 会话

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

## 13. 模型设置

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

## 14. WebSocket

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

## 15. 静态文件

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
1. 直接检查顶层数组值键（如 `novels`、`characters`、`locations`）
2. 检查嵌套 `items` 数组（如 `entries.items`、`arcs.items`）
3. 回退到空数组
