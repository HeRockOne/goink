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
| [provider-status.md](architecture/provider-status.md) | 内置 Provider 配置状态（7 个 provider，联网核实） |
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
> 2026-08-05：文档更新——narrative-panel.md 未来卡片改为 react-markdown 渲染原始大纲；SNAPSHOT_DESIGN.md/git/DESIGN.md 更新 goink.md 指纹账本角色；ROADMAP/README/architecture.md 修复主题系统旧引用；audit-repair-summary.md 标注 OutlineParser 已过时；mcp-tools/mcp-schema audit 更新 goink.md 操作描述。
> 2026-08-05：全量审计修复同步文档——phase-gate.md 更新（默认 seed/visited 持久化/loop 生效/故障排查）；agent/DESIGN.md 加过时声明（MaxTurns=100、压缩已实现含工具定义 token、全量 tools、重试上限）；rollback/DESIGN.MD 补 FK 级联已知限制；narrative-panel.md 补顶层字段与 characters.status；architecture.md 更新 search 描述；AGENTS.md 修正 CGO 编译说明；build.ps1 CGO_CFLAGS 动态化。

## tools/ — 验证工具（可运行）

| 工具 | 说明 |
|------|------|
| [cacheprobe](../../cmd/cacheprobe/README.md) | 缓存命中率探针（tiktoken 精确计数）：严格按门禁配置完整流程，NS 落库后 miss 降 26.4%；核心逻辑在 internal/cacheprobe 库，设置面板「缓存模拟」Tab 可手动触发，`go run ./cmd/cacheprobe [门禁轮数] [短对话轮数]` |

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
| audit-repair-2026-08-05.md | 全量代码审计与修复记录（P0-P3：门禁 seed/死角色拦截/RAG 原子性/FTS5 检索/外键等，含真机验证） |

---

## 归档规则

- 一次性的审计报告、测试报告、调研记录、讨论记录 → `archive/`
- 描述系统当前设计的文档 → `architecture/`
- 尚未实施或作为参考的方案 → `design/`
> 2026-08-07：缓存链路补全（prompt_cache_key=sessionID 路由粘性，对齐 opencode PR #22569，消除小米 MiMo 直连偶发全 miss；子 agent fork 完整主历史，重复 read 的 4-10K/轮 miss 归零；cacheprobe 补子 agent 内部序列模拟，now 99.5% vs legacy 99.3%；全 miss 告警日志；UI：思考开关+深度合并下拉、工具执行中去掉处理中徽章）
> 2026-08-07：文档过时审计并修复——theme-system.md 重排（全文被压成单行）+ 删除已移除的特效系统（d032284，粒子变量标注遗留）；phase-gate.md require 表对齐出厂默认配置（prepare 9 项/write 2 项/maintain 13 项）、main-cmd-next 统一为 next、新会话起始 prepare；billing-panel.md 状态改已实施 + 缓存字段优先级对调（首选 prompt_tokens_details.cached_tokens，fallback prompt_cache_hit_tokens）；architecture.md 工具 57→59、/api/search-memory 路径、目录树补 15 个模块、get_writing_context 去伪字段、API 端点 29→31、内置 skill 表补全、help 51→53；token-injection.md skill 41→42/工具 57→59/auto 37→38；narrative-panel.md 未来卡数据源改为 useOutlineCache 读 outlines/NNN.md（3 章）、z-index 50→8、近期待收筛选 ≥当前章；provider-status.md 补 doubao/minimax/mimo；cache-hit-mechanism.md 修正行号；README 索引补 provider-status.md
> 2026-08-07：叙事面板数据口径审计修复（对齐 maintain 流程）——当前卡角色改 characters_in 优先（回退 active_chars）、物品改 item_occurrences 本章流转（后端 get_writing_context 新增字段）、未定时伏笔单独分组、弧线当前节点排除 actual_chapter 提前完成节点、弧线节点 Limit 50→200；UI 修复——标题栏移除 Logo/标语/GitHub 链接、叙事按钮改 ScrollText 图标、新增门禁开关按钮（Shield/ShieldOff）、状态栏门禁条改 flex 中段防重叠、叙事面板标题栏删除、overlay 避开 header/状态栏、对话历史分页渲染（默认 30 轮+加载更早）、性能优化（移除 6 处 backdrop-blur、sidebar 透明度提高、bg-layer 纹理减半）
> 2026-08-08：门禁必读技能体系——新增 read_required 工具（60 个，参数化读技能，零硬编码）+ 门禁 require_reads 字段（阶段内强制 + 通配符，跨阶段读取不算）+ 每阶段必读技能配置（init 5 个/prepare common-sense-logic/outline hook+title/write show-dont-tell+anti-ai/write maintain anti-repetition+foreshadow）；sub- 前缀技能自动注入 review 子代理（[身份][sub-*技能][NS] 消息拆分，技能常量字节放 NS 前跨 review 命中缓存，替代子代理自 read）；skill 合并 chapter-title-hooks 并入 chapter-title-design（42 个内置）；创作视角 skill 审计修复（字数下限 2500、类型口径统一、自引用修正等）
> 2026-08-08：缓存模拟集成——cacheprobe 抽为 internal/cacheprobe 库（核心逻辑 + 真实生成器注入：identity/always/catalog/工具定义/子代理身份/NS 读真实 DB+goink.md，assistant 消息含 reasoning_content+tool_displays、set_phase 注入 system-reminder），cmd/cacheprobe 变薄壳 CLI；设置面板新增「缓存模拟」Tab（异步 StartCacheSimulation + cachesim:done 事件，轮数/短对话穿插可调，按设置价格估算成本）；消息级缓存优化（token/marshal/toolDefs 缓存，完整模拟 365s→13.8s 提速 26 倍）
