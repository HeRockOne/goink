# Goink 审计日志

> 按时间顺序记录每次代码变更的背景、方案与影响。条目从 `docs/README.md` 迁出。

---

> 2026-08-04：cache-hit-fix-implementation.md 已实施（P1: NS 每轮落库 + 保留 K=3 快照；P2: NS 移出压缩系统区，改末尾落库；store 排序改 id；新增 compress_test/store_test）。未落地部分：P4（命中率报警阈值）、P5（用户运营纪律）。
> 2026-08-04：outline-on-demand-fix.md 已实施（book-outline.md 总纲落点 + get_writing_context 注入总纲摘要/进度锚点 + kernel/init-phase/门禁示例更新 + 侧边栏总纲入口）。
> 2026-08-04：goink.md 定位收敛为「章节指纹账本」（仅 append 模式追加，DB 承载全部状态/悬念/设定/偏好）；edit 工具新增 change_type=append；NovelState 注入 goink.md 尾部最近 1500 字符。

> 2026-08-05：文档更新——narrative-panel.md 未来卡片改为 react-markdown 渲染原始大纲；SNAPSHOT_DESIGN.md/git/DESIGN.md 更新 goink.md 指纹账本角色；ROADMAP/README/architecture.md 修复主题系统旧引用；audit-repair-summary.md 标注 OutlineParser 已过时；mcp-tools/mcp-schema audit 更新 goink.md 操作描述。
> 2026-08-05：全量审计修复同步文档——phase-gate.md 更新（默认 seed/visited 持久化/loop 生效/故障排查）；agent/DESIGN.md 加过时声明（MaxTurns=100、压缩已实现含工具定义 token、全量 tools、重试上限）；rollback/DESIGN.MD 补 FK 级联已知限制；narrative-panel.md 补顶层字段与 characters.status；architecture.md 更新 search 描述；AGENTS.md 修正 CGO 编译说明；build.ps1 CGO_CFLAGS 动态化。

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

> 2026-08-09：系统提示词 + 技能 description 闭环（P0）——mainAgentSystem1 门禁段补**批量显式循环**（每章 set_phase("write") 声明边界 + 压缩后补读，与 set_phase 工具描述/kernel 三方一致）；新增**省 token 总纲**段（查询默认最近 N 条/写作上下文以 get_writing_context 为准/范围参数优先/审稿按身份用参数——明确"省 token 绝不损害创作质量"）；技能 description 审计 42 个内置 skill：6 个 auto 技能无触发条件已补（genre-templates 选题材时/opening-chapter 写第一章时/world-building-system 开书建世界观时/word-count-calibration 字数控制时/common-sense-logic 合理性检查时/anti-repetition 防复读时），其余 36 个触发清晰（type 系列"写X时使用"、技法系列"当用户说X时"）；main-cmd-*（manual 用户触发）与 main-core-*（always 注入）description 短可接受；固定前缀 +530 token（30.5K→31.0K），批量 5 章 ¥0.1749（+0.5%），命中率不变（98.4%）

> 2026-08-09：真机边角料优化 A+B+E——A：set_phase 成功 reminder 删除（agent.go 成功分支不再注入 StatusString user 消息——工具结果已含 success+phase，真机日志每阶段切换一条冗余消息；失败分支保留"缺什么"信息）；B：审批 auto 反馈过滤（rw_tools.go 注入条件 Feedback!="" && !="auto"——auto 模式固定值零信息量，真机每次 edit 一条 74 字符冗余；人工反馈照常）；E：并行调用指导写入系统提示词（省 token 总纲加"无依赖工具并行发出，有依赖才串行"）+ kernel 技能阶段指令前加并行说明（prepare 9 项必查/maintain 查询批/迷你维护 6 项/自检 3 查询并行；三处同步）；A+B+E 是业务侧削减多余请求/提醒，C（模拟器并行建模）单独做

> 2026-08-09：模拟器并行工具调用建模（C，对齐真机 8/8 并行行为）——新增 asstToolCalls（一次请求多条 tool_calls + 1 次 thinking）+ runPlays（按 set_phase/run_subagent 断开分组，组上限 10，组内并行；回调支持 auto 技能注入/read 清理/子代理插入）+ filterReadRequired/injectPhaseOn；buildMixedSession 三段（initScript/gateScript/batchAsIs）+ buildGateWithRounds + buildBatchWithRounds 全部改用 runPlays；同步 A：appendPhase 成功分支不再注入 reminder；**效果：批量 5 章 ¥0.1750→¥0.0741/章（总量 ¥0.8748→¥0.3703 vs 真机批量 3 章 ¥0.3597——几乎吻合，证明之前串行模拟高估 thinking）；单章 1 轮 ¥0.2292（vs 真机窗口 1 ¥0.1601，剩余差 = 模拟含开书成本）**；全量测试通过

> 2026-08-09：架构审视与暂停（用户批评"为指标而指标"）——确认模拟器未建模：压缩触发（0.7×窗口，agent.go:307）、注意力衰减（lost-in-middle，单章 prepare 9 项必查 = 注意力重置才是单章质量真实来源）、thinking 波动、工具重试；批量 10 章 mimo 128K 窗口 ≈104K > 90K 必压缩，"省 40%"是乐观下界；采纳用户智能规则：自检 3 的倍数、批量 <6 章完整门禁流程 / ≥6 章批内检查；P6 新增模拟器 API 简化待办（batchGatePlaysWith 参数泥潭 → 语义化命名函数）

> 2026-08-09：P6 模拟器 API 简化完成——batchGatePlays/batchGatePlaysWith 参数泥潭删除，拆为语义化入口 batchAsIs/batchLightSelfCheck/batchInCheck/batchFullCycle，共享 batchCore 构造器；api.go 与 diag 测试调用点全部迁移，go build + 全量测试通过

> 2026-08-10 回归修复：门禁自动推进块恢复（agent.go 循环结束处）。03:43 删除后真机回归：LLM 在阶段边界收尾问用户而非主动 set_phase（19:36:09 prepare→outline 被拒、大纲写完不 set_phase(write) 停），恢复 01:53 备份行为（CheckTransitionReady 满足即自动 SetPhase+注入+reminder），并修正原实现 bug：SetPhase 返回检查、失败不发假成功 reminder。

> 2026-08-10 门禁整体回退 01:53 备份（用户决策：回归点在 03:43 行为开关重构 + 删除自动推进）。回退 7 文件：agent.go/phase_gate.go/compress.go/phase_gate_test.go/default_phase_gate_config.go/main-cmd-phase-gate.md/门禁配置示例.md；保留 60 工具 description 规范化、token 提示词、创作 skill。DB 开关行被旧解析器静默忽略无需迁移。

> 2026-08-10 review 白名单适配新版 kernel：cacheprobe 校验发现 review 阶段 plays 调用 get_character_relations/check_story_consistency/get_locations 不在白名单（新版 kernel 审稿核对教导，真机会被硬拦截）——single+batch 两处补 3 工具，代码/示例/DB 三处同步；identity.go/kernel（内置+用户级+~/.goink）"注入已去重"改"无去重每次 set_phase 注入全文"。

> 2026-08-10 提示词矛盾修正：identity.go/main-cmd-phase-gate.md/phase_gate.go 注释的"不自动推进，必须主动 set_phase"是 07-22 原文，8/9 加自动推进代码时未同步——改与代码一致（require 满足回合收尾自动推进+可主动 set_phase）。

