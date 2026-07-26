# Preference Versioning 设计文档

## 概述

当前 PreferenceItem（创作偏好）存在两个问题：

1. **无去重**：AI 多次写入相同主题导致冗余条目。"主角性格：勇敢"和"主角性格：勇毅果断"并存
2. **无版本**：内容更新后旧内容丢失，无法追溯"之前是怎么写的"
3. **无状态**：没有"启用/草稿"标记，所有偏好都生效，互相矛盾时 AI 不知道信哪个

## 改动范围

最小化改动，不重构整个 PreferenceItem。

### 1. 新增字段

```go
type PreferenceItem struct {
    // ... 现有字段保持不变 ...
    Status  string `gorm:"column:status;default:'active'" json:"status"` // active(生效) | draft(草稿) | superseded(已替代)
    Version int    `gorm:"column:version;default:1"       json:"version"`
    // DeprecatedAt *time.Time 不引入。替代方案见下文。
}
```

### 2. 去重逻辑（在 MCP 工具层，不在 SQL 层）

`create_preference` 在执行前做三步检查：

```
① 按 (novel_id, category) 分组，找同分类现有条目
② 如果现有条目 content 与新内容的语义相似度 > 阈值 → 执行 update_preference（PATCH 方式追加/合并）
③ 如果不相似 → 正常 INSERT
```

关键设计：**去重在 Go 工具代码中手写逻辑，不在 DB 层加唯一约束。** 理由：

- 语义相似无法用 SQL 表达
- 约束太严格（同 category 就应该唯一？不是，同 category 可以有多个不同偏好）
- MCP 工具在创建前先 list 一下同 category 的现有条目，由 LLM 自己决定是创建新条目还是更新旧条目

实际上，**Layer 2 的去重不需要 Go 代码做语义判断**。让 LLM 自己决策——工具 `create_preference` 返回"同 category 已有 N 条，是否确认创建？"由 LLM 决定是否继续。

更简单的做法：`create_preference` 执行前自动查询同 category 条目，追加到提示中：

```
"当前「写作风格」分类已有以下偏好：
  [id:3] 不要虐主
  [id:7] 主角智商在线
是否仍要创建新条目？"
```

LLM 看到后会自行决定——合并已有条目（update）或确实需要新条目（create）。

### 3. 版本 / superseded 不追溯

旧版本不保留完整历史（避免 `preference_versions` 表的膨胀）。替代方案：

- `version` 递增记录更新次数
- `status = superseded` 标记被替代的条目（agent 弃用时标记，不清除）
- 查询默认 `WHERE status = 'active'`，被替代的条目不显示

如果用户想要完整的变更历史，通过 Git 提交记录追溯（偏好变化会触发 Git commit）。

### 4. MCP 工具变化

`create_preference`：返回同 category 现有条目数量，LLM 可据此选择创建或更新

`update_preference`：自动 version +1

`get_preferences`：新增 status 过滤参数（`all` / `active` / `superseded`）

无新增工具。

## 实现路线

1. 数据库 migration：新增 status + version 字段
2. `create_preference` 中增加同 category 条目查询返回
3. `update_preference` 自动递增 version
4. `get_preferences` 增加 status 过滤
