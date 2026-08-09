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
> 2026-08-09：架构审视与暂停（用户批评"为指标而指标"）——确认模拟器未建模：压缩触发（0.7×窗口，agent.go:307）、注意力衰减（lost-in-middle，单章 prepare 9 项必查 = 注意力重置才是单章质量真实来源）、thinking 波动、工具重试；批量 10 章 mimo 128K 窗口 ≈104K > 90K 必压缩，"省 40%"是乐观下界；采纳用户智能规则：自检 3 的倍数、批量 <6 章完整门禁流程 / ≥6 章批内检查；P6 新增模拟器 API 简化待办（batchGatePlaysWith 参数泥潭 → 语义化命名函数）
> 2026-08-09：P6 模拟器 API 简化完成——batchGatePlays/batchGatePlaysWith 参数泥潭删除，拆为语义化入口 batchAsIs/batchLightSelfCheck/batchInCheck/batchFullCycle，共享 batchCore 构造器；api.go 与 diag 测试调用点全部迁移，go build + 全量测试通过