> 2026-08-10 cacheprobe 注入 bug：runPlays 传 p.args JSON 给 injectPhaseOn，phaseInjectSkills key 是纯阶段名，阶段技能注入从未生效（仅 init 手动注入）——phaseOfArgs 解析修复 + batchCore 第 2+ 章 set_phase 补 readRequired；成本上修（单章 5 轮 0.7082→0.9588，批量 5 章 0.2464→0.2965）。

> 2026-08-10 set_phase 工具 description 残留"注入已去重"修正（最后一次 03:43 残留）。全量一致性核对通过：kernel 三源 hash 一致、review 白名单三处一致、无任何"去重/不自动推进"残留。

> 2026-08-11 命中率审计（600aa8c）：set_phase 成功消息静态化——原注入含 StatusString（called 工具列表逐次变化），8/9 批量每章 set_phase("write") 后每条动态消息注入历史中段，整前缀缓存失效（89-93% 掉到 86%）；改静态确认"已切换到 [X] 阶段"，工具结果已含 phase 无需 StatusString。模拟器 appendPhase 无此问题（模拟命中 95-98% 高于真机 86% 的差异正是该动态消息）。

> 2026-08-11 命中率数据构成（日志+DB 交叉验证）：真实口径 = model_usage 表 hit=18.08M/miss=3.13M（85.25%）；messages 逐请求审计 main=92.9%（138 请求）/review=90.9%（22 请求）；8/8 高命中（93-98.7%）是 mimo-v2.5（MiniMax 被动缓存）vs 8/11 deepseek-v4-flash（商汤磁盘缓存）的模型差异，代码结构相同（8/8 也有 10 个 review 子代理 98.5%）。日志 perModel 超 1M 后 %+v 输出科学计数法、context canceled 时读库失败照打假值——统计口径勿用日志 perModel，以 DB 为准。

> 2026-08-11 维护轮首请求全 miss 根因：非代码问题——DB 重建请求序列验证字节前缀 94-100% 连续（lcp 模型），但真机 hit=0（R3 连固定前缀都未命中），证明商汤 DeepSeek 缓存非字节 lcp 匹配，而是"公共前缀检测滞后一拍/完整单元匹配"：输入形态首次出现全 miss 一次（维护轮每轮首请求 130-160K），下一请求命中（命中量 ≈ 全 miss 请求输入）。35% miss（~1.1M）源于此，客户端字节已最优无法修复。NS 分层验证（稳定设定入固定前缀）模拟器收益仅 0.4%（《无限分身》简介为空，稳定部分 2 token），已回滚。

> 2026-08-11 维护轮降频（kernel+identity 三层）：maintain 必须一轮完成（7 查询并行 + 更新按依赖链聚合 + 一次性输出清单），禁止遗留待办等用户追问；identity 省 token 总纲加"轮次聚合"（每追加一轮 = 轮首全 miss 130-160K）。kernel 同步 ~/.goink/skills/ 即时生效，identity 编译部署（13-05）。

> 2026-08-11 前端：章节列表侧栏撑开修复（正文列表 min-h-0 + 分块标题 sticky 固定，滚动不跟随内容，同大纲列表行为）；提交 0c749b1/67a74e0。

> 2026-08-11 skill 吸收 human-writing（KKKKhazix 仓库，MIT）：main-tech-anti-ai-writing 八条→九条铁律，新增"禁止翻案腔（禁动作非禁字面）"（不是…而是…/表面…实际…/你以为…其实…/回头才发现/说到底及一切变形）；AI 腔补充加洞察路标（更微妙的是/还有一层/从某种意义上说）、三项以上同构排比、抽象名词配具体动词抒情、动词名词化、提示性冒号、借喻包装（仓库/抽屉/温度/浪潮/钥匙）；自查表加 3 项。只增不删，14-22 部署。

> 2026-08-11 全量 skill 审校修复（子代理审校+自查，1182aa1）：P0-字数下限统一 2500-4000（代码 MinChapterWords 默认 2500，kernel 两处 2400 错误）；P0-钩子轮换统一"不与前 2 章重复"（7 处"前 2 章"口径 vs 2 处"连续 3 章"冲突）；P0-黄金三章字数表 1500/2000 改 2500 起（低于门禁被拦）；P0-golden-three 金手指亮相内部矛盾（前 1000 字 vs 第 2 章）统一前 1000 字。P1-kernel outline/write/review 编号跳号修复；title-design 7 种→10 种（描述+标题）；kernel review 引用 16 项→22 项；sub-tech-anti-ai-grade 八条→九条引用；climax 铺垫 2000+4000 注明跨章累计；book-outline 冲击力 5 每 10 章→冲击力 ≥4 对齐 shuangdian 爽点频率；shuangdian 删除 7 型钩子轮换表改引用 hook-enhanced。P2 缺漏补全：foreshadow 单章新伏笔上限 1-3 + 红鲱鱼误导伏笔；climax 战后代价与收获闭环；shuangdian 单章内爽点位置（前 1/3 小甜头+主爽点靠后）；kernel 硬约束加"一章一事"。builtin kernel 回填"维护一轮完成"段（skills 版漂移），两版同步 + ~/.goink/skills/。

> 2026-08-11 扩写挤牙膏修复（ea17bbc）：word-count-calibration 加"一次扩到位"（缺口×1.2 目标 + 一次 read 一次 edit + 禁挤牙膏循环 + 集中扩写≥300字/点 + 缺口>30% 加段 + 扩写须信息增量）；kernel 原"无需读 word-count-calibration"改为"扩写时必须按一次扩到位规则执行"（原规则 AI 看不到是根因）。15-57 部署。

> 2026-08-11 NS 按需注入（36ebd0c，通用缓存优化）：NS 字节与上轮相同（进度/指纹未变，如维护轮）时不落库新 NS——新消息即使字节重复也计入 miss 尾部（所有 OpenAI 兼容模型通用）；写章轮正常注入。主 agent（chat.go）+ 子代理 fork（agent.go）双处。模拟器对照（cacheprobe nsondemand 子命令）：miss 降幅 4.8%（短对话 2 轮省 8K；真机 8 维护轮收益约 2 倍）。16-40 部署。

> 2026-08-11 技能注入去重（e15dc94）：injectPhaseSkills 注入前检查 opts.Messages 已含相同技能全文则跳过（仅标记 OnSkillInjected）；压缩重建历史后技能消息被清，后续 set_phase 自动恢复注入。业界对照（websearch：Claude Code 技能 cap 5K/skill+25K 总上限、Manus/Kun 的稳定前缀纪律）：技能注入占 miss 构成 26%（单章模式每章 5 次真切换重复注入全文）。模拟器对照（cacheprobe skilldedup）：miss 降幅 18.1%（506K→415K）+ 总输入降 17%。17-19 部署。

