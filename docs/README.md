# Goink 文档索引

> 文档按用途分层：`architecture/`（系统长什么样）、`design/`（待实施/参考方案）、`adr/`（决策记录）、`archive/`（一次性过程记录）。
> 新文档请归入对应目录，并在下方补充索引。

---

## architecture/ — 系统设计（长存，随代码更新）

| 文档 | 说明 |
|------|------|
| [architecture.md](architecture/architecture.md) | 系统架构（Wails + Go + React） |
| [phase-gate.md](architecture/phase-gate.md) | 阶段门禁系统 |
| [competitor-analysis.md](architecture/competitor-analysis.md) | 竞品分析 |
| [narrative-panel.md](architecture/narrative-panel.md) | 动态叙事面板设计 |
| [token-injection.md](architecture/token-injection.md) | Token 注入构成分析 + tokencount 使用说明 |
| [cache-hit-mechanism.md](architecture/cache-hit-mechanism.md) | 缓存命中机制详解（DeepSeek 前缀缓存，含流程推演） |
| [theme-system.md](architecture/theme-system.md) | 主题系统文档（50+ CSS 变量清单 + 派生关系 + 自定义主题 + Apple 白示例） |

## design/ — 方案（长存，参考用）

| 文档 | 说明 |
|------|------|
| [token-optimization-plan.md](design/token-optimization-plan.md) | Token 优化方案全集（含行业调研、风险、Grilling 结论） |
| [cache-hit-fix-implementation.md](design/cache-hit-fix-implementation.md) | 缓存命中率技术修复方案（P1 NS 落库协议 + P2 压缩 NS 修复） |
| [outline-on-demand-fix.md](design/outline-on-demand-fix.md) | 大纲按需加载 + 防越界方案（总纲落点 book-outline.md + 卷纲强制 + 进度锚点） |
| [goink-fingerprint-ledger.md](design/goink-fingerprint-ledger.md) | goink.md 定位收敛为章节指纹账本（DB 承载全部状态，edit 新增 append 模式） |

> 2026-08-04：cache-hit-fix-implementation.md 已实施（P1: NS 每轮落库 + 保留 K=3 快照；P2: NS 移出压缩系统区，改末尾落库；store 排序改 id；新增 compress_test/store_test）。未落地部分：P4（命中率报警阈值）、P5（用户运营纪律）。
> 2026-08-04：outline-on-demand-fix.md 已实施（book-outline.md 总纲落点 + get_writing_context 注入总纲摘要/进度锚点 + kernel/init-phase/门禁示例更新 + 侧边栏总纲入口）。
> 2026-08-04：goink.md 定位收敛为「章节指纹账本」（仅 append 模式追加，DB 承载全部状态/悬念/设定/偏好）；edit 工具新增 change_type=append；NovelState 注入 goink.md 尾部最近 1500 字符。

## tools/ — 验证工具（可运行）

| 工具 | 说明 |
|------|------|
| [cacheprobe](../../cmd/cacheprobe/README.md) | 缓存命中率探针（tiktoken 精确计数）：严格按门禁配置完整流程，NS 落库后 miss 降 29.1%，`go run ./cmd/cacheprobe compare` |

## adr/ — 决策记录（长存，不可变）

| 文档 | 说明 |
|------|------|
| [0001-prompt-caching.md](adr/0001-prompt-caching.md) | Prompt Caching 消息前缀稳定化决策 |

## reports/ — 评估报告（长存，专题分析）

| 文档 | 说明 |
|------|------|
| [phase-gate-caching-assessment.md](reports/phase-gate-caching-assessment.md) | 门禁配置系统 × 缓存命中优化深度评估 |
| [comprehensive-optimization-assessment.md](reports/comprehensive-optimization-assessment.md) | 全面潜在优化评估（质量优先 × 行业交叉比对） |
| [cache-hit-rate-analysis.md](reports/cache-hit-rate-analysis.md) | 缓存命中率分析（92% 构成、根因、优化建议） |
| [system-reminder-assessment.md](reports/system-reminder-assessment.md) | system-reminder 注入机制评估（能否 Go 替代） |

## archive/ — 过程记录（归档，不再更新）

| 文档 | 说明 |
|------|------|
| billing-panel.md | 计费面板技术设计（已完成功能的实现记录） |
| billing-test-report.md | 计费面板测试报告（含缓存命中率实测 89-93%） |
| billing-bug-report.md | 计费 Bug 原始报告 |
| billing-fix-audit.md | 计费修复审计 |
| prompt-caching-research.md | Prompt Caching 行业调研记录 |
| token-project-record.md | Token 优化项目完整讨论记录 |
| mcp-tools-audit.md | MCP 工具依赖链审计 |
| mcp-schema-audit.md | MCP Schema Required 全面审计 |
| audit-repair-summary.md | 审计修复总结 |
| data-integrity-audit.md | 数据完整性 + 看板审计 |
| feature-audit.md | 功能新增审计 |

---

## 归档规则

- 一次性的审计报告、测试报告、调研记录、讨论记录 → `archive/`
- 描述系统当前设计的文档 → `architecture/`
- 尚未实施或作为参考的方案 → `design/`
- 已确定的架构决策 → `adr/`（写一次不再改，变更时新建 supersede）
