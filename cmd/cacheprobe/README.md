# cacheprobe — 缓存命中率探针（消息级 + tiktoken 精确计数）

无网络、无 LLM 调用、无需 API Key 的缓存命中率模拟工具。验证 NovelState 落库协议
（P1）对 DeepSeek/商汤前缀缓存的收益。

## 原理

DeepSeek/商汤的磁盘缓存按"请求的公共前缀"匹配（官方文档：命中 = 本次请求与上次
请求的公共前缀，TTL 内有效）。探针：

1. **连续性判定**：消息序列的字节级公共前缀（精确）——复刻 provider 的 KV cache 前缀匹配
2. **token 统计**：每条消息用 **tiktoken（o200k_base）精确计数**（`llm.CountMessageTokens`，
   含 content/tool_calls/tool_call_id/reasoning），tools 定义作为固定前缀消息计数
3. 命中 = 公共前缀覆盖的消息 token 和；miss = 其余消息 token 和

关键前提（已在代码中验证）：provider 缓存作用于**解析后的 token 前缀**（tools 定义
转 system 前缀在最前、消息按序追加在末尾），而非原始 JSON body 的字节顺序。

## 用法

```powershell
$env:CGO_CFLAGS = "-IC:\Users\Sophia\go\pkg\mod\github.com\mattn\go-sqlite3@v1.14.44"
go run ./cmd/cacheprobe compare   # 一次跑完两种协议，输出汇总对照（默认）
go run ./cmd/cacheprobe now       # 仅修复后（NS 落库）详细曲线
go run ./cmd/cacheprobe legacy    # 仅修复前（NS 不落库）详细曲线
```

## 两个场景

| 场景 | 说明 |
|------|------|
| 短问答 5 轮 | 每轮一问一答，无工具，历史极小 |
| 门禁创作 5 轮 | **严格按门禁配置 single 模式完整流程**（含 require_reads 必读技能）：prepare（9 项必查 + 3 技能 read_required）→ outline（10 个大纲技能 + 2 次 edit 大纲）→ write（11 个正文技能 read_required + 6 次 edit 写 3000 字 + 2 次字数校验 + create_item_occurrence）→ write后自审（2 技能 + 1 次修改）→ review（run_subagent + 子代理 6 步内部序列模拟 + 重读 + 3 处修复）→ maintain（7 项状态查询 + 2 搜索 + 11 项更新 + goink.md 指纹 + 2 技能）→ set_phase("prepare")。每轮约 80 次工具调用 + 子代理 7 次请求 |

> 模拟与真实请求同源（2026-08-08）：系统提示词用 `agentcfg.AgentIdentity`、always/catalog 用 `agentcfg.BuildAlwaysSkillsContent`/`BuildSkillCatalog`（真实生成器，扫描 mode: always/auto）、子代理 sub-* 技能用与 `agent.buildSubagentSkills` 同源的扫描逻辑——技能清单变动自动同步，零硬编码。

## 对照结论（门禁创作 5 轮，tiktoken 精确计数）

| 指标 | 修复前 | 修复后 |
|------|--------|--------|
| 累计 hit | 20,369,802 | 21,360,612 |
| 累计 miss | 159,656 | 124,526 |
| 命中率 | 99.2% | 99.4% |
| miss 降幅 | - | **22.0%** |

### 关键发现

1. **轮边界是分叉点**：修复前每轮首个请求把整段历史当 miss 重发；修复后轮边界只
   miss 新增的 user+NS。收益随历史（skill read、3000 字正文、maintain 查询）增长。

2. **精确口径低于字节口径**：早期字节口径（1 token ≈ 4 字节）报 35.0%，tiktoken
   精确计数修正为 **22.0%**——中文正文 token 密度高于 4 字节/token 的估算，
   字节口径高估了收益约 6 个百分点。**22.0% 是可信的真实成本节约**。

3. **收益随历史规模放大**：短问答（历史极小）now 略劣于 legacy（-11.4%，NS 落库使历史每轮膨胀 ~1.4K，短历史下膨胀成本 > 落库收益；门禁场景历史大则相反）；
   完整门禁创作 miss 降 22.0%。历史越厚，修复收益越大。

4. **命中率上限受压缩约束**：模拟未含压缩重置（压缩会把链重置为摘要，下一轮首请求
   全部 miss）。真实场景命中率上限 ≈ 压缩窗口内连续，不可能无限趋近 100%。

## 测试

```powershell
go test ./cmd/cacheprobe -v
```

覆盖：now 模式链连续、门禁场景 now miss < legacy miss、短问答差异在 NS 位置差异
范围内、hit+miss = 请求总 token（tiktoken 精确性自检）。
