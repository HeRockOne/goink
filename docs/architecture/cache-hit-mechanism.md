# 缓存命中机制详解（DeepSeek 前缀缓存）

> 日期：2026-07-31
> 目的：解答"门禁阶段重复读 skill 是否命中缓存""改 skill 是否破坏当前窗口缓存"等疑问
> 适用：DeepSeek / OpenAI 兼容格式的 Prompt Caching（自动启用，无需手动标记）

---

## 一、核心机制：缓存是"字节级精确前缀匹配"，不是相似度

**错误理解**：文本相似度查重，90% 相似 = 命中 90%。

**正确机制**（DeepSeek 官方 KV Cache）：
- 缓存对象是**请求开头部分**的计算结果
- 匹配条件是**逐字节完全相同**——任何一处不同，从那一点开始全部 miss
- 不存在"部分命中率"，只有"完全相同"或"不同"

```
请求1: [AAA][BBB][CCC]          → 全部写入缓存
请求2: [AAA][BBB][DDD]          → [AAA][BBB] 命中，[DDD] miss
请求3: [AAA][BBX][CCC]          → [AAA] 命中，[BBX][CCC] miss（BBB 变 BBX，从此断）
```

---

## 二、系统提示词层级（固定前缀 vs 动态）

首次对话注入 4 段 system 消息（`app/chat.go` `writeSystemMessages`）：

```
L1  Identity        → 1,322 tokens   人设/流程/规范（agentcfg/identity.go）
L2  Always skills   → 1,304 tokens   always 模式 skill 全量正文
L3  Skill catalog   →   572 tokens   auto 模式 skill 的 name+description 目录
L4  NovelState      → 动态注入       小说状态快照（放 user 消息之后，走缓存前缀外）
```

**固定前缀 = L1 + L2 + L3 + 工具定义（57 个 JSON）≈ 16,122 tokens**
- `writeSystemMessages` **只在创建新 session（isNew）时执行**（`chat.go:172`）
- 固定前缀写入 messages 表后**不再重写**，保证缓存稳定命中

---

## 三、命中范围：整段前缀，不止固定前缀

DeepSeek 缓存匹配的是"请求开头到最后一个与前一次相同的位置"，**通常覆盖所有历史消息**：

```
请求1: [固定前缀][novelstate][user: 写一章][tool: read writing-kernel]...
请求2: [固定前缀][novelstate][user: 写一章][tool: read writing-kernel][assistant: ...][tool: edit]...
         ↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑
         与请求1逐字节相同 → 全部命中（包括 read 的 skill、写过的正文、设定）
         [assistant][tool edit] ← 新增 → miss
```

**命中的是"前缀位置"，不是"内容查重"**：
- 所有"位置靠前、内容未变"的历史消息都命中——读过的 skill、写过的设定、正文、对话
- miss 的只有"本轮新追加的那一小段"（新 user 输入、新工具结果、新 read 内容）

---

## 四、具体场景推演

### 4.1 门禁阶段重复读 skill → 命中

```
请求N:   [固定前缀][历史][tool: read writing-kernel #1]...     → #1 写入缓存
请求N+1: [固定前缀][历史][read #1][assistant][tool: read #2]... → #1 命中，#2 新追加 miss
请求N+2: [固定前缀][历史][read #1][...][read #2][user: 继续]...  → #1 #2 都命中，只有新增 miss
```

**结论**：重复读 skill 不浪费——第一次读的在缓存里，第二次读虽新追加（miss 一次），读完后它又成为"已缓存的历史"，下次请求命中。

### 4.2 当前窗口改 skill → 零影响

| 改动 skill 的方式 | 当前窗口缓存 | 影响范围 |
|-----------------|-------------|---------|
| 改/新增 skill（含改 mode 为 always） | **零影响**（前缀已固化在 messages 表） | 只影响**新窗口**（isNew 重写前缀） |
| 触发**压缩** | 那轮重建前缀（`compress.go:88` 热加载最新 skill） | **当前窗口**压缩后生效 |

**热加载的真相**：skill 是热加载的（`store.go:61` `ListMeta` 每次读磁盘），但热加载只影响"下一次读取路径"（前端列表 / read 工具 / 压缩 / 新窗口），**不影响已固化的当前窗口缓存前缀**。

### 4.3 中途开关门禁 → 零影响

门禁开/关只是代码里的拦截开关（DB 配置），不注入、不修改任何消息。**缓存前缀不受影响**。

### 4.4 多章循环（回到 prepare 再写下一章）→ 连续命中

```
第1章...maintain→prepare
第2章: [固定前缀][第1章全部历史][novelstate(新)][user: 写一章]
       → 第1章全部历史命中 ✓，只有新 novelstate + user miss
```

### 4.5 上下文快满：压缩 vs 新窗口

| 操作 | 缓存影响 |
|------|---------|
| **压缩** | 重建系统消息 + 摘要（`compress.go:88` 热加载最新 skill）→ **那一次 miss**，之后新版本继续命中 |
| **新开窗口** | 系统前缀用最新 skill → **首轮 miss**，之后恢复命中 |

两者都会在"变更点"有一次缓存重建成本（几厘钱级），之后都正常。

---

## 五、实测数据佐证

测试报告（`docs/archive/billing-test-report.md`，商汤 sensenova-6.7-flash-lite）：

```
Turn 1: hit= 20,480  miss= 1,262   → 首轮 94% 命中（同轮内多轮工具调用）
Turn 4: hit=208,896  miss=25,896   → 89% 命中
```

hit 持续增长，正是**历史消息（读过的 skill、写过的设定、正文）在持续命中**的体现。

---

## 六、关键结论

1. **缓存是字节级精确前缀匹配**，不是相似度查重
2. **命中范围覆盖所有历史消息**，不止固定前缀——创作中累积的设定/正文/skill 内容恰是命中主体
3. **门禁重复读 skill 命中**——第一次读的在缓存里，重复读只有"新追加那次" miss 一次
4. **当前窗口改 skill 零影响**——热加载服务"下一次读取"，不破坏已固化的缓存前缀
5. **唯一缓存重建点**：压缩（当前窗口）和新窗口（首轮），都是一次性成本
6. **当前架构已是最优**：固定前缀 + 动态加载 + 追加式历史，完全匹配 DeepSeek 前缀缓存机制

---

## 相关文档

- `docs/architecture/token-injection.md` — 系统提示词层级划分 + tokencount 统计
- `docs/archive/billing-test-report.md` — 缓存命中率实测数据
- `docs/adr/0001-prompt-caching.md` — Prompt Caching 前缀稳定化决策
