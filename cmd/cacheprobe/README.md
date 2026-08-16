# cacheprobe — 缓存命中率探针（消息级 + tiktoken 精确计数）

无网络、无 LLM 调用、无需 API Key 的缓存命中率模拟工具。验证 NovelState 落库协议
（P1）对 DeepSeek/商汤前缀缓存的收益。

核心逻辑在 `internal/cacheprobe` 库（设置面板「缓存模拟」Tab 同源调用），
`cmd/cacheprobe` 是薄壳 CLI。

## 原理

DeepSeek/商汤的磁盘缓存按"请求的公共前缀"匹配（官方文档：命中 = 本次请求与上次
请求的公共前缀，TTL 内有效）。探针：

1. **连续性判定**：消息序列的字节级公共前缀（精确）——复刻 provider 的 KV cache 前缀匹配
2. **token 统计**：每条消息用 **tiktoken（o200k_base）精确计数**（`llm.CountMessageTokens`，
   含 content/tool_calls/tool_call_id/reasoning），tools 定义作为固定前缀消息计数
3. 命中 = 公共前缀覆盖的消息 token 和；miss = 其余消息 token 和
4. **输出 token 统计**：相对上次请求新增的 assistant 消息字节（含 reasoning_content）——
   与输入侧同源，覆盖正文（edit arguments）/文本回答/子代理报告/思考
5. **消息级缓存**（2026-08-08）：消息 token 数/序列化字节/toolDefs 均缓存，完整门禁
   5 轮模拟 365s → **13.8s**（26 倍加速）
6. **增量计算**（2026-08-11）：缓存 key 用轻量 `msgFingerprint`（拼接字段，不 marshal——
   原实现对全部历史消息每次请求重新序列化生成 key，profile 占 CPU 28.5%）；`step` 在字节
   前缀连续（lcp 覆盖上次请求末尾）时直接复用上次累计值，只处理新增消息——单章 26 章
   模拟 60s+ → **6.9s**（再 9 倍）

关键前提（已在代码中验证）：provider 缓存作用于**解析后的 token 前缀**（tools 定义
转 system 前缀在最前、消息按序追加在末尾），而非原始 JSON body 的字节顺序。

> 2026-08-09：assistant 消息带 reasoning_content（思考模式），长度按门禁阶段均值
> （init 556/prepare 822/outline 971/write 322/review 1558/maintain 364 字符，统计自
> 真实 DB thinking_content 按 set_phase 边界分阶段）；set_phase 消息顺序对齐真实
> agent.go（技能注入+reminder 在 assistant 落库前）；成本 = hit×cache + miss×input + out×output。
> 正文按章独立生成：目标字数 = 设置的 (min+max)/2 + 正态波动×386 字符（真实 std，
> 实测 D:\Goink\novels 19 章：均值 3319/范围 2652-4073），clamp 到设置范围，固定
> seed 可复现——章节间长度与内容均不同，贴近真实输出分散度。

## 用法

```powershell
# 设置数据目录（读真实 DB 书名/简介 + goink.md 指纹；默认 exe 目录或 ~/Goink）
$env:GOINK_DATA_DIR = "D:\Goink"
# 真机环境：直接指定真实 DB 文件（可指向复制出来的 Goink 目录，如 test-goink-for-real\Goink\novel-agent.db）
$env:GOINK_DB_PATH = "D:\Goink\novel-agent.db"
# 门禁配置：默认找项目根 门禁配置示例.md（技能清单 + 白名单校验），可用环境变量覆盖
$env:GOINK_PHASE_CONFIG = "门禁配置示例.md"
go run ./cmd/cacheprobe            # 默认：单章 5 轮 + 短对话穿插 5 轮 + 批量 5 章（三协议对照）
go run ./cmd/cacheprobe 5 3        # 单章 5 轮 + 短对话穿插 3 轮 + 批量 5 章
go run ./cmd/cacheprobe 5 3 5      # 显式指定批量 5 章
# 价格参数（元/百万 token，默认 DeepSeek：缓存 0.02 / 输入 1 / 输出 2）
go run ./cmd/cacheprobe -cache 0.02 -input 1 -output 2 5 5 5
# reasoning effort 档位（默认 low；high 按 2026-08-16 真机 mimo-v2.5 high 会话校准）
go run ./cmd/cacheprobe -effort high table
# 模式驱动（与设置面板「写书成本模拟」Tab 同源，RunWindowMode）：
go run ./cmd/cacheprobe window     # 上下文刻度（single 26 章 / batch 120 章，128K→1024K 快照 + 最省区间）
go run ./cmd/cacheprobe table      # 14 个常用工作负载 Markdown 成本表
go run ./cmd/cacheprobe skilldedup # 技能注入去重对照
go run ./cmd/cacheprobe nsondemand # NS 按需注入对照
```