> 2026-08-11 缓存模拟器重构为模式驱动（a411a5a→14ce8f0 系列）：
> - **窗口刻度模拟**（a411a5a/ad24acc）：CLI `window` 子命令 + 设置面板「上下文窗口刻度」——单窗口历史增长到 128K/256K/512K/1024K 的累计成本快照 + 区间每章成本 + 最省区间；single（26 章≈1M）/batch（120 章≈1M）两模式
> - **合并进写书成本模拟**（f633c98/278f97c）：删独立 StartWindowSimulation，改为模式驱动 `StartCacheSimulation(mode, gateRounds, shortQARounds, batchChapters, batchRounds)`——single=每章完整门禁逐章累积 / batch=每批 6 章批次循环 / mixed=混合会话（批量可多轮循环、章号顺延）；`RunWindowMode` 单一入口，`cacheprobe.Run` 三方对照恢复原样；修复 marks 未到达元素 Threshold=0 显示"0K"、nil marks panic（5cd8fd5）、批末审稿回改 chapters/001.md 导致刻度章号反序（adb4f72，取最大章）
> - **混合模式批量轮次 + 章号连续**（b65128d/64c1ab3）：批量部分按 batchRounds 批次循环，base 偏移接在单章轮之后（不再从第 1 章重写、刻度不挤在单章轮）；label 显示"批量 5 章 × 3 轮"
> - **阶段轮次成本表**（14ce8f0）：mixed 模式改用阶段打点（`StageMark`：开书/短对话 N/单章 N/批量轮 N 每阶段结束的累计成本 + 区间增量 + 每章成本），替代上下文刻度——混合窗口大小由输入决定，刻度到不了大档位且反映不出工作负载结构
> - **性能 9 倍**（a11a7e0）：缓存 key 从 json.Marshal 结果改为轻量 msgFingerprint（拼接字段不序列化，原实现对全部历史消息每次请求重复 marshal 占 CPU 28.5%）；step 增量路径（lcp 覆盖上次请求末尾时直接复用 prevMsgsN + 上次字节，只处理新增消息；子代理 fork/legacy 剔除 NS/transform 时回退全量）；结果零变化（single 26 章 hit/miss 完全一致），60s+→6.9s
> - **UI**（563020d）：hint 移到 label 行、输入框可清空重输（NumInput 本地字符串 state）、按钮与输入框同一水平线

> 2026-08-11 技能注入短提醒（aeed97c→cdbdbc3，用户质疑"可见≠被注意"后业界验证落地）：
> - **可见性判定**（aeed97c）：模拟器去重判定从 injectedPhases 记录改为 `visibleIn` 全文比对（history+cur 中 role=system 且 content 相同）——压缩清理历史后判定失败自动重新注入，杜绝"记录还在内容没了"的误判；真机 agent.go 遍历 opts.Messages 同标准
> - **短提醒注入**（2a798b6 模拟器 + cdbdbc3 真机）：首次进入阶段注入全文（学习内容常驻历史）；再次进入同阶段注入 `BuildSkillsReminder` 短提醒（技能名+description 要点，~300 字符 vs 全文 8K，紧跟请求尾部注意力最强位置）——解决 Lost in the Middle（单章 5 轮时技能全文在历史 24.6% 位置，注意力衰减）；业界对照：Anthropic skills #591 index-page / hermes-agent system-reminder / autogen 300-token 修复；模拟 miss 降 13.8%（506,703→436,595）命中率 97.4% 不变

> 2026-08-12：写书成本模拟全面改造——① 对齐真机：RunWindowMode 支持上下文窗口（0.7×窗口压缩建模，阈值读设置自定义，如 95%）、门禁拦截建模（set_phase 25%）、thinking 分布采样、miss 构成（RunWindowMode 挂 missCat）；② 独立面板：从设置对话框迁出为顶栏「写书成本估算」全屏面板（两层结构：第一屏三档预设逐章精写 9 章/批量连写 18 章/边写边聊 3+2+3 + 每章成本柱状图 + 月成本卡 + 建议句；高级详情折叠：场景对比/miss 构成/单场景深挖）；③ 布局修复：FULLSCREEN_PANELS 隐藏 SidePanel 空壳（profile/cachesim 左侧空白），顶栏入口清 sidebarPanel 残留；④ 删除窗口刻度表（128K-1024K 打点与压缩建模结构性矛盾——1M 窗口 0.7 阈值下 1024K 永远未到达，改为峰值口径仍误导，整体移除；CLI runWindow 保留）；⑤ 预设语义修正（single 参数实为章数，轮/章标签纠正）；⑥ 命中率校准系数（sim_hit_rate_adjust 默认 0.95 持久化，模拟命中率 96-97% → 真机 89-93% 区间，输入总量不变 hit 转 miss、miss 分类等比缩放，TestApplyHitAdjust）；⑦ 模拟结果写 goink.log（cachesim done/failed 含窗口/命中率/成本/压缩次数）；⑧ 批量场景 API StartCacheSimScenarios（事件 cachesim:batch-done）。

> 2026-08-12：流畅性修复（真机测试断点专项）——① 注入路径加固：BuildSkillsContent 恢复 read_required 时代语义（nil store 防御 + 技能缺失跳过而非整体失败）、injectPhaseSkills 加 defer recover + panic 日志、OnSkillInjected 补 nil map 防御、chatImpl recover 不再静默（Wails 模式也记日志）；② 门禁自动推进重构：提取 autoAdvancePhase，LLM 收尾 break 前先检查推进（推进则 continue 继续循环，AI 阶段间无缝衔接），删除循环后兜底（旧实现注入后 Run 结束 → "AI 完成阶段就停"）；③ 402 加入短重试（2 次，商汤免费渠道实测 402 几分钟恢复，旧实现直接判死）；④ prepare 白名单补 create_story_arc/update_chapter_plan/update_writing_snapshot（AI 必查后补卷纲/快照/章节计划不再被拦，门禁配置示例同步）；⑤ 拦截降噪：同工具同阶段连续拦截 2 次后不再注入提醒消息（纯噪音）；⑥ 死循环检测窗口 4→6 轮（write 阶段密集只读轮误判卡死）。

> 2026-08-12 文档审计：cache-simulation.md/cmd-cacheprobe-README/phase-gate.md/TODO-P4 同步上述全部变更（模式驱动、阶段表、性能、技能可见性 + 短提醒）。

> 2026-08-13 前端崩溃修复（86fec21）：角色/地点面板 React #31 崩溃 —— LLM 经 MCP 工具把 abilities/tags 写成对象数组（{name,level,description}），前端按字符串数组渲染对象导致渲染失败；新增 parseStringArray（lib/utils.ts）规整为字符串数组（对象取 name ?? description），应用于 CharacterListView/CharacterGraph/LocationListView/LocationGraph。

> 2026-08-13 工具层数组规范化（dedfacc）：LLM 经 MCP 工具把 abilities/tags 写成对象数组（{name,level,description}）是角色面板 React #31 崩溃根因（86fec21 前端兜底）；本轮在 create/update character/location/item/lore 六工具入库前用 NormalizeStringArray 自动规整为纯字符串数组（对象取 name ?? description），非数组返回工具错误让 LLM 重试；schema 描述补正反例。

> 2026-08-14：批量审稿覆盖强化（P2 部分落地）——kernel（skills/ + builtin 备份）review 段 + 批量质量节奏段：run_subagent 报告**必须列出实际审读的章节清单**，主 agent 收到报告先核对覆盖范围，覆盖不全（漏章/只审开头）必须补审后才进入修复；已同步 ~/.goink/skills/（运行时 user 级优先）。机制说明：子代理不受阶段门禁管（门禁仅主 agent），审稿覆盖此前靠 kernel 软约束，现改为"报告清单 + 主 agent 核对"的可核验流程。

> 2026-08-14：review 阶段修复验证门（创作质量闭环）——single/batch review 块 require 加 check_story_consistency（default_phase_gate_config.go + 门禁配置示例.md 同步）：主 agent 修复审稿问题后必须亲自跑一次程序化核对（4 类 SQL：伏笔超期/角色断档/物品冲突/死者复出）才能进 maintain——修复动作从"无验证放行"变为"客观验收"，与子代理审稿（发现视角）形成双层；子代理不受门禁管，其核对仍靠身份提示词。

