# 缓存命中率分析：为什么平均 92%，能否更高

> 日期：2026-08-04（初版），2026-08-04（交叉比对修订）
> 依据：代码实读 + 实测数据（`archive/billing-test-report.md`）+ 官方一手资料交叉验证：
> - DeepSeek 官方 KV Cache 文档（api-docs.deepseek.com/guides/kv_cache/）
> - Anthropic 工程博客《Lessons from building Claude Code: Prompt caching is everything》(2026-04)
> - Claude Code 缓存运行机制 doc、Chat-deep DeepSeek 实测（99.79% 稳定前缀域名；改首行=0 命中）
> - arXiv 2601.06007 DeepResearch 跨厂商评测、dev.to Cursor/KV 复用分析
> 结论先行：**92% 已在本提供商机制下的高分区、符合预期；剩余 8% 主要由两个可实施问题贡献：跨轮字节连续性被 NovelState 打断（P1），以及压缩非 cache-safe（P2）。参照 Claude Code/Cursor 成熟做法可解。**

---

## 一、机制实证：DeepSeek 硬盘缓存到底是什么规则

官方文档（三段演示，非猜测）：

1. **每条缓存前缀是独立完整单元，后续请求必须完整匹配该单元才能命中**。请求只在两个位置落盘单元：**用户输入结束位置、模型输出结束位置**；另有两个补充路径：**公共前缀检测**（多请求共有的前缀会被单独落盘成一个单元）、**固定 token 间隔**（长输入/长输出按固定间隔截单元，官方未公布间隔，实测多为 64/128 的倍数）。
2. `prompt_tokens = prompt_cache_hit_tokens + prompt_cache_miss_tokens`，计费口径唯一。
3. **best-effort**，不保证 100%；命中前提是"整体单元已落盘"；缓存不使用自动清空（几小时到几天）。

对照实测（billing-test-report）重新解释我们的数据：

```

Turn 1  LLM调用1: hit= 20480  miss= 1262     → 每调用 hit 增量恒定 20480
Turn 1  LLM调用2: hit= 40960  miss= 2713     → miss 增量 1451 / 1884 / 2218 ...
Turn 4  LLM调用3: hit=208896  miss=25896      → hit = 20480 × 调用数
```

**每轮每调用的 hit 增量恒定 20480**，即命中对象只有一个固定长度的单元；其构成 ≈ 固定前缀（17.5K）+ 首轮 user + 首轮 NovelState。之后的所有历史（各轮 assistant 正文、工具结果）从未被再次命中，哪怕同轮内重复发送的字节完全相同。

Chat-deep 的独立实测（同一个提供商机制）是决定性佐证：

- 稳定长前缀连续 4 个请求：输入 23,602，命中 23,552 = **99.79%**（只有要追加的新内容 miss）
- 仅把"首行"改为每次不同，其余全部相同：5 个请求 0 命中。

结论：**该缓存的成败全在第一个字节分叉点。分叉点越靠后，命中率越高；任何靠前跳动，后面全灭。** 这也解释了首轮之后 89-93% 与"理想追加式会话 99%"之间的差——Goink 当前的分叉点不在"上一条用户消息"级，而是更早。

---

## 二、根因：NovelState 不落库，把字节分叉点卡在"每条 user 消息之后"

消息组装（`app/chat.go:200-212`）：

- 轮次 N 请求 = `[固定前缀][历史][userN][NovelState_N]`（NovelState 只追加内存、不落库，`chat.go:212`）
- 轮次 N+1 请求 = `[固定前缀][历史][userN][assistantN][toolN]...[userN+1][NovelState_{N+1}]`

前一条 user 消息后面的字节从 `[NovelState_N]` 跳变为 `[assistantN]`，于是：

1. 轮次 N 的 assistant 输出、工具结果（章节正文、查询结果，单轮几千 token）**永远不在已落盘单元内**，下一轮重新全额计费；
2. 内容在上一轮末尾已"被请求"过，但因 NovelState 占位下一轮消失，字节序列不连续，官方"模型输出结束位置"单元对不上，沦为 miss。