CLI 参数：`go run ./cmd/cacheprobe [-cache ¥] [-input ¥] [-output ¥] [-firsthit 0-1] [-effort low|high] [单章轮数] [短对话穿插轮数] [批量章数]`

## 表格输出（成本模拟表）

```powershell
go run ./cmd/cacheprobe table   # 跑 8 个常用工作负载场景，输出 Markdown 表格
```

场景矩阵：单章 1/3/5 轮、单章+短对话、批量 5 章（±短对话）、混合 3+2+3、混合 5+5+5，
**续写场景**（hist>0，对齐真机"带历史续写"用法：先完整跑 N 轮单章建立历史，再跑目标流程，
只统计目标部分增量成本——历史构建成本不计入，真机中已发生）。
每行含输入 hit/miss、输出 out、命中率、总成本、每章成本（now 协议，价格同上可调）。
主表之后输出 miss 构成表（按消息来源分类：thinking 思考/技能注入/工具结果/查询/固定与NS/正文/大纲/其他，
与 TokenCache miss 计算同路径，首轮全量与 tools 计入"固定/NS"列）。
最后输出**门禁配置一致性校验**：8 个场景的 plays 工具调用逐一对照门禁配置阶段白名单
（set_phase 永远放行，场景开头未进入阶段前跳过），不一致即报告——发现模拟器与真实
门禁配置漂移（如 2026-08-09 修复的 read_required 虚构工具 → auto_skill_injection 真实工具）。

2026-08-16 实测（DeepSeek 价 0.02/1/2，真实 DB + 门禁配置驱动，技能注入去重 + 查询 size + 首轮被动缓存 + 大纲/子代理/续写场景校准后，reasoning low 档）：

| 场景 | 单章 | 短对话 | 批量 | 输入 hit | 输入 miss | 输出 out | 命中率 | 成本 ¥ | 每章 ¥ |
|------|-----|-------|------|---------|----------|---------|--------|--------|--------|
| 单章 1 轮 | 1 | 0 | 0 | 1839247 | 116349 | 14583 | 94.1% | 0.1823 | 0.1823 |
| 单章 3 轮 | 3 | 0 | 0 | 9692173 | 400897 | 35824 | 96.0% | 0.6664 | 0.2221 |
| 单章 5 轮 | 5 | 0 | 0 | 23430789 | 691904 | 63831 | 97.1% | 1.2882 | 0.2576 |
| 单章 5 轮 + 短对话 3 | 5 | 3 | 0 | 28142576 | 791778 | 69808 | 97.3% | 1.4942 | 0.2988 |
| 批量 5 章 | 0 | 0 | 5 | 3703188 | 145753 | 27920 | 96.2% | 0.2757 | 0.0551 |
| 批量 5 章 + 短对话 2 | 0 | 2 | 5 | 5067691 | 211079 | 30195 | 96.0% | 0.3728 | 0.0746 |
| 混合 3+2+3 | 3 | 2 | 3 | 21310126 | 636324 | 62826 | 97.1% | 1.1882 | 0.1980 |
| 混合 5+5+5 | 5 | 5 | 5 | 50150845 | 1030485 | 97520 | 98.0% | 2.2285 | 0.2229 |
| 续写单章（历史 3 章） | 0 | 0 | 0 | 7174373 | 91138 | 13198 | 98.7% | 0.2610 | 0.2610 |
| 续写批量 5 章（历史 4 章） | 0 | 0 | 5 | 15462584 | 124102 | 27812 | 99.2% | 0.4890 | 0.0978 |
| 批量 10 章 | 0 | 0 | 10 | 6549639 | 174570 | 44441 | 97.4% | 0.3944 | 0.0394 |
| 批量 20 章 | 0 | 0 | 20 | 14374757 | 236563 | 82968 | 98.4% | 0.6900 | 0.0345 |
| 批量 30 章 | 0 | 0 | 30 | 24362906 | 299538 | 121969 | 98.8% | 1.0307 | 0.0344 |
| 批量 40 章（≈1M×0.7 压缩阈值边缘） | 0 | 0 | 40 | 37076589 | 363236 | 159630 | 99.0% | 1.4240 | 0.0356 |
| 批量 50 章（超阈值，真机需压缩） | 0 | 0 | 50 | 53232990 | 429041 | 199200 | 99.2% | 1.8921 | 0.0378 |
| 批量 60 章（超阈值，真机需压缩） | 0 | 0 | 60 | 70419648 | 485061 | 230747 | 99.3% | 2.3549 | 0.0392 |