> 2026-08-14：skill 口径清理 + always 技能内置兜底——① kernel（skills/ + builtin 备份）去硬编码数量（"42 个"→"以目录为准"、阶段技能表标题"30 个"→去数字，审计遗留 #3/#4 闭环）；② main-core-ai-communication-standard 复制进 internal/skill/builtin/（审计遗留 #5：原仅用户级存在，新环境未同步时 always 注入静默缺失；内置兜底后同名优先级 novel > user > builtin 不变，用户级仍覆盖）；③ AGENTS.md 数量口径 43→44（37 auto + 5 manual + 2 always）；identity.go 系统提示词全文复核无功能性 bug（唯一措辞张力：L143"手动推进"与 L162"自动推进"并存，不改）。

> 2026-08-14：搜索点击白屏根因修复（日志体系首战告捷）——ErrorBoundary 捕获到 [ContentPanel] e?.getModel is not a function：ContentPanel.tsx 误用 CodeMirror 5 API（getModel，CM6 EditorView 无此方法），只在"搜索结果带高亮打开文件"路径（pendingHighlightRef 非空）触发 → 白屏。修复：存活检查 getModel → view.state；doHighlight 加防御（view 已销毁/空文档跳过、matchPos 越界 clamp 行号、dispatch try/catch）。顺带修复 rag/vector_store.go FtsCount 用 rows.Scan 未调 Next（"sql: Scan called without calling Next"）→ 改 QueryRowContext，老库 FTS 缺失检测恢复。

> 2026-08-14：字数扩写"一次扩到位"约束落实（真机挤牙膏复盘：第 3 章扩写 5 次 search_replace + 4 次 get_chapter_list，kernel 有规则但 word-count-calibration 是按需技能、LLM 从未 read）——三处叠加：① write 阶段 auto_skill_injection 加 main-tech-word-count-calibration（single+batch，default_phase_gate_config.go + 门禁配置示例.md 同步，切换进 write 即注入全文）；② kernel（skills/ + builtin 备份）write 段改写为自包含规则（缺口×1.2 目标 → 一次 read 一次 edit → 立即复查，重复时以当前缺口×1.2 为准，禁止小步挤牙膏）；③ agent.go 两处批量章边界字数拦截消息（wcMsg）附"一次扩到位"操作指引。生效前提：设置页门禁 Tab 点"恢复默认"（DB 已存旧配置不会被 seed 覆盖）。

> 2026-08-14：门禁 require 按阶段重置（真机阻塞复盘）——SetPhase 真切换时清空 calledTools/successfulTools（internal/agent/phase_gate.go），require 语义回归"本阶段内必须调用"，与 readsByPhase 技能按阶段隔离对齐；修复批量第 3 章 maintain 的 goink.md 指纹（edit）被轮末自动推进提前推到 done 白名单冻结、set_phase 回退后又被立即推进的死循环；5 个依赖跨阶段累计的测试更新（write 回退/第二轮前补本阶段 require）+ 新增回归测试 TestMaintainRequireNotPrefilledByWritePhase。

> 2026-08-14：前端错误可观测体系（真机白屏黑盒修复）——① 新增 Wails 绑定 LogFrontendError(message, stack)（app/handler.go，slog ERROR 落 goink.log）；② 新增 frontend/src/lib/errorLog.ts：installGlobalErrorHandlers 安装 window error/unhandledrejection 全局钩子（main.tsx 挂载前调用）+ logFrontendError 上报；③ shared/ErrorBoundary 增强：componentDidCatch 上报 message+stack+componentStack 到 goink.log、失败界面加"重新加载"（原仅 console.error），新增 label prop；main.tsx 根级包裹 ErrorBoundary（保底），WorkspaceView 的 ContentPanel 区域包裹（崩溃重灾区）；前端渲染崩溃从"全屏白屏无痕"变为"错误界面 + goink.log 可查堆栈"。

> 2026-08-14：prompt_cache_key 自动降级（自定义端点 400 修复）——internal/llm/stream.go ChatStream 重构为 send 闭包：首次收到 400 且错误体含 prompt_cache_key/Unsupported parameter 时，进程内记忆该 provider（包级 sync.Map）并去掉参数重发一次，后续请求不再发送；修复英伟达 stepfun 等不忽略未知参数的自定义端点对话不可用；内置 provider 缓存路由行为不变；新增 TestPromptCacheKeyAutoDowngrade（httptest 验证降级重发与记忆生效）。

> 2026-08-14：真机 DB/日志排查（会话 sess_2_18cba7e58a206358 全链路）——① 修复叙事面板物品名缺失：app/writing_context.go 两处把 map[int64]bool 直接传 id IN ? 致 GORM 参数类型错误（日志 "id IN \"map[21:true]\""，当前卡物品/写前预览物品流转名称为空），改 []int64；② build.ps1 补 sqlite_fts5 tag（mattn/go-sqlite3 缺该 tag 时 FTS5 模块缺失，真机日志 "no such module: fts5" 全文检索被禁用，搜索降级向量+LIKE）；③ 观察项：deepseek-v4-flash 会话命中率 67.8%（6 次全量缓存未命中，服务端缓存驱逐/路由波动，客户端前缀稳定无告警）；④ 遗留：自定义端点（英伟达 stepfun）发送 prompt_cache_key 被 400 拒绝致对话失败，待 provider 级开关方案。

> 2026-08-15：质感体系（finish）落地——四预设 plain/aura（氛围光：多光源渐变+主色调投影+噪点）/glass（局部毛玻璃）/paper（纸纹）；--fin-shadow-*/--fin-glass-*/--fin-texture token，Tailwind v4 @theme inline 把 shadow-*/backdrop-blur-* 工具类映射到 token（组件零改动，72 处组件阴影接入）；低性能模式开关（关 blur/纹理+阴影降档）；自定义主题 JSON 支持 finish 字段；theme-system.md 新增「八、质感体系」。

> 2026-08-15 移动端流式显示修复（mobile/app.js + chat.go/api_server.go/agent.go）：① 双通道双气泡——自己发消息时 SSE 与 WS 全局广播收同一事件流，WS started/thinking 会清空 DOM 并创建第二个 AI 气泡；加 selfStreaming 标志，发消息期间 handleChatEvent 直接忽略 WS 事件（旁观桌面端对话不受影响）② SSE 通道补 tool_call/phase_gate 处理（此前只有 WS 通道有，工具调用阶段移动端无任何反馈）③ 流式渲染改纯文本 + 60ms 节流（原每 token 全量 marked.parse 重渲染，长文卡顿且未闭合 markdown 结构渲染异常看着像内容缺失），done/error 才 marked 渲染 ④ thinking 默认展开 ⑤ ChatFromAPI 加 ctx 参数，事件发送改阻塞 select（原 select default 丢弃，慢客户端下 channel 满丢中间片段导致过程跳变），handleChat 去 close(events) 改由 ctx.Done 退出（close 后 send panic 风险）⑥ phase_gate 数据随 EventCallback/WS 广播传出（移动端可见阶段进度）。

> 2026-08-15：二维码改回移动端页面完整 URL（http(s)://ip:port/mobile/?token=xxx）——goink:// 自定义协议在系统相机/浏览器扫码时无 handler 无法连接；URL 格式让系统扫码直接打开页面，页面 JS 自动写入令牌（replaceState 清理地址栏），应用内扫码同解析，兼容 goink:// 与纯令牌旧码。

