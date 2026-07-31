# Token 计费面板测试报告

> 日期：2026-07-30
> 测试者：API 端点
> 模型：商汤 / sensenova-6.7-flash-lite
> 小说：test（ID: 11）

---

## 一、测试流程

### 1. 新建对话
发送：
```
POST /api/chat
{
  "message": "这个世界的修炼体系是什么",
  "novel_id": 11,
  "provider": "商汤",
  "model": "sensenova-6.7-flash-lite"
}
```

### 2. 同 session 多轮追问
```
Turn 2: "故事的主角是谁，他有什么背景"
Turn 3: "时间线是怎么发展的，有哪些重要事件"
Turn 4: "有哪些重要的地点和势力"
```

### 3. 验证手段
- 日志 `usage 推送` 查看每次 EventUsage 的累计值
- API `GET /api/sessions` 查看 DB 存储的 session.Usage

---

## 二、测试结果

### 2.1 后端累计日志

```
Turn 1 - LLM调用1: accComp=  93  perModel.comp=  93  hit= 20480  miss= 1262
Turn 1 - LLM调用2: accComp= 153  perModel.comp= 153  hit= 40960  miss= 2713
Turn 1 - LLM调用3: accComp= 481  perModel.comp= 481  hit= 61440  miss= 4597
                                   ────────
Turn 2 - LLM调用1: accComp= 557  perModel.comp= 557  hit= 81920  miss= 6815
Turn 2 - LLM调用2: accComp= 872  perModel.comp= 872  hit=102400  miss= 9386
                                   ────────
Turn 3 - LLM调用1: accComp= 952  perModel.comp= 952  hit=122880  miss=12265
Turn 3 - LLM调用2: accComp=1447  perModel.comp=1447  hit=143360  miss=16023
                                   ────────
Turn 4 - LLM调用1: accComp=1545  perModel.comp=1545  hit=163840  miss=20264
Turn 4 - LLM调用2: accComp=1601  perModel.comp=1601  hit=184320  miss=25096
Turn 4 - LLM调用3: accComp=2058  perModel.comp=2058  hit=208896  miss=25896
```

**结论：每轮递增，无重置** ✅

### 2.2 DB 存储一致性

从 `GET /api/sessions` 获取的 `session.Usage`：

```json
{
  "acc_completion_tokens": 2058,
  "prompt_cache_hit_tokens": 208896,
  "prompt_cache_miss_tokens": 25896,
  "per_model": {
    "sensenova-6.7-flash-lite": {
      "comp": 2058,
      "hit": 208896,
      "miss": 25896
    }
  }
}
```

| 对比项 | 全局累计 | per_model | 一致？ |
|--------|---------|-----------|-------|
| 输出 token | `acc_completion_tokens` = 2058 | `comp` = 2058 | ✅ |
| 缓存命中 | `prompt_cache_hit_tokens` = 208896 | `hit` = 208896 | ✅ |
| 未命中 | `prompt_cache_miss_tokens` = 25896 | `miss` = 25896 | ✅ |

**结论：全局与按模型数据一致** ✅

### 2.3 成本验证

面板使用用户设置价格（默认）：
- 缓存命中价格：¥0.27 / M
- 输入价格：¥1.35 / M
- 输出价格：¥8.1 / M

```
hitCost  = 208896 × 0.27 / 1M = ¥0.0564
missCost = 25896 × 1.35 / 1M  = ¥0.0350
outCost  = 2058  × 8.1  / 1M  = ¥0.0167
totalCost                      = ¥0.1081
```

---

## 三、缓存命中率

| Turn | 缓存命中 | 未命中 | 命中率 |
|------|---------|--------|--------|
| 1    | 61,440  | 4,597  | 93.0% |
| 2    | 102,400 | 9,386  | 91.6% |
| 3    | 143,360 | 16,023 | 89.9% |
| 4    | 208,896 | 25,896 | 89.0% |

首轮后命中率稳定在 89-93%，符合 DeepSeek/Sensenova 缓存特性。

---

## 四、修正的 Bug

| Bug | 根因 | 修复 |
|-----|------|------|
| per_model 双倍累加 | `m["hit"] += hitTokens` 写了两次 | `tokens.go` 删除旧重复行 |
| UpsertModelUsage 传累计值 | 传入 `m["hit"]`（累计）而不是 `hitTokens`（增量） | 改为传 delta 值 |
| 面板 fallback 到全局合计 | `selectedModel` 没有 per_model 数据时回退到 `acc_completion_tokens` | 改为显示 0 |

---

## 五、测试结论

**后端累计逻辑正确**，`acc_completion_tokens` 和 `per_model.comp` 始终保持一致，多轮对话无重置。
