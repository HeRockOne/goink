# Goink 文档索引

> 文档按用途分层：`architecture/`（系统长什么样）、`design/`（待实施/参考方案）、`adr/`（决策记录）、`archive/`（一次性过程记录）。
> 新文档请归入对应目录，并在下方补充索引。

---

## 待办清单

| 文档 | 说明 |
|------|------|
| [TODO.md](TODO.md) | 待落地清单：真机验证（P1）、批量三章一轮批内检查（P2）、批量大小建议（P3）、技能注入去重评估（P4）、模拟器差异收敛（P5） |

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
| [cache-simulation.md](architecture/cache-simulation.md) | 缓存命中模拟库（cacheprobe）实现原理、成本估算口径与验证结论 |

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
| [cacheprobe](../../cmd/cacheprobe/README.md) | 缓存命中率探针（tiktoken 精确计数，2026-08-09 起含精确输出统计 + 分阶段思考模拟 + 正文真实波动）：模拟一个真实对话窗口——短对话（查/改设定）与单章/批量创作交替、一条历史贯穿；成本 = hit×缓存价 + miss×输入价 + out×输出价（默认 0.02/1/2，CLI 可调）；`table` 子命令输出 8 个常用工作负载 Markdown 成本表（实测批量 5 章 ¥0.10/章、单章 ¥0.24-0.28/章，与真实日志吻合）；正文长度读真实设置、小说信息读真实 DB；核心逻辑在 internal/cacheprobe 库，设置面板「写书成本模拟」Tab 可手动触发，`go run ./cmd/cacheprobe [单章轮数] [短对话穿插轮数] [批量章数]` |

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
> 2026-08-08：cacheprobe 新增批量创作场景——按 batch 门禁模式模拟（prepare 一次 → outline 一次出 N 章大纲 → write 循环 N 章正文，技能仅循环开头加载 → review/maintain 统一一次 → done，连续 2 批体现批次边界）；gateScript 拆为 prepare/outline/write/selfReview/review/maintain 分段复用；CLI 第三参数批量章数、设置面板加批量章数输入；实测批量 5 章×2 批 miss 降 20.7%（单章 5 轮 27.0%）
> 2026-08-08：批量模式 + clean 真机验证（第 11-16 章，mimo-v2.5）——单章 ¥0.16/章（命中 97.7%）、批量 3 章 ¥0.12/章（命中 98.7%，与模拟吻合）；clean（已读技能清理）负收益——模型会重读被清内容，miss 120K→457K、输出翻倍，¥0.23/章 比批量贵 92%，结论：永久关闭（模拟假设"清了不用重读"错误）；事后技能补读审计——门禁只在 set_phase 校验，模型"先干活后补读"解锁，必读技能变手续非指导，已改事前强制（8c70a41：必读未加载时拦截 edit/update/create/run_subagent，只读/查询/管理放行）；UI 修复（d6c270e）——流式 text 段固定 id 防 ThinkingBlock 重挂载丢展开状态 + 思考结束自动收回、门禁进度条加单章/批量徽章（PhaseStatus 增 mode）、叙事面板刷新先取最新章节号再拉上下文（写完新章 current/past 卡立即更新）
> 2026-08-08：clean 数据复核与功能移除——查 goink.log model_usage 累计（会话 id=9），窗口 3 数值被 turn 2"继续"（19:46:53 被停止）污染（+525K hit/+78K miss），修正后批量+clean = ¥0.602/3 章（¥0.20/章），仍比纯批量 ¥0.12/章 贵 67%；根因不变（重读全 miss + 前缀断裂命中率 98.7%→93.8% + 输出 +78%）；已移除 clean 运行时功能（删除 context_clear.go/test、agent.go 选项与发送前 transform、app SetContextClear、config 字段、ContextRing 开关），cacheprobe 模拟保留作对照研究；AGENTS.md 补充调试日志位置 D:\Goink\goink.log
> 2026-08-08：token 链路优化（每章 ¥0.195→¥0.130，省 33%）——auto-inject 必读技能（set_phase 时系统自动注入该阶段 auto_skill_injection 技能为 system 消息，model 无需调 auto_skill_injection 工具，省掉工具调用两跳 + 拦截重试）+ set_phase 自动推进（require 满足后自动切换，不再等 LLM 调）；工具 read_required 重命名为 auto_skill_injection，配置字段 require_reads 统一为 auto_skill_injection（避免与 require 字段混淆）；删系统提示词阶段表格（与常驻 kernel 90% 重复，2 条唯一指令补入 kernel prepare）；kernel 复制到内置做备份；NS 缩短 800 字符已确认不能做（每章指纹 ~300 字，800 只够 2 章，防重复检查需 3 章）；writing_context 描述精简风险大不动；子代理完整历史 fork 精简后成本更高（模拟验证 1.4x）保持现状；设置面板缓存模拟同步 auto-inject 模式并简化展示（去掉旧/新对比，只显示总输入/命中率/费用）
> 2026-08-08：cacheprobe 场景重构为真实对话窗口——短对话（带工具调用：查/改设定）与单章/批量创作交替发生在同一条历史（buildMixedSession），替代三个互斥的独立场景；正文长度与字数校验读真实 app_config 设置（min/max 章节字数，目标 = min+(max-min)/2）；设置面板改「写书成本模拟」Tab（旧版本/当前版本列、M 单位、费用估算），单章轮数/短对话穿插/批量章数均可为 0；实测混合窗口(2+2+2) miss 降 23.7%
> 2026-08-09：cacheprobe 成本估算贴近真实——TokenCache 新增精确 output 统计（diff 新增 assistant 消息字节，覆盖正文 edit arguments/文本回答/子代理报告，与输入侧同源）；assistant 消息补 reasoning_content（按门禁阶段均值：init 556/prepare 822/outline 971/write 322/review 1558/maintain 364 字符，统计自真实 DB thinking_content 按 set_phase 边界分阶段）；set_phase 消息顺序对齐真实 agent.go（技能注入+reminder 在 assistant 落库前）；成本公式补输出项（CLI 价格参数化 -cache/-input/-output，默认 0.02/1/2），设置面板删 3000/轮拍脑袋估算改用精确 output；config/profile 默认价 1.35/8.1/0.27 → 0.02/1/2；实测单章 1 轮 ¥0.24/章（out 26K），与真实 ¥0.13-0.20/章 量级吻合；业务代码零改动
> 2026-08-09：cacheprobe 正文长度真实波动——每章正文独立生成，目标字数 = 设置的 (min+max)/2 + 正态波动×真实 std（386 字符，实测 D:\Goink\novels 19 章均值 3319/范围 2652-4073），clamp 到设置范围；chapterBody 全局单章 → chapterBodies 按章 6 段（16 章预生成，覆盖单章+批量+子代理读取），章节列表/场景 word_count 按章取波动值；固定 seed 42 可复现；实测单章 ¥0.2427、窗口(5+5+5) ¥2.5417，与波动前量级一致
> 2026-08-09：cacheprobe table 子命令——跑 8 个常用工作负载场景输出 Markdown 成本表（单章 1/3/5 轮、+短对话、批量 5 章 ±短对话、混合 3+2+3/5+5+5，含 hit/miss/out/命中率/总成本/每章成本，now 协议）；实测批量 5 章 ¥0.10/章（与真实日志 ¥0.12/章吻合）、单章 ¥0.24-0.28/章、混合 5+5+5 ¥0.25/章
> 2026-08-09：设置面板「写书成本模拟」Tab 同步最新模拟口径——CacheSimResult/Scenario 暴露输出 token（now_output/legacy_output），前端表格对齐 CLI table 列：输入 hit/输入 miss/输出 out/命中率/成本 ¥/每章 ¥（每章 = 成本 ÷ max(1, 单章轮数+批量章数)，短对话不计产出）；文案更新为 hit×缓存价 + miss×输入价 + out×输出价 + 分阶段 thinking
> 2026-08-09：cacheprobe table 补 miss 构成表——TokenCache 新增 missCat 分类钩子（与 miss 计算同路径，首轮全量与 tools 计入 fixed），ScenarioResult/Result 暴露 now/legacy/clean MissByCat；table 输出 8 场景 × 8 类别（thinking/技能注入/工具结果/查询/固定与NS/正文/大纲/其他）；实测单章 5 轮 miss 构成：thinking 34% + 工具结果 25% + 固定/NS 12% + 技能注入 10% + 正文 10%（技能注入为每章常量注入在当轮尾部新增区，每章 miss 一次，与真实代码 agent.go:512 无条件注入一致）；诊断测试 TestDiagMissBreakdown 补 buildMixedSession 同路径验证（缺口=0）
> 2026-08-09：批量质量节奏验证（白金方法论对照）——先判断现阶段单章/批量质量把控实现（单章：prepare 每章 9 项对齐 + 每章自审 + 每章子代理审稿 + 章末 maintain；批量：prepare 仅一次 + 全章纲 + 每章读大纲防串章 + miniMaintain 每章实时结算[优于单章章末落库] + 字数校验，但无写后自审、review 只审第 1 章）；按起点白金方法论"三章一轮"制度（每 3 章停笔自检，反对攒批积错）建模对比：批量 5 章现状 ¥0.0915/章、每章自审 ¥0.1050/章(+14.7%)、三章一轮 ¥0.0918/章(+0.3%)——三章一轮几乎免费（自检插在历史厚处，前缀命中），质量节奏对齐白金制度；结论：批量质量补齐方向 = 三章一轮批次自检（非每章自审），业务代码未动，待真机验证后实施
> 2026-08-09：批量大小权衡验证（TestDiagBatchSizeTradeoff）——单批 3/5/6/10 章每章成本 ¥0.119/0.092/0.090/0.084（批内越大越省，固定成本摊薄）；2批×3章 ¥0.112 比单批6章 ¥0.090 贵 24%（批边界新窗口成本）；结论：批量上限不由质量决定（三章一轮自检任何批量下 +0.3%），由上下文窗口/压缩点决定；"批量 3 章为一批/上限 3 章"是错误的（3 章是最贵档）；门禁配置示例.md 头部补批量使用建议说明（5-10 章/批 + 批内三章一轮自检 + 不拆多批）
> 2026-08-09：批内三章自检实现与粒度验证——实现 = outline 一次出全批大纲，write 循环内每 N 章插入批次检查（不跳阶段，避免 set_phase 技能重复注入与大纲分批；批内 maintain 不需要，每章 miniMaintain 已实时结算）；batchGatePlaysWith(chapters, checkKind, checkEvery) 支持两种粒度：checkKind=1 轻量自检（selfReviewPlays 2 技能+1 修改，+0.3%）/ checkKind=2 完整批次检查（batchCheckPlays：run_subagent 审最近 N 章+修复+字数复查，+4.9%）；批量 5 章实测：现状 ¥0.0915、三章一轮轻量 ¥0.0918、三章一轮完整 ¥0.0960（仍比单章 5 轮 ¥0.2751 省 65.1%）
> 2026-08-09：批内完整批次检查修正为走阶段切换（门禁白名单约束，用户指出的建模漏洞）——write 阶段白名单无 run_subagent（CheckToolAllowed 一律拦截），run_subagent 只能在 review 阶段；修正 batchCheckPlays = set_phase("review")（write→review next 推进 + write require 满足 + 字数校验通过）→ 子代理审最近 N 章+修复+字数复查 → set_phase("write")（review→write 回退到 visited 阶段，phase_gate.go:380）；batch review 段配置无 auto_skill_injection（切 review 不注入），回 write 注入 write 技能（重复注入成本计入）；修正后三章一轮完整检查 ¥0.0971/章（+6.1%，仍省 64.7%）；无需修改门禁配置
> 2026-08-09：批内检查 vs 完整门禁流程对比（用户论点验证）——checkKind=3 完整门禁流程（每批 review+maintain，2 次 maintain）¥0.1110/章，比批内检查（统一 maintain×1）¥0.0971 贵 14.3%：maintain 固定成本（13 项+技能注入 2.6K）重复 ×2、write/maintain 技能重复注入 ×2、阶段切换减半；且每章 miniMaintain 已实时结算状态，批末 maintain 只是收尾，重复做是纯浪费（真实中分次 maintain 还重复查询累计状态，更贵）；结论：批内检查 + 统一 maintain 最优（质量节奏对齐白金 + 成本 +6.1%）
> 2026-08-09：新增 docs/TODO.md 待落地清单（P1 真机验证/P2 批量三章一轮批内检查/P3 批量大小建议/P4 技能注入去重评估/P5 模拟器差异收敛），防止上下文丢失；README 顶部加待办索引
> 2026-08-09：质量 × 成本全方案对比（TestDiagBatchSelfReview 扩展）——质量分 = 写后自检(3)+审稿覆盖×3+状态实时(2)+写前对齐(1)+章纲/防串章(1)，满分 10；批量 5 章实测：三章一轮·批内检查质量 7.8/成本 ¥0.0971（性价比 80.3 最优档）、完整门禁流程质量 9.0/¥0.1110（性价比 81.1 最高）、单章 9.0/¥0.2750（性价比 32.7 最低）；结论：批内检查 = 质量与成本最佳平衡，要满分质量选完整门禁流程
> 2026-08-09：批量大小边界效应验证（TestDiagBatchCheckCoverage，用户质疑后补测）——5 章时批内检查覆盖 60%（检查触发 1 次）vs 完整门禁流程 100%（质量 9.0 vs 7.8，多 ¥0.014 值得，性价比 81.1 > 77.8，用户判断正确）；6 章时批内检查第 3/6 章触发覆盖 100%，质量同为 9.0 但 maintain 只 1 次，成本 ¥0.0991 vs ¥0.1033（性价比 90.8 > 87.1）批内检查反超；决策规则：批量 ≥6 章用批内检查，批量 3-5 章用完整门禁流程
> 2026-08-09：质量表补 token 消耗维度（hit/miss/out 列）——单章 hit 37.9M vs 批量 10-14M（单章每章完整门禁轮历史重发大 3 倍）；完整门禁流程 miss 149.7K vs 批内检查 139.5K（maintain 技能注入×2 + 阶段切换 +10K）；输出 out 单章 109.8K vs 批量 52-62K（单章近 2 倍，每章重复技能注入与审稿的 assistant 消息多）
> 2026-08-09：批量质量方案定案（业界对照）——用户质疑批内检查过度复杂 + 指出"子代理审 3 章但只 read 1 章修 2 处"不一致；websearch 对照 ainovel-cli/novel-creator/QMAI/webnovel-writer/xuanji-write：业界 = 轻量高频（每章状态对照/正则，零阶段切换）+ 重型低频（每 10 章/段级 LLM 评审）；废弃 batchInCheck（每 3 章阶段切换子代理 = 重型高频）；定案 batchLightEndReview = 每 3 章轻量自检（白金三章一轮，+0.3%）+ 批末全批审稿（子代理审全批 + 查 N 修 N，+4%）；实测批量 5 章 ¥0.0954/章、质量 9.0、性价比 94.4 全场第一、零阶段切换
> 2026-08-09：批量质量定案落地（P2 kernel 部分完成，Go 代码与门禁配置零改动）——skills/main-core-writing-kernel.md（用户级）+ 内置备份 + ~/.goink/skills 三处同步：批量流程概述与新增"批量质量节奏"段（每 3 章 read revision-pass+anti-ai-grade 轻量自检并立即修复、不调 run_subagent 不 set_phase；批末 run_subagent 审全部 N 章、逐章 read 自查+edit 修复+字数复查、不要只审第 1 章）+ review 阶段指令批量说明；main-cmd-phase-gate.md 批量流程描述同步；门禁配置示例.md 头部批量建议更新为定案方案；剩余 P1 真机验证
> 2026-08-09：门禁逐项验证 + 重复 set_phase 细节防御——门禁配置零改动逐项确认（自检 read/edit 在 write 白名单、edit 事前技能强制 4 必读技能已注入通过、批末审稿在 review 白名单且 review 段无 auto_skill_injection 不拦截）；确认用户提醒的细节：SetPhase 同阶段直接成功（phase_gate.go:328-330）+ injectPhaseSkills 无条件注入（agent.go:512）→ LLM 在 write 循环重复 set_phase("write") 会重复注入技能全文（~3K miss/次 + 挤占注意力），模拟器 plays 假设 LLM 规范未建模此风险；防御 = kernel/main-cmd-phase-gate/门禁配置示例三处加"write 循环内禁止重复 set_phase"强约束（业务代码不动——同阶段跳过注入有压缩后技能缺失风险）
> 2026-08-09：自检重心从文笔转向一致性（用户人群洞察）——目标用户是尝鲜 AI 写小说的普通人，抓不住文笔/AI 味/节奏，能抓住的是设定矛盾（前文死了的角色后文又出现）、文风偏移、章节混乱——这些是注意力衰减+遗忘机制的产物；每 3 章自检从 read revision-pass+anti-ai-grade（文笔向）改为**状态对照自检**（batchLightCheckPlays：get_characters/get_timeline/get_writing_snapshot 读状态，对照最近 3 章正文查设定一致性并修复，业界 check_consistency 同模式）；成本 ¥0.0954→¥0.0968（+1.5%），质量 9.0 不变，性价比 93.0 仍全场第一；kernel/main-cmd-phase-gate/门禁配置示例同步（审稿重点同样是一致性优先）
> 2026-08-09：自检双向合并（一致性 + 文笔不冲突，用户纠正）——每 3 章自检 = 一致性（重点：get_* 状态对照查设定矛盾）+ 文笔（次重点：read revision-pass + anti-ai-grade 查节奏/AI 味）一起做；成本 ¥0.0968→¥0.1003/章（+3.6% vs 现状），质量 9.0，性价比 89.7 仍全场第一；kernel/main-cmd-phase-gate/门禁配置示例三处同步"一致性重点 + 文笔次重点"
> 2026-08-09：批量循环机制修正 + 技能注入去重落地（P4）——核查门禁配置（batch write 有 loop:true，phase_gate.go:376-379 允许 write 回退 outline）与 Go 代码（agent.go:779 门禁自动推进只在回合结束兜底执行）后发现：批量循环本质靠 LLM 单回合连续工具调用，xindoo/ainovel-cli 等同行实测证实"LLM 自觉连写 3-5 章后主动结束任务"不可靠；用户设计定案：**每章显式 set_phase("write") 阶段边界 + 注入去重**（第一个 write 完整注入，后续 write 去注入）——injectPhaseSkills 改为只注入 missingInjections 缺失技能（agent.go:215-239），压缩成功后 ResetReads 清空门禁技能记录（compress.go，防去重误判导致压缩后技能衰减）；模拟器 batchCore 每章加 set_phase 显式边界（N-1 次幂等成功零校验）；成本 ¥0.1003→¥0.1027/章（+2.4%），性价比 87.7 仍全场第一；技能文档三处反转"禁止重复 set_phase"为"每章 set_phase('write') 声明章边界"（2507bab 约束撤销）；与 ainovel-cli StopGuard/逐章验收对照：Goink 无 end_turn 概念，显式阶段边界 + 门禁自动推进兜底等效
> 2026-08-09：门禁系统性重构（子代理审计 18 问题 + 7 盲区 + 10 测试缺口）——PhaseConfig 显式行为开关：inject/inject_dedup/same_phase/word_count_check(*bool,nil=legacy 仅 write)/word_count_reset(*bool)/mutating_guard，缺省=legacy 零迁移，替代 "write" 阶段名硬编码与隐式注入/同阶段语义；**门禁自动推进已删除**（与文档/评估"高危不做"对齐，消除失败路径假成功 reminder + 技能记错阶段缺陷）；set_phase 失败不计成功；auto_skill_injection 通配符展开（injectPhaseSkills 按 ListMeta 展开具体名）；删死代码 FailNext/SaveWordCount/LoadWordCount；default 配置 single/batch write 显式声明开关；新增 5 个开关测试；全量测试通过；剩余 9 项（require 按阶段重置/wordCountOK 结构化/visited 重置/init 可达/配置损坏告警/工具元数据化/per-session 状态/写入校验/技能缺失升 error）列入 TODO P5 待真机验证
> 2026-08-09：模拟器接真机环境（用户提供 test-goink-for-real/Goink 复制的 D 盘数据目录）——新增 internal/cacheprobe/gate.go 门禁配置驱动：GOINK_PHASE_CONFIG > 项目根 门禁配置示例.md 解析阶段/白名单/技能清单/行为开关，skillsFor 驱动全部 readRequired、phaseInjectSkills 按配置重建（配置改自动跟随，消除硬编码漂移）；table 新增"门禁配置一致性校验"（8 场景 plays vs 白名单，暴露并修复 2 处真漂移：read_required 是模拟器虚构工具→改 auto_skill_injection 真实工具（参数格式对齐 auto_skill_injection_tools.go）、批量场景省 initScript 导致初始阶段误判→跳过首 set_phase 前校验）；openRealDB 修 bug（gorm.Open 错误被忽略导致 nil 解引用 panic，改失败 fallback）；GOINK_DB_PATH 指向复制的真实 DB 后成本口径变化（单章 1 轮 ¥0.244→¥0.326、批量 5 章 ¥0.101→¥0.123，真实字数范围/小说信息驱动），全场景白名单校验 ✓
> 2026-08-09：工具结果真实化（用户要求模拟真机数据行为）——只读工具（get_*/search_*/read）走真实 Registry.Execute 读真实 DB 副本（realToolResult/execRealTool，ToolContext 注入 DB+NovelID+SkillStore），返回与真机 resultJSON 等价的 content；写工具（edit/update/create）保持模拟结果但包装成真机格式（{"success":true,"data":...}，wrapResult 对齐 agent/safety.go resultJSON），不污染副本可复现；17 处 toolMsg 调用点接入 playResult；**发现模拟器此前严重低估查询结果 token 量**（真实 get_writing_context/get_characters 返回远大于硬编码假数据，单章 5 轮查询 miss 21K→698K）；成本口径再变（单章 1 轮 ¥0.326→¥0.473、批量 5 章 ¥0.123→¥0.199/章），命中率更贴近真机（单章 1 轮 96.8% vs 真机 96.6%）；批量仍省 ~60%
> 2026-08-09：模拟器查询参数对齐真机省 token 调用（用户指出工具是否带参数）——核查全部查询工具 schema：get_characters(brief/size/search)/get_scenes(chapter_id/brief)/get_timeline(current_chapter 窗口切分)/get_story_arcs(current_chapter)/get_reader_perspective(counts_only)/get_lore/get_items(mode required,分页) 均带省 token 参数（Description 明令"不要一次获取全部"），get_lore/get_items/get_character_relations 的 mode/character_ids 为 required（模拟器 {} 调用真实执行校验失败自动 fallback，未爆炸）；真机日志证实 LLM 按省 token 指令调用（get_characters {size:15,brief:true}、get_reader_perspective {counts_only:true}）；模拟器 plays 全部对齐：get_characters {"brief":true,"size":15}、get_timeline/get_story_arcs {"current_chapter":N}、get_reader_perspective {"counts_only":true}、get_scenes {"brief":true}、get_lore/get_items {"mode":"list","size":10}（maintain/自检/子代理审稿同步）；成本：单章 1 轮 ¥0.4215→¥0.3788、批量 5 章 ¥0.1768→¥0.1570/章（vs 8/8 真机批量 3 章 ¥0.12/章，数据积累 19 章 vs 当时 3 章是剩余差异主因——get_writing_context 15K vs 当时 5-8K）；工具本身无 bug（get_writing_context 固定最近 5 章摘要+ID 索引，19 章 vs 1 章返回 13.5K→15.9K 不随总章数爆炸）
> 2026-08-09：按真机日志二次校准（用户指出"看日志"）——提取 8/8 窗口 1 完整会话（sess_1_18c9cd85fdd3d85c 写第 13 章，50 次工具调用）与窗口 2（sess_1_18c9d0c33c74f9ac 批量 17-19 章）：真机 prepare 用 brief（get_characters 721/get_scenes 2,545 brief）+ 窗口（timeline/arcs 2.7-2.9K current_chapter）+ 全量（reader 8,108/get_writing_context 14,942）；修正模拟器：characters/scenes brief、reader 全量、timeline/arcs current_chapter；get_lore/get_items 移出 prepare（真机不调）；**review/maintain 重复全套查询已建模**（真机 18:30-18:32 审稿重查 timeline/arcs/reader + maintain 全套 18.2K 字符，每次新消息 miss）；审稿子代理改为少量定向（真机 ~200-700 字符）；批量 5 章 ¥0.1711/章 vs 真机批量 3 章 ¥0.1199（吻合）；单章差 2.7 倍根因 = 模拟含开书（init 技能 29.7K）+ 数据积累（19 vs 12 章），窗口 1 是第 13 章续写无开书
> 2026-08-09：无上限工具加硬 LIMIT（子代理审计 100 章爆炸风险）——get_scenes 不传 chapter_id 全量（100 章 ~147K 字符）→ ListByNovel Limit(100) 最近优先；get_reader_perspective suspense 无截断（只种不收线性涨）→ ListActive Limit(100) + writing_context 计数改 Count 查询（截断不影响计数）；get_item_occurrences 全量（核心道具每章出现）→ ListByItem Limit(50) 最近优先；前端 UI/API 用无限制版本（ListAllByNovel/ListAllByItem，非 LLM 上下文）；工具 Description 更新"最近 N 条"语义；当前 19 章未触发限制数字不变；P2（timeline 异常区排序/writing_context timeline 100→50/arcs full 截断）待真机
> 2026-08-09：身份 × 工具参数矩阵落地（用户提出按身份精确取数）——作家（prepare）：get_characters brief（status 由 get_writing_context 提供）、get_timeline/arcs current_chapter 窗口、get_reader_perspective 全量（悬念内容写作必需）、get_scenes 按章；审稿核对（review/maintain）：主会话全量核对（characters 全量查 status、timeline/arcs 窗口、reader 全量、check_story_consistency 自动核对）——真机实测 review/maintain 重复全套查询；审稿子代理：fork 主历史（正文+状态已在上下文）+ 少量定向（brief+size 小）+ check_story_consistency；模拟器按身份参数建模（reviewPlays 加核对序列、subPlays 轻量化）；kernel 技能 prepare/review 段加参数指导（三处同步）
> 2026-08-09：全量审计工具范围参数（用户要求）——17 个查询工具逐一核对：15 个支持范围/指定/分页取数（get_writing_context current_chapter 树状、get_timeline/arcs 窗口、get_scenes chapter_id、get_characters search/brief/分页、get_lore/items/locations mode+ID 指定、get_entity_appearances 实体维度、search_* limit），仅 2 个只有全量路径，已补范围参数：**get_item_occurrences 加 chapter_from/chapter_to**（章节范围，join chapters 表按章号过滤，store ListByRange + 测试）；**get_reader_perspective 加 search/planted_from/planted_to**（内容关键词定向 + 种植章节范围，store ListActiveFiltered + 测试）；两个工具 Description 明确"禁止无范围/无过滤拉全量（上限 50/100 条）"
> 2026-08-09：60 工具 Description 全量规范改写（子代理审计 A13/B35/C12 + websearch 结合 OpenAI/Anthropic 官方最佳实践 + 小说创作场景）——统一 5 项结构（职责/触发时机/正面示例/负面约束/返回上限）；C 类 12 个重写（get_items 修 brief 参数 bug——描述声称支持但 Args 无此字段；get_lore/get_locations/get_stats/get_entity_appearances/search_* 补返回上限与触发；delete 三件套补不可恢复警示）；B 类 35 个补缺（触发条件按创作时机写：维护阶段/审稿核对/开书规划；防误触边界：get_timeline/get_story_arcs/get_chapter_list/get_character_relations/get_writing_snapshot 描述互标"管什么不管什么"；web_fetch 补长网页警示；run_subagent 补报告要求；set_phase 补批量循环打卡语义；create_item_occurrence 修正与 update_item 自动记录的矛盾描述）；修 delete_record 映射表笔误（get_timeline_entries→get_timeline）；红线 15 项方法论载体只增量零删减；固定前缀 +3.3K token（27.2K→30.5K），批量 5 章成本 +1.75%（¥0.1711→¥0.1741），命中率不变（98.4%）——一次前缀失效后恢复，机制未破坏
> 2026-08-09：架构审视与暂停（用户批评"为指标而指标"）——确认模拟器未建模：压缩触发（0.7×窗口，agent.go:307）、注意力衰减（lost-in-middle，单章 prepare 9 项必查 = 注意力重置才是单章质量真实来源）、thinking 波动、工具重试；批量 10 章 mimo 128K 窗口 ≈104K > 90K 必压缩，"省 40%"是乐观下界；采纳用户智能规则：自检 3 的倍数、批量 <6 章完整门禁流程 / ≥6 章批内检查；P6 新增模拟器 API 简化待办（batchGatePlaysWith 参数泥潭 → 语义化命名函数）
> 2026-08-09：P6 模拟器 API 简化完成——batchGatePlays/batchGatePlaysWith 参数泥潭删除，拆为语义化入口 batchAsIs/batchLightSelfCheck/batchInCheck/batchFullCycle，共享 batchCore 构造器；api.go 与 diag 测试调用点全部迁移，go build + 全量测试通过