> 2026-08-15：API 认证二维码升级为完整连接串（goink://ip:port?token=xxx&tls=0|1）——新增 Wails 绑定 GetAPIConnectInfo（局域网 IP + API 端口 + 协议，app/handler.go）；桌面端设置页二维码携带连接参数；移动端扫码一次完成地址+令牌写入（localStorage goink_api_base 持久化，刷新不丢）并验证 /api/health，兼容旧纯令牌码；mobile/API.md 补协议说明。

> 2026-08-15：质感体系收敛 + 叙事画布实底——① 去除 paper（暖纸）预设（CSS/类型/UI/文档四删，存量 finish=paper 自动回退 plain）；② 动态叙事面板画布 .narrative-content 背景从"纯点阵网格透明底"改为 var(--background) 实色底 + 点阵网格（防 bg-layer 渐变透出，glass 预设下也不透明）。

> 2026-08-15：自定义主题质感层接管（修"自定义主题辣鸡"）——根因：--bg-layer-grad（全窗口背景渐变）与 --glow/--glow-strong（辉光）不随自定义主题派生，残留内置太虚青蓝调，与新配色打架。修复：index.css [data-theme^="custom:"] 兜底块补派生（primary 渐变 + 主色透明度发光，旧主题/手填 JSON 全部生效）；generateTheme 同步输出 3 键（71→74，themeColors.test.ts 同步）。

> 2026-08-15：build.ps1 部署补 runtime/ 全量同步（onnxruntime.dll + models/ + git/）——构建期 "未找到 ONNX Runtime 库" 是编译临时 exe 无 runtime 的误报；但部署目录缺 runtime 时向量检索与 bundled git 真实不可用（此前 D:\Goink\runtime 靠手动放置）。

> 2026-08-15：双端对话状态同步补齐（移动端/桌面端按钮与回合状态）——移动端 WS 事件 started/done/error 同步发送/停止按钮（此前桌面生成时按钮无变化，可并发再发）、stopChat 对 WS 同步流走 /api/chat/cancel；新增 syncWithDesktopState（进入聊天页与 WS 连接时查询 /api/sync/state，中途加入补流式气泡，同会话也补）；桌面端 chat:api_event 补 apiStreaming 状态（移动端生成时停止按钮点亮，此前发送/停止双灰）、done/error 正确结束 api turn（此前永远 streaming）、onStop 取消移动端会话而非桌面 sessionId；ChatInput isLoading 合并 apiStreaming。

> 2026-08-15：移动端用户消息即时同步桌面端——started 事件补 message 字段（app/chat.go），桌面端 chat:api_event started 分支用 ev.message 填 turn.userMessage（此前为空，用户消息要等 chat:api_done 重建才显示）。

> 2026-08-15：移动端 SSE 自流按钮卡死修复——根因：服务端 /api/chat 的 events channel 永不关闭（app/api_server.go 注释明说），done 后连接保持打开；移动端 sendMessage 收到 done/error 只渲染不跳出循环，永久挂在 reader.read() 上，按钮恢复代码永不执行（且 selfStreaming 屏蔽 WS done 双重卡死）。修复：done/error 置 finished 跳出 while + abortCtrl.abort() 主动断开服务端残留连接。

> 2026-08-16 默认主题调色（frontend/src/index.css light/dark token 层）：主色 #6b8fad→#4a7da6（提饱和降明度，按钮/链接在浅背景下更清晰），同步 ring/sidebar/bubble-user/accent/glow/narrative-border/particle 等全部派生 token；绿系提对比（success/tag-green/tool-green/status-ok/usage-ok 饱和+明度调整）；卡片层次（--card 0.62→0.8、--sidebar 0.85→0.92、--narrative-card-bg 0.6→0.75）；边框 0.12→0.15、次要文字 #4a6a80→#3d5a70；dark 背景渐变残留旧蓝修正为 dark 主色 rgba(161,196,214)。自定义主题 generateTheme 自生成全套 token 不受影响。

> 2026-08-16：聊天 UI 修复（用户截图评审后拍板"都修"）——① **工具卡片按轮合并**：渲染层对连续同名同状态的普通工具段分组（同一轮并行调用如 get_lore ×4 合并为一张卡 + ×N 徽标，ChatPanel 渲染前计算 toolCounts/toolMergeIds，ToolCallCard 加 count prop），消除重复卡片堆叠；② **「底部」滚动按钮改右下角**（sticky 容器 justify-end + pointer-events 穿透），不再遮挡消息内容；③ **diff 标签独立样式**（TabBar 加 FileDiff 图标 + 琥珀色 tag-amber-foreground + italic 保留），与正文标签可区分；④ 完成徽标加 ✓ 图标 + 加粗、卡片 opacity 0.85→1（对比度修复）、新增 .tool-count 徽标样式。前端 62 测试全过。

> 2026-08-16：**写书成本模拟 GUI 同步新能力**（用户追问接口）——① SimScenarioReq 加 effort（推理深度 low/high，空=low）与 hist_chapters（续写场景历史章数），StartCacheSimScenarios 支持续写场景（hist>0 走 runContinueSimulationSync：RunContinue + 价格估算，无窗口刻度，Label 区分续写单章/续写批量）；② runCacheSimulationSync 加 effort 参数（SetSimEffort + defer 恢复 low，simMu 串行防串扰）；③ 前端 CachesimView：高级详情加「推理深度」下拉（low/high，作用于预设与全部场景）、默认场景集加"续写单章（历史 3 章）""续写批量 5 章（历史 4 章）"、场景编辑器加「历史章」输入。bindings 透传无需重新生成。

> 2026-08-16：**模拟器定位明确：严格门禁阶段规范基准**（用户拍板）——plays 建模"LLM 严格按门禁配置与 kernel 规范执行"的应有成本；真机低于模拟 = LLM 偷工减料（软约束失效、缺大纲、miniMaintain 2/6），高于模拟 = 发癫（抽风思考、拖沓重试、人工介入），个别 LLM 发癫行为不纳入模拟，对标时先剔除（如批量 ~35K 抽风输出）。

> 2026-08-16：批量 5 章真机验证（sess_2_18cc3abdae039518）发现两个软约束失效缺口并硬约束化——① 每 3 章轻量自检完全未触发（kernel 软约束，LLM 不遵守）→ agent.go 章边界注入自检提醒（handleBatchChapterBoundary，已完成章号 N%3==0 时注入状态对照+一致性+文笔自检提醒）；② miniMaintain 六件套只执行 2/6 件（require 按阶段累计，第 1 章满足后后续章不调也能转出，真机确认）→ SetPhase 批量 write 章边界（同阶段）检查 missingMiniMaintain，缺件拒绝声明下一章 + ResetPhaseCounts 按章重置 require 与字数状态（每章独立结算，含最后章转出 review）。新增 TestBatchChapterBoundaryRequiresMiniMaintain/TestBatchChapterBoundaryResetCounts；模拟器 miniMaintainPlays 每章已建模（对齐硬约束后真机）。

> 2026-08-16：模拟器首轮固定前缀被动缓存建模（真机对齐第三次校准）——真机 mimo-v2.5 批量 5 章首轮输入 34.3K 命中 28.7K（83.7%，MiniMax 对固定字节前缀有服务端被动缓存），模拟此前首轮全 miss 高估 ~30K/会话。新增 SimFirstHitRatio（默认 0.84，CLI -firsthit 可调，DeepSeek 场景可设 0），step() 首轮分支对 fixed 类消息+tools 按比例拆分 hit/miss 并同步 MissByCat；新增 TestFirstRoundHitRatio。建模后批量 5 章 miss 184.7K→159.1K 与真机 167.0K 差 4.7%，命中率 95.7% vs 真机 98.3%（剩余差距 = mimo 每轮新增固定内容的被动缓存，字节前缀口径未建模）；输出口径差异（真机 60.7K vs 模拟 23.7K）为审稿拖沓真实行为非失真。