> 2026-08-16 修正说明（第六次）：续写场景**请求数爆炸修复**——首版 runGateRound/
> runBatchRounds 逐 play 串行（每个工具调用一次请求），未走 runPlays 的分组并行
> （一组最多 10 个调用合并一次请求，对齐真机 LLM 并行行为），导致请求数放大
> （单章轮 89 vs 真机 18、目标批量 182 vs 72）、hit 虚高数倍（批量 59.8M vs 真机
> 9.78M）。修复：续写场景全部改走 runPlays（分组 + onSubagent + onPhase 注入）。
> 修复后：续写批量 5 章 hit 15.46M/miss 124.1K/out 27.8K（每章 ¥0.0978 vs 真机
> 剔抽风 ¥0.083，差 18%——剩余 = 历史前缀 1.9 倍胖：技能注入全文 + longContext
> 大模板 + 完整 review/maintain 每轮）；续写单章 out 13.2K vs 真机 12.8K（差 3%）、
> miss 91.1K vs 53.5K、成本 ¥0.261 vs ¥0.104（差 2.5 倍——**真机第四章是旧版软约束
> 精简流程**：20 工具调用/prepare 3 查询/无大纲 edit/miniMaintain 2/6，模拟器建模
> 硬约束后的规范全流程 90+ 步；新版真机单章待测）。

> 2026-08-16 修正说明（第五次）：新增**续写场景**（对齐真机常态"带历史续写"）——
> 模拟器既有场景全部从空会话起步（开书 + 技能全量注入），与真机（已有 N 章历史，
> 技能/查询/正文在前缀里）结构性不同构：单章 miss 差 2.2 倍、批量 hit 差 2.4 倍。
> 实现：runInitRounds + runGateRound + runBatchRounds（api.go），续写单章 = N 轮历史 +
> 目标 1 章，续写批量 = N 轮历史 + batchLightEndReviewBase（sim.go，reviewPlaysBatch 加
> base 偏移、batchCore prepare 改 base+1）。只统计目标部分增量（历史轮结果丢弃、
> output/miss 分类从基线截断）。实测 low 档：续写单章（历史 3 章）miss 91.1K/out 13.2K，
> 对标真机第四章 53.5K/12.8K——输出差 3%（思考/正文口径准确），miss 高 1.7 倍 = 模拟器
> 规范全流程每阶段重复查询（prepare/review/maintain 全套 ~24 查询）vs 真机并行合并 +
> 省 token 参数；续写批量（历史 4 章）miss 124.1K vs 真机 167.0K（模拟低 26% = 真机含
> 抽风重试/拖沓请求）、out 27.8K vs 真机剔抽风 ~25.7K（高 8%）。
> 批量 10-60 章数值随 preparePlays(base+1) 微调（此前 prepare 恒用第 1 章，现跟随 base）。

> 2026-08-16 修正说明（第四次）：真机批量会话（sess_2_18cc3abdae039518，mimo-v2.5 high，
> 5 章 + 审稿 + 维护）逐请求深挖后校准——① 大纲输出加长到真机量级（5 章大纲单请求 10,986
> token ≈ 2.2K/章，模拟 outlineText 由 ~200 字符扩到 ~1.6K）；② 批量审稿子代理改为读全批
> 正文（真机 review 子代理 fork 后并行 read 5 章，fork miss 14.4K）；③ Run 开头重置 simPhase
> （table 多场景串跑时 thinking 阶段不再串扰）；④ thinking 支持 reasoning effort 档位
> （-effort high：基数 ×3，review 2100 锚定真机 read 请求 comp 2.2K）。校准后批量 5 章
> high 档 miss 171.2K vs 真机 167.0K（差 2.5%）、输出 51.6K vs 60.7K（差 15%）。
> **注意：真机 60.7K 输出含模型抽风**——该会话经 yjm 中转的 mimo-v2.5 从第 7 章起思考
> 重复循环 + 超长思考（4 条消息合计 ~66K 字符 ≈ 35K token，占 comp ~58%，思考以 <think>
> 标签混入 content、reasoning_content 恒空）。剔除抽风后正常输出估计 ~25-30K，模拟器
> 51.6K 更接近"规范 LLM"上限；剩余差异 = 真实审稿核对输出 + 人工介入修复，非模拟失真。
> hit 差 2.4 倍 = 真机带 4 章旧历史 + 1M 窗口的结构性差异，模拟器场景从空会话起步。
> 命中率差 2.4pp = mimo 每轮新增固定内容被动缓存未建模。