对照 Claude Code 的成熟做法（Anthropic 工程博客明确）：**正常轮次的缓存前缀 = 上一请求的全部内容，只有最新一轮是新的**（"On a normal turn, the prefix is the entire previous request and only the latest exchange is new"）。成熟编码器（Claude Code/Cursor）都是"边界随对话推进"，即历史的 assistant/tool 内容在下一轮命中，只有真正新增的一轮是新算的。**Goink 因为 NovelState 的"幽灵占位"，历史边界没有随对话推进，被固定死在第一轮之后。**

这是行业反复强调的头号错误模式："动态字段放中间，一处跳动毁全部"（ProjectDiscovery 7%→74-84%）。Goink 的 NovelState 每轮内就在末尾，**跨轮看它插在了字节流中间**。

---

## 三、92% 在业界属于什么水平

| 参照 | 命中率 | 说明 |
|------|--------|------|
| DeepSeek 官方示例（A+B → A+B+C） | 第 3 次起命中 | 前缀完整复用即可命中 |
| Chat-deep 实测·稳定前缀连续请求 | **99.79%** | 追加式增长 + 稳定前缀 |
| Chat-deep 实测·首行每次变化 | **0%** | 分叉点太靠前 = 全灭 |
| Claude Code（Anthropic 官方） | 未公开数字，但"对命中率实时告警，低了挂 SEV" | 把命中率当服务可用性守护 |
| Cursor（dev.to 实测会话） | ~99% 会话内输入 token 来自缓存 | 大稳定前缀 + 纯追加式会话 |
| 行业聚合（Vellum/Helicone） | 50-80% | 多数生产者的低分区 |
| **Goink 当前** | **89-93%（跨轮平均 ~92%）** | ③结构稳定前缀已命中，历史未追上去 |

结论：92% 不是"低"，是**卡在可再进一步的结构分叉点上**。真正的行业天花板（99%）要求的是"上轮请求的全部内容都是本轮的缓存前缀"，Goink 目前只做到了"固定前缀"这一层。

---

## 四、可落地的提升手段（按收益排序，均对齐成熟做法）

### P1：NovelState 快照落库到轮次末尾，恢复跨轮字节连续性（预期 +3~6pp，对齐 Claude Code "messages 追加式"）

实现：每轮把注入的 NovelState 也写成 `role=system, to_api=true` 消息，放在**该轮 assistant/tool 追加之后**（即每轮末尾）。旧一轮的快照追加新快照时置 `to_api=false`（消息 append-only 不变，只是可见性切换，已有机制支持）。

```
轮次 N:   [前缀][历史][userN][assistantN][toolN][NS_N]
轮次 N+1: [前缀][历史][userN][assistantN][toolN][NS_N][userN+1][NS_{N+1}]
            ↑↑↑ ↑ 与上一轮完整请求逐字节相同 = 变成缓存单元 → 全部命中 ↑↑↑↑↑
```

- 效果：上一轮的章节正文、工具结果进入缓存前缀，命中率从 89-93% 区间进入 93-99% 区间（Chat-deep 4 连测已是 99.79% 的形态），长会话省的是大头。
- 代价：多一条旧 NS 快照常驻上下文（1-3K token），用 `to_api=false` 换新抑制膨胀。
- 质量核对：模型看到的 token 总量基本不变、不删任何创作规则，不碰红线。
- 联动：**P2 必须同步修**（详见下），否则压缩会把 NS 写回系统区、又造出双份 NS。

### P2：压缩把 NovelState 写进系统区 → 双份 NS + 前缀污染（真实 Bug，随 P1 同改）

`persistCompression`（`compress.go:249-254`）在重建新版本系统消息时**把 novelState 一并写入 `role=system` 靠前位置**，而 `chat.go` 每轮又在末尾动态追加一份 —— 导致**压缩后上下文同时存在两份 NovelState**，且系统区那份混入"稳定前缀区"，会拖累历史前缀、与 P1 的"NS 只放末尾"方向冲突。