> 2026-08-16：会话管理统一（方案 A，用户拍板）——① 移除消息区「最近 5 条」列表形态（RecentSessions.tsx 删除），顶部「历史」为唯一会话入口（SessionHistory 抽屉：搜索+分页+批量删除+**当前会话高亮**）；② 「新对话」直接进入空白输入页（欢迎引导卡片），不再先展示历史列表；③ 启动自动恢复上次活跃会话（读 settings.last_session_id → GetSession 校验 novel_id 匹配后 handleSelectSession，仅首次挂载，切小说保持清空）；④ activeSessionId 语义统一为 null（原 undefined/null 混用）；⑤ 清理 sessions state 与各处 GetSessions 刷新死代码（chat:done 保留当前会话消息刷新）。

> 2026-08-16：消息 usage 落库时序修复 + reasoning_effort 会话恢复 + 模拟器四校准（真机批量会话深挖闭环）——① agent.go updateUsage 原在 appendMsg assistant 之前执行，UpdateMessageUsage 按"最新 assistant 消息"定位恒命中上一条 → 每条消息 usage 滞后一个请求、末条丢失（消息级审计不可信，model_usage 表才是权威）；且无工具调用轮（纯文本回答）完全不走 updateUsage，计费不累计（turn 5/6 无记录即此因）。修复：updateUsage 挪到 appendMsg 之后 + 无工具调用分支补调。② 前端 ChatPanel 切换历史会话恢复该会话持久化的 reasoning_effort + 思考开关（sessions 表按会话保存，此前只恢复 title/usage，换窗口后 effort 掉回全局默认），带当前模型合法性校验。③ 模拟器四校准（internal/cacheprobe）：outlineText 加长到真机量级（5 章大纲单请求 10,986 token ≈ 2.2K/章，模拟 ~200 字符 → ~1.6K）、批量审稿子代理改读全批正文（simulateSubagentChapters，真机 fork miss 14.4K）、Run 开头重置 simPhase（table 多场景串跑 thinking 阶段串扰）、thinking 支持 -effort high 档（基数 ×3，review 2100 锚定真机 read 请求 comp 2.2K）。校准后 -effort high 批量 5 章 miss 171.2K vs 真机 167.0K（差 2.5%）。**真机输出 60.7K 含模型抽风**（yjm 中转 mimo-v2.5 思考重复循环 + 超长思考 ~35K token 占 58%，<think> 混入 content、reasoning_content 恒空 → thinking_content 落库 0 的根因），剔除后正常输出估计 ~25-30K，模拟器 51.6K 为规范口径上限；hit 2.4 倍差 = 真机带 4 章旧历史 + 1M 窗口的结构性差异（场景不同构，非失真）。

> 2026-08-16：模拟器新增**续写场景**（真机"带历史续写"建模闭环，用户提出）——既有场景全从空会话起步，与真机（已有 N 章历史）结构性不同构（单章 miss 差 2.2 倍/批量 hit 差 2.4 倍）。实现：api.go 加 runInitRounds/runGateRound/runBatchRounds + buildContinueSingle/buildContinueBatch + RunContinue（历史轮结果丢弃，output/miss 分类从历史基线截断，只统计目标增量）；sim.go reviewPlaysBatch 加 base 偏移、batchCore prepare 改 base+1、新增 batchLightEndReviewBase。table 加"续写单章（历史 3 章）""续写批量 5 章（历史 4 章）"两行。实测（low 档）对标真机：续写单章 out 13.2K vs 第四章 12.8K（差 3%）；续写批量 miss 124.1K vs 167.0K（真机高 26% = 抽风重试/拖沓）、out 27.8K vs 剔抽风 ~25.7K（差 8%）。high 档思考基数 ×3 对简单续写偏高（真机 mimo 思考强度介于 low/high 之间，简单任务接近 low）。

> 2026-08-16：**移除门禁拦截行为建模**（用户"不要把发癫行为放进来"）——RunWindowMode 原先默认 simGateBlockRate=0.25（真机 set_phase 失败率 25%：LLM require 未满足即强行切换），属不规范行为建模，已删除启用点（默认 0 恒放行，代码保留供对照研究）；phaseThinkCharsHigh/outlineText 注释标注"剔除抽风消息后校准"（大纲 11K 为规范产物保留）。window/设置面板成本口径变为纯规范基准。

> 2026-08-16：续写场景**请求数爆炸修复**（用户质疑"差别巨大"后深挖）——首版续写场景
> 循环逐 play 串行（每工具调用一次请求），未走 runPlays 分组并行（≤10 调用合并一请求，
> 对齐真机并行行为）→ 请求数放大 5 倍（单章轮 89 vs 真机 18、批量 182 vs 72）、hit 虚高
> 数倍（批量 59.8M vs 真机 9.78M，成本 ¥1.445 vs ¥0.414）。修复：runInitRounds/
> runGateRound/runBatchRounds 全部改走 runPlays（分组 + onSubagent + phaseInjectOn）。
> 修复后：续写批量每章 ¥0.0978 vs 真机 ¥0.083（真机低 18% = 批量会话跑在硬约束化前，
> miniMaintain 2/6/自检未触发）；续写单章 out 差 3%、成本 ¥0.261 vs ¥0.104（真机低
> 2.5 倍 = 第四章旧版软约束精简流程 20 工具调用——模拟器预测严格门禁下的应有成本，
> 真机偷工减料不在对标误差内，新版硬约束真机单章待测）。

> 2026-08-16：门禁并发清理 + 批量配置审查——① 删 Agent.phaseGate/phaseGateMu/getPG/setPG 死代码（pg 已 Run 局部化，共享字段残留零调用）；② 删 RunSubAgent 的 pg 参数与 savedPhaseGate 残留（子代理不受门禁管，参数从未使用）；③ 批量门禁配置审查：write require 含 miniMaintain 六件套（create_scene/update_character/create_timeline_entry/update_timeline_entry/create_item_occurrence/update_writing_snapshot）+ loop 循环 + 每章 set_phase("write") 字数校验提醒；观察点（不动）：batch write require 按阶段累计不按章，第 2-N 章 miniMaintain 靠软约束。

> 2026-08-16：事件通道会话隔离（并发串台真修复）——turn_id 按会话独立递增会碰撞，桌面端 agent:{turnID} 事件名改 agent:{sessionID}:{turnID}（agent.go + ChatPanel 订阅同步）；chat:api_event 全事件补 session_id（chat.go EventCallback + done 事件）；桌面端 api 回合 id 改 api-{session}:{turn}；移动端 WS 按 ev.session_id 过滤非当前会话事件（started 切换信号除外）。

> 2026-08-16：事件通道会话隔离补漏（usage/压缩事件串台回归修复）——08-16 事件名改 agent:{sessionID}:{turnID} 时只改了 agent.go + ChatPanel 订阅，tokens.go（EventUsage 推送）与 compress.go（emitCompression 压缩事件推送）仍用旧名 agent:{turnID} → 前端收不到 → 状态栏 token 统计与压缩阶段 UI 静默缺失。修复：两处事件名对齐带 session_id，emitCompression 签名加 sessionID 参数（4 调用点传 opts.SessionID），app/chat.go 过时注释同步；go build + internal/agent 测试通过。

