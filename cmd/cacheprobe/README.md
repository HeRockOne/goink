# cacheprobe — 缓存命中率字节级探针

无网络、无 LLM 调用、无需 API Key 的缓存命中率模拟工具。验证 NovelState 落库协议
（P1）对 DeepSeek/商汤前缀缓存的收益。

## 原理

DeepSeek/商汤的磁盘缓存按"请求的字节级公共前缀"匹配（官方文档：命中 = 本次请求与
上次请求的公共前缀，TTL 内有效）。探针用真实的消息序列化（Go map + encoding/json，
键序确定）构造每次 LLM 调用请求，按字节公共前缀计算理论命中量。

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
| 门禁创作 5 轮 | **严格按门禁配置 single 模式完整流程**：prepare（9 项必查 + 2 个 prepare 技能）→ outline（5 个大纲技能 + edit 大纲）→ write（4 个正文技能 + 3 次 edit 写 3000 字正文 + create_item_occurrence）→ review（run_subagent + 修复）→ maintain（7 项状态查询 + search_lore/items + update_chapter_meta/writing_snapshot/chapter_plan + create_scene + update_character/arc_node + create_timeline_entry + update_reader_perspective_entry + edit goink.md）→ set_phase("prepare")。每轮 40 次 LLM 调用 |

> 系统提示词、always skill（main-core-writing-kernel.md / main-core-ai-communication-standard.md）、41 个内置 skill 的 read 内容均取自仓库真实文件（相对仓库根解析，go run 与 go test 结果一致）。

## 对照结论（门禁创作 5 轮，完整门禁流程 + 真实 skill 内容）

| 指标 | 修复前 | 修复后 |
|------|--------|--------|
| 累计 hit | 31,333,897 | 31,468,875 |
| 累计 miss | 280,408 | 182,350 |
| 命中率 | 99.1% | 99.4% |
| miss 降幅 | - | **35.0%** |

### 关键发现

1. **轮边界是分叉点**：修复前每轮首个请求把整段历史当 miss 重发，且随历史（含 skill 内容 + 3000 字正文）增长：

   | 轮边界调用 | 修复前 miss | 修复后 miss |
   |-----------|------------|------------|
   | Turn 2 边界 | 24,694 | 2,559 |
   | Turn 3 边界 | 24,803 | 2,609 |
   | Turn 4 边界 | 24,912 | 2,659 |
   | Turn 5 边界 | 25,021 | 2,709 |

   修复后轮边界只 miss 新增的 user+NS（~2.5KB）；修复前把整段累积历史（约 24.7KB 且每轮 +110B）重发为 miss。

2. **字节命中率被工具定义稀释**：57 个工具定义占请求字节绝大部分，两种协议下都恒定命中，导致字节命中率都逼近 99%+。**真实的成本差异在 miss 字节**——未命中部分按全价计费，修复后 miss 降低 35.0%。

3. **收益随历史规模放大**：短问答（历史极小）差异可忽略（-0.1%）；完整门禁创作 miss 降 35.0%。历史越厚（skill read、正文、maintain 状态查询），修复收益越大。

4. **命中率上限受压缩约束**：模拟未含压缩重置（压缩会把链重置为摘要，下一轮首请求全部 miss）。真实场景命中率上限 ≈ 压缩窗口内连续，不可能无限趋近 100%。

## 测试

```powershell
go test ./cmd/cacheprobe -v
```

覆盖：now 模式链连续、legacy 模式轮边界分叉、门禁场景 now miss < legacy miss、
短问答场景两者差异在噪声范围内。