修复：压缩重建新版本时，NovelState **不再写系统区**，改为遵循与 P1 一致的协议（每轮末尾、随轮次落库、旧快照 to_api=false）。这是 P1 的伴随修复，二者必须同改，否则压缩后前缀协议不一致。

### P3：压缩摘要请求已 cache-safe，无需改（核对修正 2026-08-04）

复核 `internal/agent/compress.go:44-64` `generateSummary`：压缩摘要请求 = **父会话完整 system + 完整历史 + 压缩指令作为最后一条 user 消息**（`msgs[len] = {role:user, content:compressionPrompt}`），正是 Claude Code 的 cache-safe fork 形态。且 `GenerateText`（`llm/generate.go:27`）**不带 tools** —— 对纯摘要任务这是合理且更省的：工具定义根本不重发、不再占 token，同时完整复用历史前缀。保持现状即可，不需要"补发 tools"。

压缩后的全量 miss 点只发生在**压缩后新版本的首轮**（摘要替换了历史，前缀自然重建）——这是任何实现的必然成本，与 Claude Code 一致（"压缩那轮 miss 后恢复"），无需优化。

### P4：把命中率当可用性守护（对齐 Claude Code "低于阈值挂告警"）

Goink 已有 `computePrefixHash` 告警日志和 `cache_hit_ratio` 字段，但无阈值告警。建议：
- session 级 `cache_hit_ratio` 连续多轮 < 某阈值（如 75%）且会话较长时，日志 Error/前端提示，定位前缀回归（换模型、改 skill、压缩异常等）。
- 把写码文档中"89-93%"的指标迁移到实测 telemetry，以它为基线设告警，防止回归。

### P5：运营纪律（零代码，防回退）

- **会话中途不换 provider/model / 不动 reasoning_effort**：DeepSeek 缓存与模型绑定；Claude Code 明确 effort 变更 = 新缓存键，全量重算。Goink 前端文案已提示换模型。
- **TTL 窗口**：DeepSeek 官方缓存存活几小时到几天（非 Claude 的 5min），写一章后隔几小时返回，TTL 大概率仍存活；若命中率骤降再检查间隔。
- **子任务走子 Agent**：Claude Code 用子 Agent 隔离上下文防前缀膨胀。Goink 的 `run_subagent` 已是独立上下文 + 独立缓存，符合此路径，保持。

### 5. 不做清单（守住创作质量红线）

- 裁剪工具定义/精简 schema 省 token：**禁止**（AGENTS.md 红线）。工具定义是稳定前缀主体与命中主力，交叉比对确认是"该留的大头"。
- 按阶段动态裁剪 tools 数组：tools 一变，前缀全废（ADR-0001 已拒绝；Claude Code 同类场景用 defer_loading 存根，Goink 全量发送等价于其"tools 永不移除"设计）。
- NovelState 挪回固定系统前缀：每轮变化会毁掉整个 17.5K 稳定区（ADR-0001 旧坑）。

---

## 五、与现有文档/决策的关系

- 与 `architecture/cache-hit-mechanism.md` 结论一致（动态尾部 miss 正常），但纠正了其第四节"跨轮全部命中历史"的乐观解读——实测增量恒定，历史并未命中；根因在 NovelState 幽灵占位。
- 与 `adr/0001-prompt-caching.md` 思路一致：NovelState 走缓存前缀外（末尾）是对的，缺的一半是"末尾也要落库形成完整上一请求单元"。
- 与 `design/token-optimization-plan.md` Phase 1 结论一致：工具全量保缓存稳定，本文不推翻该项决策。

---

## 相关文档

- `adr/0001-prompt-caching.md` — 前缀稳定化决策
- `architecture/cache-hit-mechanism.md` — 缓存机制详解（含本文数据的再解读）
- `architecture/token-injection.md` — 注入构成统计
- `archive/billing-test-report.md` — 实测数据
- `design/token-optimization-plan.md` — Token 整体优化方案
- 外部：DeepSeek KV Cache 官方文档、Anthropic Claude Code Prompt Caching 工程博客、Chat-deep DeepSeek 缓存实测、arXiv 2601.06007