> 2026-08-16：重建"太虚·夜"示例主题 JSON——消除与内置 dark 的漂移（--card 0.72→0.6、--sidebar 0.78→rgba(13,20,32,0.82)、--sidebar-accent 蓝调→rgba(17,27,43,0.6)、--reader-paper #121a28→#111827）；删除已移除特效系统的 effects 遗留字段；补全 chart/usage/narrative/editor/bubble-ai 等扩展 token；新增 finish: "aura" 字段（质感层变量留空走派生，不锁死质感预设）。

> 2026-08-16：写时把关落地——① get_writing_context 新增 dead_characters 聚合字段（status=dead 角色名，写前防死者复出，工具描述同步）；② 门禁 single+batch write 转出 require 加 check_story_consistency（写完必须 SQL 实证核对四类硬错误才能进 review，current_chapter 必填防敷衍，default_phase_gate_config.go + 门禁配置示例.md 同步）；③ kernel（skills/+builtin+~/.goink/skills 三处同步）write 段加"写完核对"步骤、批量每 3 章自检加 check_story_consistency。

> 2026-08-16：cacheprobe miss 构成审计修复（update 45.9% 异常）——根因：8/9 阶段技能改系统注入（auto-inject）后，模拟器 plays 的 auto_skill_injection 工具调用残留（read_required 改名后 filterReadRequired/各过滤点条件未同步），技能全文被重复计数。修复：filterReadRequired 及全部过滤点匹配 auto_skill_injection（sim.go 5 处 + api.go 6 处 + 测试 5 处）、批量路径（buildBatchWithRounds/buildBatchLoopCompress）补过滤、missCatOf 按工具名分类（正文不再混入 outline）。修正后全场景 miss 降 8-23%（单章 1 轮 135.9K→93.3K、批量 5 章×2批 227.8K→175.1K）；README 表更新。遗留：批量查询 miss 随章数暴涨为真实行为（get_chapter_list 每章 2 次真实返回全量列表 + check_story_consistency），优化空间 = get_chapter_list 字数校验不需要摘要字段。

> 2026-08-16：批量查询 miss 随章数暴涨根因修正（用户质疑"写了 100 章就查 100 章？"）——**是模拟器失真非真实行为**：get_chapter_list 工具真实业务支持分页且描述强制"检查字数用 size=1"，但模拟器 plays 传 `{}` → 真实执行按默认 size=50 返回全量列表（含摘要）→ 查询 miss 随章数线性暴涨。修复：plays 全部 get_chapter_list 补 size 参数（prepare size=5 浏览、字数校验 size=1）。修正后批量 60 章成本 ¥4.54→¥1.78（-61%），批量每章成本随章数摊薄至 ~¥0.030 不再爆炸；README 表二次更新。

> 2026-08-16：cacheprobe 同步写时把关（建模镜随业务更新）——write 段 plays 补 check_story_consistency（writeBodyPlays 一处覆盖 single+batch 全部章，current_chapter 参数，结果 ~1K token），消除"门禁 require 已加但模拟器未建模"的成本低估漂移。

> 2026-08-16：会话历史删除后最近会话列表不刷新（ChatPanel.sessions 是 RecentSessions 数据源，SessionHistory 删除只刷新自身列表）——新增 onDeleted 回调（SessionHistory deleteSelected 成功后触发）+ ChatPanel refreshSessions，删除后立即同步最近会话。

> 2026-08-17 maintain 内容校准软约束落地（skills/main-core-writing-kernel.md + skills/main-tech-data-hygiene.md）：检查项 8/10/12/13 扩展校准语义——角色改 status 同步校准 description/personality（禁"预测性剧情"描述）；arc node target_chapter 与卷纲 detail_json 对齐（偏差>3章以 volume 为准）；伏笔回收/校准双动作（title/content/target_chapter 过时即修正，禁僵尸数据）；读者认知 create 前查重优先 update；阶段技能表 maintain 行登记 main-tech-data-hygiene（数据卫生：内容校准）。data-hygiene 修两处审计问题：规则三第 4 条改"update 标记 revealed_chapter 归档保留历史，物理删除走 delete_record"；自检清单第 5 项呼应 maintain require 强制 check_story_consistency。双份同步 ~/.goink/skills/（store.go ListMeta 每轮 Run 动态重扫，无需重启）。DB 门禁配置 maintain 段（single+batch）用户已在设置页落地：auto_skill_injection 含 main-tech-data-hygiene、tools/require 加 check_story_consistency（require 14 项）。

> 2026-08-17 data-hygiene 内置化 + 门禁默认配置同步（internal/skill/builtin/main-tech-data-hygiene.md + app/default_phase_gate_config.go + 门禁配置示例.md）：main-tech-data-hygiene 复制进内置 skill 目录（44→45 个，go:embed 编译进二进制，用户级/小说级删除后仍可从内置兜底）；defaultPhaseGateConfig maintain 段（single+batch）三处与 DB 当前生效配置对齐——tools/require 加 check_story_consistency（require 13→14 项）、auto_skill_injection 加 main-tech-data-hygiene，设置页清空恢复默认后不丢失该能力；门禁配置示例.md 同步。go build ./... 通过；新 exe 部署后内置生效。

> 2026-08-17 细度提升 + 门禁文档同步（internal/skill/builtin/）：main-tech-data-hygiene 加厚三处——规则二触发条件扩为"status 变更时 + 查询 A 顺带核对关键实体字段"（覆盖 status 未变但字段过时场景）并补全字段名（角色 description/personality/abilities、物品 description/lore）；规则三补"重叠判定"（同一事实/悬念措辞不同即重叠）与"合并操作"（新进展并入已有条目 content）；规则四补操作工具（update_arc_node 改 target_chapter）。main-cmd-phase-gate 同步 7 处过时点：require 表 maintain 13→14 项（+check_story_consistency）、批量说明"13 项清单"改"14 项 require + 15 项检查清单"、默认必读技能 write 补 word-count-calibration、maintain 补 data-hygiene（两处表格 + 段落）、maintain 行为要点提内容校准。skills/main-tech-data-hygiene.md 源目录副本由用户删除（内置化后不需要，git 历史保留）。go build ./... 通过；用户级副本即时生效，内置版下次构建生效。

> 2026-08-17 模型发现集成 models.dev 自动填充（internal/llm/discover.go）：DiscoverModels 函数在从上游 /models 端点获取模型列表后，对每个模型调用 LookupModelSpec 从 models.dev 全球模型数据库补充参数（ContextWindow/MaxOutputTokens/SupportsThinking/SupportsVision/ReasoningLevels），仅当上游未提供时填充，避免覆盖上游明确声明的参数。编译通过，前端无需修改（默认值逻辑保留作为兜底）。

> 2026-08-17 models.dev 离线缓存兜底（internal/llm/models_dev.go）：loadFromDisk 去掉 TTL 拒绝（磁盘文件不限年龄，只要能解析就加载）；ensureCache 重构为先从磁盘加载（不限年龄），再按需网络刷新——网络失败时保留已加载的磁盘数据，不再导致 cache=nil。首次成功获取后，即使 models.dev 永久不可达也能用本地缓存填写模型参数。编译通过。

> 2026-08-17 models.dev 模型 ID 匹配修复（internal/llm/models_dev.go）：LookupModelSpec 支持中转商组织前缀的模型 ID（如 deepseek-ai/DeepSeek-V3）——提取 / 后的 bareID 参与精确匹配；模糊匹配改为双向 contains（models.dev key/name 包含上游 ID，或上游 ID 包含 models.dev key），覆盖版本后缀和大小写差异。编译通过。

