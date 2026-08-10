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
go run ./cmd/cacheprobe            # 默认：单章 5 轮 + 短对话穿插 5 轮 + 批量 5 章
go run ./cmd/cacheprobe 5 3        # 单章 5 轮 + 短对话穿插 3 轮 + 批量 5 章
go run ./cmd/cacheprobe 5 3 5      # 显式指定批量 5 章
# 价格参数（元/百万 token，默认 DeepSeek：缓存 0.02 / 输入 1 / 输出 2）
go run ./cmd/cacheprobe -cache 0.02 -input 1 -output 2 5 5 5
```

CLI 参数：`go run ./cmd/cacheprobe [-cache ¥] [-input ¥] [-output ¥] [单章轮数] [短对话穿插轮数] [批量章数]`

## 表格输出（成本模拟表）

```powershell
go run ./cmd/cacheprobe table   # 跑 8 个常用工作负载场景，输出 Markdown 表格
```

场景矩阵：单章 1/3/5 轮、单章+短对话、批量 5 章（±短对话）、混合 3+2+3、混合 5+5+5。
每行含输入 hit/miss、输出 out、命中率、总成本、每章成本（now 协议，价格同上可调）。
主表之后输出 miss 构成表（按消息来源分类：thinking 思考/技能注入/工具结果/查询/固定与NS/正文/大纲/其他，
与 TokenCache miss 计算同路径，首轮全量与 tools 计入"固定/NS"列）。
最后输出**门禁配置一致性校验**：8 个场景的 plays 工具调用逐一对照门禁配置阶段白名单
（set_phase 永远放行，场景开头未进入阶段前跳过），不一致即报告——发现模拟器与真实
门禁配置漂移（如 2026-08-09 修复的 read_required 虚构工具 → auto_skill_injection 真实工具）。

2026-08-09 实测（DeepSeek 价 0.02/1/2，真实 DB + 门禁配置驱动）：

| 场景 | 单章 | 短对话 | 批量 | 输入 hit | 输入 miss | 输出 out | 命中率 | 成本 ¥ | 每章 ¥ |
|------|-----|-------|------|---------|----------|---------|--------|--------|--------|
| 单章 1 轮 | 1 | 0 | 0 | 4068768 | 108515 | 26413 | 97.4% | 0.2427 | 0.2427 |
| 单章 3 轮 | 3 | 0 | 0 | 16894432 | 254669 | 66284 | 98.5% | 0.7251 | 0.2417 |
| 单章 5 轮 | 5 | 0 | 0 | 37678047 | 405548 | 108258 | 98.9% | 1.3756 | 0.2751 |
| 单章 5 轮 + 短对话 3 | 5 | 3 | 0 | 41862365 | 442502 | 112446 | 99.0% | 1.5046 | 0.3009 |
| 批量 5 章 | 0 | 0 | 5 | 11609220 | 147160 | 54644 | 98.7% | 0.4886 | 0.0977 |
| 批量 5 章 + 短对话 2 | 0 | 2 | 5 | 13152407 | 171796 | 57436 | 98.7% | 0.5497 | 0.1099 |
| 混合 3+2+3 | 3 | 2 | 3 | 36943576 | 381493 | 106333 | 99.0% | 1.3330 | 0.2222 |
| 混合 5+5+5 | 5 | 5 | 5 | 79916540 | 584716 | 166320 | 99.3% | 2.5157 | 0.2516 |

miss 构成（now 协议，token）：

| 场景 | miss 总计 | thinking | 技能注入 | 工具结果 | 查询 | 固定/NS | 正文 | 大纲 | 其他 |
|------|----------|----------|----------|----------|------|---------|------|------|------|
| 单章 1 轮 | 108515 | 26109 | 29745 | 15146 | 2975 | 27958 | 4586 | 1509 | 487 |
| 单章 3 轮 | 254669 | 81027 | 34539 | 58044 | 11882 | 38468 | 22012 | 6894 | 1803 |
| 单章 5 轮 | 405548 | 139485 | 39333 | 101207 | 21165 | 48978 | 39982 | 12279 | 3119 |
| 批量 5 章 | 147160 | 35714 | 29745 | 25803 | 3427 | 27958 | 17258 | 6753 | 502 |
| 混合 5+5+5 | 584716 | 197149 | 41730 | 133666 | 35266 | 90908 | 61704 | 20216 | 4077 |

结论：
- 批量模式每章成本最低（¥0.10/章，轮边界少、技能只加载一次），与真实日志批量
  ¥0.12/章 吻合；单章轮内历史累积使后续章略贵（¥0.24-0.28/章）；短对话穿插小幅抬高成本。
- miss 大头是 thinking（真实模型思考输出）与工具结果（正文/状态写入），合计 ~60%；
  技能自动注入（每阶段常量内容，但注入在当轮尾部新增区，每章 miss 一次）约占 10%，
  其中首轮 initInject 一次性 9.1K 已含在内。

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

「设置 → 缓存模拟」Tab：可调单章轮数/短对话穿插轮数/批量章数（0=不跑），异步运行（`StartCacheSimulation` +
`cachesim:done` 事件，不卡 UI），按设置页模型价格估算成本（输入/输出/缓存命中单价，
与 ContextRing 同源）。约 20 秒出结果（5+5+5）。

## 测试

```powershell
go test ./internal/cacheprobe -v
```

覆盖：now 模式链连续、门禁场景 now miss < legacy miss、批量场景 now miss < legacy miss（批次边界）、
短问答差异在 NS 位置差异范围内、hit+miss = 请求总 token（tiktoken 精确性自检）。
