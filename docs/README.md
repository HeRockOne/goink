# Goink 文档索引

> 文档按用途分层：`architecture/`（系统长什么样）、`design/`（待实施/参考方案）、`adr/`（决策记录）、`archive/`（一次性过程记录）。
> 新文档请归入对应目录，并在下方补充索引。

---

## 待办清单

| 文档 | 说明 |
|------|------|
| [TODO.md](TODO.md) | 待落地清单：真机验证（P1）、批量三章一轮批内检查（P2）、批量大小建议（P3）、技能注入去重评估（P4）、模拟器差异收敛（P5） |

---

## architecture/ -- 系统设计（长存，随代码更新）

| 文档 | 说明 |
|------|------|
| [architecture.md](architecture/architecture.md) | 系统架构（Wails + Go + React） |
| [phase-gate.md](architecture/phase-gate.md) | 阶段门禁系统 |
| [competitor-analysis.md](architecture/competitor-analysis.md) | 竞品分析 |
| [narrative-panel.md](architecture/narrative-panel.md) | 动态叙事面板设计 |
| [token-injection.md](architecture/token-injection.md) | Token 注入构成分析 + tokencount 使用说明 |
| [cache-hit-mechanism.md](architecture/cache-hit-mechanism.md) | 缓存命中机制详解（DeepSeek 前缀缓存，含流程推演） |
| [provider-status.md](architecture/provider-status.md) | 内置 Provider 配置状态（7 个 provider，联网核实） |
| [theme-system.md](architecture/theme-system.md) | 主题系统文档（50+ CSS 变量清单 + 派生关系 + 自定义主题 + Apple 白示例） |
| [cache-simulation.md](architecture/cache-simulation.md) | 缓存命中模拟库（cacheprobe）实现原理、成本估算口径与验证结论 |
| [writing-mode-guide.md](architecture/writing-mode-guide.md) | 创作模式决策与质量底线成本区间（规范基准每章成本、决策树、三道质量硬约束、真机核查方法） |

## design/ -- 方案（长存，参考用）

| 文档 | 说明 |
|------|------|
| [token-optimization-plan.md](design/token-optimization-plan.md) | Token 优化方案全集（含行业调研、风险、Grilling 结论） |
| [cache-hit-fix-implementation.md](design/cache-hit-fix-implementation.md) | 缓存命中率技术修复方案（P1 NS 落库协议 + P2 压缩 NS 修复） |
| [outline-on-demand-fix.md](design/outline-on-demand-fix.md) | 大纲按需加载 + 防越界方案（总纲落点 book-outline.md + 卷纲强制 + 进度锚点） |
| [goink-fingerprint-ledger.md](design/goink-fingerprint-ledger.md) | goink.md 定位收敛为章节指纹账本（DB 承载全部状态，edit 新增 append 模式） |

## tools/ -- 验证工具（可运行）

| 工具 | 说明 |
|------|------|
| [cacheprobe](../../cmd/cacheprobe/README.md) | 缓存命中率探针（tiktoken 精确计数，2026-08-09 起含精确输出统计 + 分阶段思考模拟 + 正文真实波动；2026-08-11 重构为模式驱动 + 9 倍性能）：模拟一个真实对话窗口——短对话（查/改设定）与单章/批量创作交替、一条历史贯穿；成本 = hit*缓存价 + miss*输入价 + out*输出价（默认 0.02/1/2，CLI 可调）；`table` 子命令输出 14 个常用工作负载 Markdown 成本表 + miss 构成 + 门禁白名单校验；`window` 子命令输出上下文刻度（128K/256K/512K/1024K 快照 + 最省区间）；正文长度读真实设置、小说信息读真实 DB；核心逻辑在 internal/cacheprobe 库，设置面板「写书成本模拟」Tab 可手动触发（模式驱动：单章/批量/混合，混合输出阶段轮次成本表），`go run ./cmd/cacheprobe [单章轮数] [短对话穿插轮数] [批量章数]` |

## adr/ -- 决策记录（长存，不可变）

| 文档 | 说明 |
|------|------|
| [0001-prompt-caching.md](adr/0001-prompt-caching.md) | Prompt Caching 消息前缀稳定化决策 |

## reports/ -- 评估报告（长存，专题分析）

| 文档 | 说明 |
|------|------|
| [phase-gate-caching-assessment.md](reports/phase-gate-caching-assessment.md) | 门禁配置系统 x 缓存命中优化深度评估 |
| [comprehensive-optimization-assessment.md](reports/comprehensive-optimization-assessment.md) | 全面潜在优化评估（质量优先 x 行业交叉比对） |
| [cache-hit-rate-analysis.md](reports/cache-hit-rate-analysis.md) | 缓存命中率分析（92% 构成、根因、优化建议） |
| [system-reminder-assessment.md](reports/system-reminder-assessment.md) | system-reminder 注入机制评估（能否 Go 替代） |

## archive/ -- 过程记录（归档，不再更新）

| 文档 | 说明 |
|------|------|
| [audit-log.md](archive/audit-log.md) | 审计日志（2026-08-04 ~ 2026-08-27，代码变更 + 架构决策记录，按时间倒序） |
| [llm-chain-audit-2026-08-12.md](archive/llm-chain-audit-2026-08-12.md) | LLM 链路全量审计（系统提示词/工具/skill/缓存/门禁/压缩，16 项修复 + 9 项遗留） |
| [true-machine-verification-2026-08-12.md](archive/true-machine-verification-2026-08-12.md) | 审计修复后真机验证手册（5 场景：init 可达/技能注入/batch 残留/压缩恢复/命中率并发，含 sqlite 自查命令） |
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
| audit-repair-2026-08-05.md | 全量代码审计与修复记录（P0-P3：门禁 seed/死角色拦截/RAG 原子性/FTS5 检索/外键等，含真机验证） |
| [full-audit-2026-08-22.md](archive/full-audit-2026-08-22.md) | 全量审计（系统提示词/28个MCP工具/门禁系统/50个skill，1个P0安全+2个P1 Bug+6处P1代码模式+4个Critical Skill矛盾，创作质量红线全部守住） |
| [creation-test-audit-guide.md](archive/creation-test-audit-guide.md) | 创作测试会话审计指南（DB schema/审计流程/6层架构闭环/4会话对比/错误分类/快速SQL脚本，供新会话接手） |
| [architecture-fragility-audit-2026-08-26.md](archive/architecture-fragility-audit-2026-08-26.md) | 全架构脆弱性审计（成本C1-C4/质量Q1-Q6/前端F1-F4，10章实证，P1-P4优先级清单） |

---

## 归档规则

- 一次性的审计报告、测试报告、调研记录、讨论记录 -> `archive/`
- 描述系统当前设计的文档 -> `architecture/`
- 尚未实施或作为参考的方案 -> `design/`