> 2026-08-17 模型发现跳过编辑表单（frontend/src/components/settings/ModelDiscoveryPanel.tsx）：handleImportSelected 拆分导入逻辑——context_window 和 max_output_tokens 均大于0的模型（models.dev 已填充参数）直接调用 onAddModel 导入，不再进入 pendingImports 编辑表单；仅参数不完整的模型才显示编辑表单让用户手动填写。前端编译通过。

> 2026-08-17 会话级模型持久化（frontend/src/components/chat/ChatPanel.tsx）：handleSelectSession 打开历史会话时恢复 sessions.model 记录的模型——根据纯 modelID 匹配 AvailableModel.ModelName 切换 selectedKey，并缓存 restoredModel 供后续 reasoning_effort 校验逻辑使用（避免 React setState 异步导致的竞态）。前端编译通过。

> 2026-08-17 getLocalIP VPN 适配修复（internal/netutil/ip.go + app/api_server.go + internal/api/server.go + internal/webdav/server.go + internal/cert/cert.go + app/handler.go + frontend GeneralConfigTab）：新增 internal/netutil/ip.go 封装 NetworkInterface 结构体 + GetLocalInterfaces() + GetLocalIP()——三级选择优先级（LAN+非VPN > 非VPN > 任意）、VPN 接口名检测（tun/tap/wg/clash/v2ray 等）+ VPN IP 段检测（100.64-127.x、198.18-19.x、169.254.x）；4 处 getLocalIP() 统一改为调用 netutil.GetLocalIP()；app/handler.go 新增 GetLocalInterfaces() Wails 绑定 + 前端网卡选择下拉框（仅多网卡时显示）。编译通过。

> 2026-08-18 移动端 token 逐模型成本弹窗 + 设置页重构 + 门禁批量修复（7431b1d）：mobile/app.js 新增 token 用量弹窗按 provider+model 分组展示成本；mobile/index.html/style.css 新增 modal 组件样式；GeneralConfigTab.tsx 布局重构（单页合并 API+模型+主题）；phase_gate.go 新增 BatchCheckPhaseGate 批量章节门禁；internal/agent/agent.go 门禁入口适配；delete_tools.go 角色删除工具重构（禁删主角/主角名保护）；新增 anti-ai-writing/info-density/show-dont-tell 三技能内置。20 文件，+1427/-496 行。

> 2026-08-18 移动端 i18n + token 数据修复 + pacing/promise 检查（f37c840）：mobile/app.js 中文化全部 UI 文案（设置/对话/主题/Token 用量/小说管理）；appearance_tools.go 新增 CheckPacingGap/CheckPromiseFulfillment 两个 MCP 检查工具（+370 行测试）；api_server.go 修复 token 数据格式化；build.ps1 更新打包逻辑。9 文件，+871/-127 行。

> 2026-08-18 check_story_consistency 结果门禁实现（78a0748）：agent.go + phase_gate.go 实现 result gating——require 检查工具返回的 issue 中如有 level=critical/blocker 则门禁拦截不放行；phase_gate_test.go 扩展测试覆盖 critical 阻断场景（216 行新测试）。3 文件，+212/-61 行。

> 2026-08-18 outline 数据库 + MCP 工具（init_consistency P2）（13ea6b3）：internal/outline/store.go + types.go 新增 outline 数据库存储层（读写小说大纲条目）；mcp_tools/outline_tools.go 新增 6 个 MCP 工具（get/set/list/update/delete outline 条目）；migrate/migrate.go 新增 outline 表；writing_context_tools.go 适配 outline 数据。6 文件，+398/-2 行。

> 2026-08-18 init_consistency 开篇阶段检查（P3）（afed134）：appearance_tools.go 新增 CheckInitConsistency 综合检查（角色/地点/物品/大纲/世界观一致性），覆盖开书阶段；测试从 370 行扩至 506 行。2 文件，+231/-7 行。

> 2026-08-18 init_consistency 子检查完成 + 技能更新（6c63154）：appearance_tools.go 补全 CheckInitConsistency 剩余子检查逻辑；main-core-init-phase.md + main-tech-book-outline.md 技能文档同步更新（outline 工具用法、init_consistency 检查项说明）。4 文件，+85/-23 行。

> 2026-08-18 outline 工具加入 init 门禁配置（单章+批量）（015a102）：default_phase_gate_config.go init 段 tools/require 加入 outline 系列工具（get/set/update outline）。1 文件，+2/-2 行。

> 2026-08-18 agentcfg 引用从 book-outline.md 迁移到数据库大纲（e0244b8）：identity.go + novel_state.go 将系统提示词中对 book-outline.md 的引用更新为数据库大纲（outline database）。2 文件，+2/-2 行。

> 2026-08-18 check_story_consistency emoji 清理（3bc6068）：appearance_tools.go + 测试文件将 check_story_consistency 返回的 emoji 标记替换为标准 error level 文本。2 文件，+12/-12 行。

> 2026-08-19 outline 工具 LLM 白名单修复 + 过时引用清理（8c86d6c+）：identity.go mainAgentTools 补 5 个 outline 工具（get_outline/update_outline/create_outline_beat/update_outline_beat/delete_outline_beat）、reviewAgentTools 补 get_outline（此前工具注册了但 LLM 看不到，init 阶段无法写总纲）；writing_context_tools.go description 更新为数据库 outline schema；main-core-init-phase.md（builtin+skills）修复产出物描述/校验项/一致性表 6 处过时 book-outline.md 引用；main-core-writing-kernel.md（builtin+skills）硬约束更新；main-cmd-phase-gate.md 门禁描述同步；default_phase_gate_config.go init+outline 段 edit_paths 清理 book-outline.md（init 已用 update_outline 工具不走 edit）。7 文件。

> 2026-08-19 总纲结构化编辑面板（P2 前端）：app/outline_view.go 新增 6 个 Wails 绑定（GetOutline/SaveOutline/GetOutlineBeats/CreateOutlineBeat/UpdateOutlineBeat/DeleteOutlineBeat）；app/handler.go App struct 加 outline store + OnStartup 初始化；OutlineEditor.tsx 结构化编辑面板（4 字段 + 大爽点列表 CRUD）；ContentPanel.tsx 拦截 book-outline.md 渲染 OutlineEditor（替代旧 preview 模式）；useApp.ts 补 6 个方法导入。6 文件。

> 2026-08-20 代码审计清理（dead code + 文档同步）：删除前端死组件 frontend/src/components/chat/PhaseGateBar.tsx（从未被 import，门禁进度条实际渲染在 shell/StatusBar.tsx）；清理 internal/agent/display.go 8 个废弃工具映射（get_chapter_content/edit_chapter/create_new_chapter/get_creative_profile/update_creative_profile/get_character_memory/get_novel_info/lint_chapter，工具已被 read/edit/get_*/check_story_consistency 取代，registry 无此工具）+ chapterTools 死条目 + buildDisplay 死 case 分支；gofmt 修复 internal/agent/agent.go 4 处缩进错位 + rw_tools.go 1 处；文档同步工具数 60→69（architecture.md/token-injection.md 注释/main-cmd-phase-gate.md）；main-core-writing-kernel.md 流程描述对齐 default 门禁配置（maintain→done 终点，done 是刻意设计——完成一轮后停下而非无限循环回 prepare），同步三份 opens architecture.md 数据管线、phase-gate.md（循环重置/工作流/阶段链/故障排查）、README.md；main-cmd-phase-gate.md 同步 done 终点。9 文件，验证通过（go build ./...、go test ./internal/agent/...、前端 npm run build）。
