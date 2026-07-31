# 功能新增审计报告

> 日期：2026-07-30
> 构建：成功

## 修改清单

### 1. 输入框默认提示（ChatPanel.tsx）
- **文件**：`frontend/src/components/chat/ChatPanel.tsx`
- **改动**：空状态（无会话、无小说）时显示 4 张提示卡片，引导用户输入
- **文件**：`frontend/src/i18n/locales/zh-CN.json` 和 `en.json`
- **改动**：新增 `welcomeTitle`、`welcomeDesc`、`hintWrite` 等 8 个 i18n key

### 2. Token 月度趋势图（ProfileView）
- **文件**：`app/profile.go`
- **改动**：新增 `GetTokenUsageTrend(days int)` 方法，从 `message.ExtraMetadata.usage` 查询每日 token 消耗，价格从用户设置读取
- **文件**：`frontend/src/components/profile/ProfileView.tsx`
- **改动**：新增 Token 趋势区域，包含 4 个汇总卡片 + 近 7 日堆叠柱状图（缓存命中/未命中/输出）

### 3. DOCX 导出
- **文件**：`internal/export/docx.go`
- **改动**：纯标准库实现（`archive/zip`+ XML），无外部依赖
- **文件**：`internal/export/export.go`
- **改动**：注册 `"docx"` 格式
- **文件**：`frontend/src/components/export/ExportDialog.tsx`
- **改动**：新增 DOCX 选项，设为默认格式

## 数据规范

### Token 趋势图数据源
```
message.ExtraMetadata.usage → 按 created_at 分组 → 每日累计
```
字段：`prompt_tokens`、`completion_tokens`、`cached_tokens`

### 价格来源
优先用户设置的 `app_config` 表 `price_input`/`price_output`/`cache_price`，无设置时使用默认值。

## 注意事项
- ONNX Runtime 缺失是预期行为，不影响新增功能
- Token 趋势图无历史数据时不显示（首次使用需积累数据）
