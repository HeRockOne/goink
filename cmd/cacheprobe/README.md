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
4. **消息级缓存**（2026-08-08）：消息 token 数/序列化字节/toolDefs 均缓存，完整门禁
   5 轮模拟 365s → **13.8s**（26 倍加速）

关键前提（已在代码中验证）：provider 缓存作用于**解析后的 token 前缀**（tools 定义
转 system 前缀在最前、消息按序追加在末尾），而非原始 JSON body 的字节顺序。

## 用法

```powershell
# 设置数据目录（读真实 DB 书名/简介 + goink.md 指纹；默认 exe 目录或 ~/Goink）
$env:GOINK_DATA_DIR = "D:\Goink"
go run ./cmd/cacheprobe            # 默认 5 轮门禁 + 5 轮短对话对照
go run ./cmd/cacheprobe 5 3        # 5 轮门禁 + 3 轮短对话
```

CLI 参数：`go run ./cmd/cacheprobe [门禁轮数] [短对话轮数]`

## 两个场景

| 场景 | 说明 |
|------|------|
| 短问答 N 轮 | 每轮一问一答，无工具，历史极小（测 NS 落库膨胀成本） |
| 门禁创作 N 轮 | **严格按门禁配置 single 模式完整流程**（含 require_reads 必读技能）：prepare（9 项必查 + read_required）→ outline（大纲技能 + 2 次 edit）→ write（正文技能 read_required + 6 次 edit 写 3000 字 + 字数校验 + create_item_occurrence）→ write后自审 → review（run_subagent + 子代理 6 步内部序列模拟 + 修复）→ maintain（7 项查询 + 搜索 + 更新 + goink.md 指纹）→ set_phase("prepare")。每轮约 80 次工具调用 + 子代理 7 次请求 |

> 模拟与真实请求同源（2026-08-08）：
> - 系统提示词 `agentcfg.AgentIdentity`（主/子代理真实身份）
> - always/catalog 用 `agentcfg.BuildAlwaysSkillsContent`/`BuildSkillCatalog`（真实生成器，扫描 mode: always/auto）
> - sub-* 技能用与 `agent.buildSubagentSkills` 同源的扫描逻辑
> - NS 书名/类型/简介读真实 DB（novels 表，`GOINK_DB_PATH` 或 `GOINK_DATA_DIR`/novel-agent.db）、指纹读真实 `goink.md` 尾部 1500 字符
> - assistant 消息含 reasoning_content + tool_displays；set_phase 后注入 user system-reminder（对齐 agent.go）
> - 技能内容从 SkillStore 读取（三层查找），打包后可用，零仓库路径依赖
> - 技能清单/小说数据变动自动同步，零硬编码。进度按 turn 动态（模拟创作推进；真实为 DB chapters 计数）

## 对照结论（门禁创作 5 轮，tiktoken 精确计数）

| 指标 | 修复前 | 修复后 |
|------|--------|--------|
| 累计 hit | 56,303,413 | 58,322,417 |
| 累计 miss | 577,397 | 424,873 |
| 命中率 | 99.0% | 99.3% |
| miss 降幅 | - | **26.4%** |

### 关键发现

1. **轮边界是分叉点**：修复前每轮首个请求把整段历史当 miss 重发；修复后轮边界只
   miss 新增的 user+NS。收益随历史（skill read、3000 字正文、maintain 查询）增长。

2. **精确口径低于字节口径**：早期字节口径（1 token ≈ 4 字节）报 35.0%，tiktoken
   精确计数修正为 **26.4%**——中文正文 token 密度高于 4 字节/token 的估算，
   字节口径高估了收益约 8 个百分点。**26.4% 是可信的真实成本节约**。

3. **收益随历史规模放大**：短问答（历史极小）now 略劣于 legacy（-10.9%，NS 落库使历史每轮膨胀 ~1.4K，短历史下膨胀成本 > 落库收益；门禁场景历史大则相反）；
   完整门禁创作 miss 降 26.4%。历史越厚，修复收益越大。

4. **命中率上限受压缩约束**：模拟未含压缩重置（压缩会把链重置为摘要，下一轮首请求
   全部 miss）。真实场景命中率上限 ≈ 压缩窗口内连续，不可能无限趋近 100%。

## 集成（设置面板）

「设置 → 缓存模拟」Tab：可调门禁轮数/短对话轮数，异步运行（`StartCacheSimulation` +
`cachesim:done` 事件，不卡 UI），按设置页模型价格估算成本（输入/输出/缓存命中单价，
与 ContextRing 同源）。约 14 秒出结果（5+5 轮）。

## 测试

```powershell
go test ./internal/cacheprobe -v
```

覆盖：now 模式链连续、门禁场景 now miss < legacy miss、短问答差异在 NS 位置差异
范围内、hit+miss = 请求总 token（tiktoken 精确性自检）。
