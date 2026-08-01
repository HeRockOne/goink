# ADR-0001：Prompt Caching 消息前缀稳定化

> 状态：Accepted（2026-07-28，已实施并实测验证）
> 相关文档：`archive/prompt-caching-research.md`（调研）、`archive/billing-test-report.md`（实测）、`architecture/token-injection.md`（注入构成）

---

## 决策

系统提示词采用**稳定前缀 + 动态注入**结构：

1. `writeSystemMessages` 只写入稳定前缀（identity + always skills + skill catalog），写入 messages 表
2. **NovelState（goink.md）不写入稳定前缀**，改由 `chatImpl` 在每轮对话时动态追加到 user 消息之后
3. **工具定义全量发送**（`registry.OpenAI(nil)`），不按阶段裁剪，靠 `allowed_tools` 白名单做运行时限制
4. 工具按名称排序（`sort.Strings`），保证确定性顺序
5. 增加 `computePrefixHash` 前缀哈希检测，缓存失效时日志警告

## 背景

- DeepSeek/OpenAI 兼容提供商按请求前缀做 KV Cache，命中后按约 10% 计费
- 原实现把 NovelState 写入系统消息尾部，每轮内容变化会**破坏整个缓存前缀**，导致缓存失效
- 工具定义占注入 80%（12,924 tokens），是最大的 token 开销，但作为稳定前缀可被缓存

## 备选方案

| 方案 | 结论 | 原因 |
|------|------|------|
| 工具按阶段动态裁剪 | ❌ 拒绝 | 每轮 tools 数组变化会破坏缓存前缀，收益(-6K)被缓存损失抵消 |
| NovelState 保留在系统消息 | ❌ 拒绝 | 每轮变化破坏前缀，缓存失效 |
| 缓存断点标记（cache_control） | ❌ 暂缓 | Anthropic 专属，Goink 主要用 OpenAI 兼容格式 |

## 后果

**正面**：
- 首轮后缓存命中率 89-93%（实测，见 `archive/billing-test-report.md`）
- ~17,500 tokens 的工具+系统前缀在后续轮次按 10% 计费

**代价**：
- 每轮仍发送全量 57 个工具 JSON（12,924 tokens），未裁剪——这是为了缓存稳定刻意做的取舍
- 后续 AI 接手时容易误判"工具太多应该裁剪"，需先理解本 ADR 再做变更

## 关联

- 实测命中率数据：`archive/billing-test-report.md`
- 注入构成统计：`architecture/token-injection.md`
- 完整方案对比：`design/token-optimization-plan.md`