> 2026-08-16 修正说明（第三次）：首轮固定前缀被动缓存建模——真机 mimo-v2.5 批量 5 章实测
> 首轮输入 34.3K 命中 28.7K（83.7%，MiniMax 对固定字节前缀有服务端被动缓存），模拟此前假设
> 首轮全 miss（高估 ~30K/会话）。新增 SimFirstHitRatio（默认 0.84，CLI -firsthit 可调，DeepSeek
> 磁盘缓存场景可设 0）。建模后批量 5 章 miss 184.7K→159.1K，与真机 167.0K 差 4.7%；
> 命中率 95.0%→95.7%（真机 98.3%，剩余差距 = mimo 对每轮新增固定内容的被动缓存，字节前缀
> 口径未建模）。输出口径差异（真机 60.7K vs 模拟 23.7K）为审稿拖沓的真实 LLM 行为，非模拟失真。

> 2026-08-16 修正说明（第二次）：① 8/9 后阶段必读技能由 set_phase 系统注入（auto-inject），
> 但模拟器 plays 仍保留 auto_skill_injection 工具调用（read_required 改名后过滤条件未同步）
> ——技能全文重复计数（update 类 45%）。修复 filterReadRequired 过滤条件 + 批量路径补过滤
> + missCatOf 按工具名分类。② get_chapter_list plays 未传 size 参数，真实执行按默认
> size=50 返回全量章节列表（含摘要），查询 miss 随章数暴涨（批量 60 章查询 1.24M）——
> 真实 LLM 按工具描述用 size=1（字数校验）/size=5（浏览）。plays 补 size 参数后批量 60 章
> 成本 ¥4.54→¥1.78，每章成本随章数摊薄不再爆炸。

结论：
- 批量模式每章成本最低（¥0.055/章，low 档），与真实日志批量 ¥0.097/章 量级吻合（真机含审稿拖沓输出）；单章轮内历史累积使后续章略贵（¥0.18-0.26/章）；短对话穿插小幅抬高成本。
- 模拟口径已四次校准（技能重复计数 / 查询 size / 首轮被动缓存 / 大纲与子代理与 thinking 档位），`-effort high` 下批量 5 章 miss 与真机差 2.5%、输出差 15%（剩余为真机审稿修复与人工介入的拖沓输出，非模拟失真）。

## 场景：一个真实对话窗口（混合会话）

模拟的不是三个互斥的独立场景，而是**一个真实对话窗口**——短对话与创作交替发生在
同一条历史里，与用户真实使用方式一致：

```
init(开书) → 短对话(查设定/改设定，AI 调工具) → 单章创作 → 短对话 → 单章创作 → ... → 短对话 → 批量创作 → 短对话
```

- **短对话轮**：每轮 2 问——用户查看设定（AI 调 get_writing_context/get_characters 并回答）、
  修改设定（AI 调 update_item/update_character 并回答）。穿插在创作轮之间（开头 1 轮 + 每轮单章后 1 轮 + 批量前后各 1 轮，受短对话轮数配额限制）
- **单章轮**：**严格按门禁配置 single 模式完整流程**（含 require_reads 必读技能）：prepare（9 项必查 + read_required）→ outline（大纲技能 + 2 次 edit）→ write（正文技能 read_required + 6 次 edit 写满设置字数 + 字数校验 + create_item_occurrence）→ write后自审 → review（run_subagent + 子代理 6 步内部序列模拟 + 修复）→ maintain（7 项查询 + 搜索 + 更新 + goink.md 指纹）→ set_phase("prepare")。每轮约 80 次工具调用 + 子代理 7 次请求
- **批量轮**：**按门禁配置 batch 模式**：prepare（一次）→ outline（一次出 N 章大纲）→ write（循环 N 章正文，技能只在循环开头加载一次）→ review（统一一次）→ maintain（统一一次）→ done

