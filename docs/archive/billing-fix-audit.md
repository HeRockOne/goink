# 计费面板修复审计报告

> 日期：2026-07-30
> 构建：成功

## 修改汇总

### 1. `internal/llm/stream.go`
- **已恢复原始代码**：移除本地未提交的 `hasFinish` 检查修改，恢复到 977a7eb 版本的原始逻辑

### 2. `internal/agent/tokens.go`
- **缓存字段优先级调整**：优先 OpenAI 标准格式 `prompt_tokens_details.cached_tokens`，fallback DeepSeek 格式
- **新增 `acc_completion_tokens`**：session 级累计输出 token
- **字段对齐**：同时发送 `running_tokens`（角色成本）和 `detail`（角色显示）
- **新增每消息持久化**：每次 EventUsage 将 API 返回的精确 usage 写入当前 assistant 消息的 ExtraMetadata

### 3. `internal/session/store.go`
- **新增 `UpdateMessageUsage`**：保存 API usage 到消息的 ExtraMetadata.usage
- **新增 `GetSessionCumulativeUsage`**：从消息历史 SUM 计算 session 累计 token

### 4. `frontend/src/components/chat/ContextRing.tsx`
- 输出成本改用 `usage.acc_completion_tokens`（累计值）代替 `usage.completion_tokens`（单次值）

## 数据流

```
LLM API → stream.go parseSSE → agent.go EventUsage 
  → tokens.go updateUsage
      ├─ 累加 accHit/accMiss/accCompletion (内存累计)
      ├─ 保存到 message.ExtraMetadata.usage (DB持久化)
      ├─ 更新 session.Usage (DB持久化)
      └─ wails 推送前端
```

## 对账能力

每条 assistant 消息的 `ExtraMetadata` 中记录 API 原始 usage：
```json
"usage": {
  "prompt_tokens": 12000,
  "completion_tokens": 300,
  "total_tokens": 12300,
  "cached_tokens": 9000
}
```
可逐消息对账。