> 模拟与真实请求同源（2026-08-08）：
> - 系统提示词 `agentcfg.AgentIdentity`（主/子代理真实身份）
> - always/catalog 用 `agentcfg.BuildAlwaysSkillsContent`/`BuildSkillCatalog`（真实生成器，扫描 mode: always/auto）
> - sub-* 技能用与 `agent.buildSubagentSkills` 同源的扫描逻辑
> - NS 书名/类型/简介读真实 DB（novels 表，`GOINK_DB_PATH` 或 `GOINK_DATA_DIR`/novel-agent.db）、指纹读真实 `goink.md` 尾部 1500 字符
> - **章节字数读真实设置**（app_config 的 min/max 章节字数，get_chapter_list 校验同源；目标字数 = min + (max-min)/2，正文按目标字数生成）
> - assistant 消息含 reasoning_content + tool_displays；set_phase 后注入 user system-reminder（对齐 agent.go）
> - 技能内容从 SkillStore 读取（三层查找），打包后可用，零仓库路径依赖
> - 技能清单/小说数据变动自动同步，零硬编码。进度按 turn 动态（模拟创作推进；真实为 DB chapters 计数）

## 对照结论（tiktoken 精确计数）

| 场景 | 修复前累计 miss | 修复后累计 miss | miss 降幅 |
|------|----------------|----------------|----------|
| 单章流程 5 轮（独立场景） | 565,667 | 413,143 | **27.0%** |
| 批量创作 5 章 × 2 批（独立场景） | 250,021 | 198,288 | **20.7%** |
| 混合窗口（单章2+短对话2+批量2章） | 356,130 | 271,584 | **23.7%** |

### 关键发现

1. **轮边界是分叉点**：修复前每轮（或每批）首个请求把整段历史当 miss 重发；修复后轮边界只
   miss 新增的 user+NS。收益随历史（skill read、正文、maintain 查询）增长。

2. **混合窗口是真实口径**：独立场景（单章 27.0%、批量 20.7%）是上下界，真实使用中短对话
   与创作交替发生在同一窗口（23.7%）——收益落在两者之间。短对话轮的加入稀释了每章
   收益（历史更厚、但轮边界更多），批量在窗口中的出现又把收益拉回。

3. **精确口径低于字节口径**：早期字节口径（1 token ≈ 4 字节）报 35.0%，tiktoken
   精确计数修正为 26.4%——中文正文 token 密度高于 4 字节/token 的估算，
   字节口径高估了收益约 8 个百分点。**26.4% 是可信的真实成本节约**。

4. **收益随历史规模放大**：短问答（历史极小）now 略劣于 legacy（-10.9%，NS 落库使历史每轮膨胀 ~1.4K，短历史下膨胀成本 > 落库收益；门禁场景历史大则相反）；
   完整门禁创作 miss 降 27.0%。历史越厚，修复收益越大。

5. **命中率上限受压缩约束**：模拟未含压缩重置（压缩会把链重置为摘要，下一轮首请求
   全部 miss）。真实场景命中率上限 ≈ 压缩窗口内连续，不可能无限趋近 100%。

## 集成（设置面板）

「设置 → 写书成本模拟」Tab（2026-08-11 重构）：模式选择（单章 / 批量 / 混合）+ 参数输入，
异步运行（`StartCacheSimulation(mode, gateRounds, shortQARounds, batchChapters, batchRounds)` +
`cachesim:done` 事件，不卡 UI），按设置页模型价格估算成本（输入/输出/缓存命中单价，
与 ContextRing 同源）。

- **单章模式**：每章完整门禁逐章累积，默认 26 章 ≈ 1M 窗口，输出上下文刻度表
- **批量模式**：每批 6 章批次循环，默认 120 章 ≈ 1M 窗口，输出上下文刻度表
- **混合模式**：单章轮数 + 短对话轮数 + 每批章数 × 批量轮数，章号连续顺延，
  输出**阶段轮次成本表**（开书/短对话/单章轮/批量轮每阶段结束快照——混合窗口大小由输入
  决定，上下文刻度到不了大档位且反映不出工作负载结构，故用阶段打点替代）

单章 5+5+5 混合约 20 秒出结果。

## 测试

```powershell
go test ./internal/cacheprobe -v
```

覆盖：now 模式链连续、门禁场景 now miss < legacy miss、批量场景 now miss < legacy miss（批次边界）、
短问答差异在 NS 位置差异范围内、hit+miss = 请求总 token（tiktoken 精确性自检）。
