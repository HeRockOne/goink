# Goink Agent / AgentCfg 模块代码审计摘要

> 阅读范围：internal/agent 全部 go 文件 + internal/agentcfg 三个文件 + internal/agent/DESIGN.md（offset 200 至结尾）。
> 本摘要只报告事实与设计要点，不含改进建议。行号均以当前文件为准。

---

## 0. 模块总览

| 文件 | 行数 | 职责 |
|---|---|---|
| agent.go | 1114 | Agent 主循环 Run、子 Agent 调度、阶段技能注入、自动推进 |
| phase_gate.go | 714 | 阶段门禁 PhaseGate：解析、拦截、状态机、持久化 |
| compress.go | 447 | 上下文压缩（主/子 agent 两条路径）+ 保留消息重建 |
| tokens.go | 260 | token 计数、usage 统计/累计/持久化/推送 |
| interrupt.go | 39 | 中断时排干 tool_call_end 事件（flushInterruptedTools） |
| cancel.go | 58 | CancelManager 集合，按 key 注册/取消/清理 |
| safety.go | 100 | 死循环检测、toolOutput 构造、tool_calls 转 JSON |
| display.go | 345 | 工具/阶段中文展示文本、活动类别、章节标题查询 |
| events.go | 78 | AgentEventType / AgentEvent / AgentLoopResult 定义 |
| errors.go | 32 | FriendlyError 错误转用户友好文案 |
| phase_gate_test.go | 792 | 门禁状态机回归测试（含多轮完整流程、batch 循环、技能强制） |
| compress_test.go | 95 | retainMessages 排除 NS、persistCompression 消息顺序 |
| safety_test.go | 92 | 死循环检测边界 + toolPattern 拼接/排序/截断 |
| agentcfg/identity.go | 282 | AgentType、工具白名单、三种 Agent 的 System1 提示词 |
| agentcfg/novel_state.go | 65 | NovelState 快照构建（System3） |
| agentcfg/skill_catalog.go | 120 | SkillCatalog / AlwaysSkills 拼接 |

---

## 1. Agent.Run 主循环完整流程（agent.go:328-943）

### 1.1 入口与前置（328-421）

- MaxTurns 兜底：<=0 置 100（329-331）。Model 为空直接报错返回（332-334）。
- ctx = storage.WithTurn(ctx, SessionID, TurnID)（336）。
- 阶段门禁初始化（343-380）：pg 是 Run 局部变量，不再存 Agent 共享字段（339-341 注释旧实现会话串扰）。仅当 PhaseGateEnabled && phaseGate==nil && AgentType=="main" 才 parsePhaseGateFromMessages（344-351）；mode 默认 single。PhaseCurrent/PhaseCalledJSON 非空则 LoadState（355-357）。defer（360-379）：SaveState -> session.SavePhaseGateState；持久化门禁模式，回到 prepare 且 roundCompleted 时清空 mode（batch 复位，370-376，注释说明旧条件 VisitedCount()>1 恒假的问题）。
- 循环变量（382-393）：loopCount/fullResponse/responseBuffer/thinkingBuffer/isThinking/recentPatterns/failCnt/retryCount/blockCount/runningTokens。
- 工具定义全量发送（394-395）：registry.OpenAI(nil)（nil=不限制全量），allowed_tools 限制——为 Prompt Caching 设计。
- 工具 token 前缀（397-404）：json.Marshal(tools) 后 tiktoken 计数入 toolTokens。注释：压缩阈值必须计入工具定义，否则低估 10-20%，0.7 阈值偏晚中小窗口模型 context overflow。
- 前缀哈希监控（406-421）：computePrefixHash(messages,tools) 与上一轮(sessionID)比较，变化告警"缓存可能失效"+ computeSystemBlockHashes 诊断。

### 1.2 事件发射闭包 emit（423-458）

- eventSeq nil 时自建并回写 opts.EventSeq（子 agent 共享，428）。递增 *eventSeq、补时间戳、设 SubTaskID、按 EventCallback 或 wails.EventsEmit 分发（430-441），Broadcast 到 WebSocket（443-457）。

### 1.3 循环体（473-937）

(a) token 预算检查（476-498）：threshold 默认 0.7；used=sumRunningTokens+toolTokens；超 ContextWindow 比例触发：main 用 Compress（落库换版本），子用 compressInMemory（纯内存）。失败仅告警继续（495-497）。
(b) LLM 调用（500-505）：ChatStream(ctx,providerName,messages,tools,modelID,callOpts{CacheKey=SessionID})；ReasoningEffort 可选。
(c) SSE 事件消费（509-858）：EventThinking（523-529）置 isThinking/累积/emit；EventContent（531-544）先 ThinkingDone、重置 retryCount、累积、emit；EventToolCallStart（546-561）ThinkingDone+buildDisplay(PhaseSelected)+emit ToolCall phase=selected；EventToolCallEnd（563-810）见 1.4；EventUsage（810-813）只存 pendingUsage 覆盖（中途部分值只取最终值，防回跳/重复累计）；EventError（815-856）见 1.5；ctx.Done()（512-515）置 interrupted+flushInterruptedTools（排干待执行 tool_call_end，标记"操作被中断"）+break。
(d) 流结束（860-937）：无工具调用（861-876）持久化 assistant+autoAdvancePhase，推进则 continue（不 break——注释：旧实现 break 后兜底推进致"AI 完成阶段就停"用户被迫发"继续"），否则 break；有工具调用（878-905）① updateUsage(pendingUsage)一次 ② appendMsg assistant(+tool_calls/tool_displays) ③ 逐条 tool 结果 ④ 逐条 inject（role=user,system-reminder 包裹）；中断检测（907-909）；死循环检测（911-923）；清缓冲+loopCount++（925-929）；每轮末尾门禁自动推进（936，write 阶段 LLM 持续调维护工具不 break 时也要能推进）。
(e) 退出（939-942）：interrupted 返回 ctx.Err()，否则 (FinalText,ThinkingContent,TurnCount),nil。

### 1.4 EventToolCallEnd 工具执行段（563-810）

1. parseArgs+buildDisplay(PhaseExecuting)（568-569）。
2. set_phase 特殊分支（572-660）——见 2.5。
3. 门禁硬拦截（662-710）：CheckToolAllowed；技能缺失拦截（"必读技能尚未加载"）自动补注入当前阶段技能后重查放行（668-674）；仍不允许则 blockCount[phase:tool]<=2 才注入拦截提醒（降噪，680-683）、emit EventPhaseGate(ErrMsg)、toolOutputs 追加失败 tool、continue（685-693）；edit 路径检查（695-708）CheckEditPath(args[path]) 不允许则拦截+注入+失败 toolOutput。
4. 执行（712-808）：emit executing；构造 ToolContext（DB/Approver/EmitApproval/RunSubAgent 闭包传 pg/SkillStore/Messages/SearchService/WebSearch，720-743）；registry.Execute（744）；重 buildDisplay（completed/failed），web_* 成功并入 metadata（751-760）；emit completed/failed（761-767）；门禁记录（769-790）pg.OnToolCall、成功删 blockCount、auto_skill_injection 上报 skills 给 OnSkillInjected、get_chapter_list 上报 word_count_ok 给 SetWordCountOK；失败计数（793-801）仅 ErrKind==system 计，连 3 次注入禁用提醒；暂存 inject（804-806）。

### 1.5 EventError 重试（815-856）

- 可重试：APIError 429/402 或 Retryable（819-822，402 旧实现判死，实测商汤免费渠道几分钟恢复）。retryCount<10（823）；wait=retryCount*5s 上限 60s（826）；emit EventRetry；清空全部缓冲（835-838）；sleep 后查 ctx、重建 stream、continue streamLoop（839-844）。不可重试/超限：持久化 partial、emit EventError、返回（846-855）。

### 1.6 appendMsg（947-978）

- 构造 session.Message（ToAPI=AgentType=="main"，ToFrontend=role=="assistant"）；db.Create；opts.Messages append（注释：opts 必须传指针）。runningTokens 计数（968-977）。坑点：runningTokens 可能 nil（nil map 赋值 panic 曾致注入路径 panic、reads 不标记、set_phase 失败连锁），防御 if 判断（975-977）。

### 1.7 取消

- ctx.Done -> interrupted -> flushInterruptedTools -> break。CancelManager 由 RegisterCancel/UnregisterCancel/Cancel 代理（116-128）。

---

## 2. PhaseGate 阶段门禁（phase_gate.go）

### 2.1 数据结构（20-44）

- PhaseGate：phases、currentPhase、calledTools(含失败)、successfulTools(require 只看成功)、mode(single/batch)、active、wordCountOK *bool、visited、readsByPhase map[phase]map[skill]bool、roundCompleted。
- PhaseConfig：Name、Mode(空=两种适用)、Tools、Require、AutoSkillInjection、Next、FailNext(解析但未见使用)、Loop(batch write<->outline)、EditPaths。

### 2.2 解析（46-135）

- ParsePhaseGateConfig：正则 (?s)<!--\s*phase-gate-config\s*\n(.*?)--> 匹配所有块；parsePhaseBlock 按 key:value 逐行解析；只加载 Mode 空或等于入参 mode（62-66）；强制激活第一阶段 current=phases[0].Name、visited=[first]（73-83）。

### 2.3 拦截逻辑

CheckToolAllowed（479-518）：nil/未激活全允许；set_phase 始终允许；get/update_phase_gate_config 始终允许（495-497）；当前阶段 tools 命中且 isMutatingTool(edit/run_subagent/create_*/update_*/delete_*/remove_*，243-251) 时若 missingInjections 非空则硬拦截（事前技能强制，504-509）；不在列表则拦（515-517）。CheckEditPath（429-470）：EditPaths=="" 或 "*" 放行；精确/`dir/*` 前缀（/ 统一分隔符防 Windows \ 跨目录）/path.Match glob。checkRequireMet（197-204）：Require 中 successfulTools[req]==0 则 false。OnSkillInjected（166-177）：按 currentPhase 分组，阶段切换后从零开始。missingInjections（207-232）：支持通配符 path.Match。CheckTransitionReady（521-535）：Next 非空 && require 全满足 -> true,next。

### 2.4 SetPhase 状态机（321-407）

1. nil/未激活直接成功；2. target 未知返回错误；3. 同阶段切换直接成功（331-334；须 SetPhase 前记录 from，agent.go 588-591）；4. 进入 write 重置 wordCountOK=nil（336-340）；5. require 检查阻塞（342-355）；6. auto_skill_injection 检查阻塞（357-363）；7. write 转出字数强制（365-373：nil 要求先 get_chapter_list；!ok 要求扩写）；8. next/回退校验（375-394：target!=Next 时仅 batch Loop 回退 prevPhaseName 或在 visited 才允许）；9. 切换维护 visited（396-405：current.Next==target && wasVisited(target) 则 visited=[target] 且 roundCompleted=true，否则 append）。

### 2.5 set_phase 工具处理分支（agent.go:572-660）

- pg 为 nil 但应启用则重解析+LoadState（575-587）；from:=pg.CurrentPhase()（591）-> ok,warning:=SetPhase（592）。
- 成功（597-623）：OnToolCall("set_phase",true)；from!=target 才 injectPhaseSkills（真切换，同阶段批量 write 声明边界不重复注入）；batch 且 target=="write" 且字数未达标注入 get_chapter_list 提醒（607-612）；注入静态确认"已切换到 [X] 阶段"（619-620，注释禁 StatusString 因含 called 动态内容致前缀缓存失效、命中率掉到 86%）；emit EventPhaseGate+成功 toolOutput(Data.phase)。
- 失败（624-655）：warning 含 "auto_skill_injection" 则补注入当前阶段技能并重试 SetPhase，成功后继续（631-648，字符串耦合判定）；其余失败注入 {"success":false,error,current_phase} JSON+失败 toolOutput（650-654）。
- pg 为 nil：直接成功 toolOutput（658）。

### 2.6 持久化

- SaveState（631-646）：序列化 successfulTools/visited/reads 返回 (phase,JSON)。LoadState（658-700）：兼容新格式{tools,visited,reads}与旧格式扁平 map（旧格式 visited 恒重置 [currentPhase]）。SaveWordCount/LoadWordCount（649-654/703-714）。

### 2.7 autoAdvancePhase（agent.go:292-313）

- CheckTransitionReady 满足+next 非空+未在 next -> SetPhase(next)；成功 -> injectPhaseSkills(next)+注入自动推进提醒(user)+emit EventPhaseGate，返回 true。

### 2.8 ValidateGateConfig（261-315）

- 对 single/batch 分别 Parse。error：next 指向不存在阶段、require 引用 tools 外工具；warning：auto_skill_injection 技能不存在（通配符跳过）、tools 含 edit 无 edit_paths。

### 2.9 测试要点

- walkFullCycle prepare->outline->write->review->maintain->prepare（239-259）；回归 visited 重置后 prepare->write 被拦（263-280）仍可推进（283-297）；单轮回退允许（300-384）；batch done->prepare 重置（388-462）；maintain 13 项 require（513-565）；auto_skill_injection 阶段内强制（569-634）；事前技能强制（639-692，未读技能 edit/create_scene/run_subagent 被拦）；buildSubagentSkills 含 sub-* 不含 main-*（696-718）。

---

## 3. 技能注入 injectPhaseSkills（agent.go:237-285）

### 3.1 调用点
- set_phase 成功/失败重试（602,630,634）、门禁自动推进（304）、技能缺失拦截补注入（669）。

### 3.2 全文/短提醒策略
- pg 未激活或无 AutoSkillInjection 直接返回（243-249）。BuildSkillsContent 取全文（250-254）。
- 去重判定（258-277）：遍历 opts.Messages，若 role==system 且 content==全文 -> 已在上下文：BuildSkillsReminder 生成短提醒（技能名+要点 ~百 token）追加为 system 消息（注释：全文在历史靠前位置注意力衰减 Lost in the Middle，短提醒紧跟本轮请求尾部唤起遵循，262-268）；仍对非通配技能名调 pg.OnSkillInjected（269-273）门禁照常标记。
- 未在上下文 -> 注入全文（278）+OnSkillInjected（279-283）。注释"压缩重建后技能消息被清掉，后续 set_phase 时自动恢复注入全文"。

### 3.3 可见性判定
- 全文注入条件 = 历史中无相同 system 全文；短提醒条件 = 存在相同全文。通配技能名（含 *）不调 OnSkillInjected（269-271,279-281）。

### 3.4 健壮性
- defer recover 兜 panic 记日志（238-242，静默中断曾致"注入后对话停止无提示"）。runningTokens 必须非 nil（233-236 注释+appendMsg 975-977 防御）。

---

## 4. 压缩 persistCompression（compress.go）

### 4.1 触发
- main 走 Compress（落库+版本递增+DB 重建），子走 compressInMemory（纯内存）。

### 4.2 Compress 流程（80-153）
- 1.emit "compressing"（88）；2.generateSummary（90-93，原样复制 opts.Messages+末尾 compactionPrompt user 角色，全量工具 CacheKey=sessionID，与主循环共享前缀可命中缓存，46-76）；3.重建系统消息（95-108：AgentIdentity(MainAgent)/catalog/always/NovelState）；4.当前阶段必读技能全文补回 phaseSkills（110-121，注释：压缩清掉技能消息后同阶段 set_phase 走"同阶段直接成功"不注入，创作指导缺失）；5.persistCompression（123-127，见 4.3）；6.GetMessagesForAPI(newVersion) 重载+opts.ActiveVersion/Messages 重建（130-138，与 Chat() 同路径）；7.InitRunningTokens 刷 runningTokens（140-142）；8.emit "done"（144）。

### 4.3 persistCompression（229-322）事务内版本递增+全量写入
- 查 Session 并 ActiveVersion++（233-242）。写入顺序（全带 newVersion）：1.AgentIdentity（system,toAPI=true,259-261）；2.AlwaysSkills（非空，262-267）；3.SkillCatalog（269-273）；4.compressionReminder（user,toAPI=true,275-277）；5.<system-reminder>{summary}</system-reminder>（user,toAPI=true,279-281）；6.保留消息副本 apiMsgToMessage（283-288）；7.phaseSkills 全文（system,toAPI=true,290-294）；8.最新 NS 快照（system,toAPI=true,ExtraMetadata=NovelStateKindJSON,toFrontend=false，297-311，注释：NS 进历史永不清理；压缩后首轮 [fp][reminder][summary][retained][NS] 与压缩请求前缀不同属一次性重建，之后恢复完整匹配）；9.压缩边界标记（system,content="",toAPI=false,toFrontend=true,EventType=compression,313-316）。

### 4.4 compressInMemory（156-227）
- 不重建 AgentIdentity/NovelState；提取头部 system 不动；内存重建 sysMsgs+reminder+<system-reminder>{summary}+retained（186-194）；DB 只写一条压缩边界标记（197-210）；opts.SubAgentVersion++（216）。

### 4.5 retainMessages（380-442）
- 跳过前部 system；收集 user 下标；保留最近 maxUserRetain(15) 条起点 keepFrom；user>=minConversationTurns(4) 时最低留到倒数第 4 条（420-426）；排除 role==system 且以 NovelStatePrefix 开头的 NS 快照（430-435）；返回深拷贝（436-439）。

### 4.6 测试
- TestRetainMessages_ExcludesNovelState（15-37）；TestPersistCompression_NovelStateAtEnd 期望 [identity][reminder][summary][retained][NS][marker]，NS 倒数第二且 toAPI=true/toFrontend=false（39-94）。

---

## 5. tokens.go usage 统计路径（37-260）

- InitRunningTokens（16-29）：按 role 分组 tiktoken 计数。updateUsage（37-260）调用时机：main 循环流结束统一处理 pendingUsage；压缩后重新 Init。
- 分角色 detail（46-73）：system 用 fixedPrefix 精确值（52-59，读 session.ExtraMetadata.fixed_prefix_tokens）；user/assistant/tool 用 runningTokens 估算；overhead=apiTotal-detailSum（69-73）。
- 缓存命中累计（75-155）：从 session.Usage 读 accHit/accMiss/accCompletion+perModel（83-114）。双格式提取（116-148）：优先 OpenAI prompt_tokens_details.cached_tokens（键存在即按 OpenAI 语义 miss=prompt-cached，含 cached=0 全未命中，128-138）；fallback DeepSeek prompt_cache_hit/miss_tokens（141-148）。累加（150-154）。
- 持久化与模型级（156-197）：UpsertModelUsage 传增量值到 model_usage 表（166-172，modelID 缺省 unknown）；perModel[modelID]{hit,miss,comp}（175-184）；UpdateMessageUsage 按 agent_type 分开保存到各自 assistant 消息防覆盖（186-196）。
- usage 组装（207-211）：prompt/completion/total、cache hit/miss 累计、per_model、context_window、running_tokens、detail、detail_is_estimate=true、overhead_tokens。usage_ratio（233-237）：(sumRunningTokens+fixedPrefix+toolTokens)/ContextWindow*100 本地估算单调不回跳；cache_hit_ratio（238-240）：accHit/(accHit+accMiss)*100；持久化 session.Usage（243-247）；仅主 agent 推前端（249-252,子 agent 运行期间占用显示保持主会话值）；全 miss 告警 hit=0 && miss>10000（224-230,MiniMax 被动缓存调整过期时间）。

---

## 6. 辅助文件

### 6.1 cancel.go（58 行）
- key 前缀 chat:/style:/pattern:（9-13）；Register/Unregister(只删不 cancel)/IsRegistered/Cancel(delete 并 cancel)，mu 保护（16-58）。
### 6.2 interrupt.go（39 行）
- flushInterruptedTools（10-38）：ctx.Done 后排干 EventToolCallEnd，不执行标记 toolOutput{false,"操作被中断"}，buildDisplay(PhaseFailed)，default 非阻塞。
### 6.3 safety.go（100 行）
- readOnlyTools（11-23 含 read）；toolOutput resultJSON（34-47）；buildToolCalls（50-63，arguments 字符串非 map）；isStuckLoop（68-86）：最近 6 轮+loopCount>=6，uniq<=2，当前轮全只读才判死（注释 6 轮窗口因 write 密集只读查询，4 轮会误判）；toolPattern（90-98，名字+args 截断 100 字符排序拼接 |）。
### 6.4 display.go（345 行）
- 映射表（11-146）；chapterTools（149-154：get_chapter_content/edit_chapter/edit/read）；phaseDisplayNames（157-165）；buildDisplay（178-275）：set_phase 显示流转、run_subagent 按 agent_type 定制+metadata、chapter 工具查 DB 标题、goink.md/outlines 特判、executing/selected 加"正在"前缀；chapterNumber（277-299）；lookupChapterBrief（305-323 去"第N章"前缀）；buildToolDisplay（325-345,web_* 成功附 result）。
### 6.5 events.go（78 行）
- AgentEventType（8-18）、AgentEvent 字段（22-45）、AgentLoopResult（48-52）、String()（54-78）。
### 6.6 errors.go（32 行）
- FriendlyError（12-32）：Canceled->空；401->API Key 无效；403->无权限；429->空（重试由 EventRetry 悬浮处理）；>=500->服务暂不可用；其余->对话出错请重试。

---

## 7. agentcfg 系统提示词结构与白名单（identity.go/novel_state.go/skill_catalog.go）

### 7.1 identity.go
- AgentType（6-10）：MainAgent/ReviewAgent/MemoryAgent。白名单（14-109）：mainAgentTools（17-49 全集约 40 工具含 set_phase/phase_gate/auto_skill_injection/edit/read/web_*/run_subagent/check_story_consistency）；reviewAgentTools（51-62 只读+审稿相关，无 edit/create/update/delete/run_subagent/set_phase）；memoryAgentTools（64-75 只读+get_writing_context+search_story_memory）；init() 转 map（77-87）；Allowlist（98-109）。
- AgentIdentity（116-127）返回三种 System1 常量。
- mainAgentSystem1（129-200）：【核心原则】维护优先/OOC 红线(auto 全自动自主推进)/长篇建卷/卷摘要；【创作流程】阶段流程 init->prepare->outline->write->review->maintain 手动 set_phase，write 用 edit 写 chapters/NNN.md(new_content 只正文)，review 单章每章启动 review agent，maintain 必须一轮内完成 15 项清单查询并行更新聚合；【阶段门禁】配置存 DB、tool/require、set_phase、自动推进、批量 write 每章 set_phase("write") 声明边界真切换才注入全文、压缩后补回（159-165）；【省 token 总纲】并行/最近 N 条/get_writing_context 全量/每轮聚合（实测 130-160K miss/轮）；【文件路径】【技能】mode 语义三层优先级命名铁律；【goink.md 维护】只记章节指纹追加式其余写 DB。
- reviewAgentSystem1（202-253）：子代理身份边界（无 run_subagent/edit/set_phase/字数校验）、只读、系统自动注入 sub-* 技能、审稿流程、🔴/🟡/🟢 输出与总体结论、质量第一。
- memoryAgentSystem1（255-282）：只读检索分析员、全景观测优先 get_writing_context、结构化报告。

### 7.2 novel_state.go（26-65）
- NovelStatePrefix="【小说基础信息】\n"（14，NS 落库识别前缀勿改）。NS 元数据常量（18-22）。NovelState(db,novelID)：查 novel 表->书名/类型/简介；当前进度锚点 chapter 计数+"创作须服务于全书总纲，只展开本卷，后续卷设定不得提前使用"（43-46）；goink.md 章节指纹 git.ReadFile(GoinkPath()) 取最近 maxGoinkChars(1500) 字符尾部截断（固定窗口字节稳定符合缓存协议，截断附说明，48-62）。NS 每轮轮末按需注入为 system，属动态字节在尾部，不影响缓存前缀。

### 7.3 skill_catalog.go（120 行）
- BuildSkillCatalog（12-73）：只列 auto 模式，按 novel>user>builtin 分组每组 - name: description，结尾附 read/edit 用法，<available_skills> 包裹；manual/always 不出现。BuildAlwaysSkillsContent（77-100）：只拼 always 模式，【常驻技能】头+"--- name ---"+sk.Content，非空返回 TrimSpace。与 AgentIdentity 同属稳定前缀区。filterBySource/filterByMode（102-119）。

---

## 8. 设计要点与已知坑（事实汇整）

### 8.1 文档与代码不一致（DESIGN.md 为过时声明，以代码为准）
- set_phase 分支、PhaseGate 拦截、autoAdvancePhase、压缩触发不在 DESIGN.md 伪代码中（DESIGN.md 289-291 仍标"占位 TODO"）。工具定义发送：DESIGN.md OpenAI(opts.AllowedTools)（204）；代码恒 OpenAI(nil) 全量+allowed_tools（agent.go 395）。死循环窗口：DESIGN.md "4 轮 <=2 种+turn>=4"（353）；代码 "6 轮+turn>=6"（safety.go 68-86）。to_frontend 规则：DESIGN.md 宣称 role==assistant 唯一（300-333）；代码压缩边界标记 role=system 却 toFrontend=true（compress.go 313-316,197-210）。Message ParentTurnID 顶层列（DESIGN.md 391-398,426）：代码 session.Message 未用，改用 TurnID+SubTaskID 路由子 agent（agent.go 948-960）。tokens 命名：DESIGN.md enrichUsage（435），实际 updateUsage。DESIGN.md 整体只到 439 行。

### 8.2 可疑/易错点
- agentDB 包级变量（agent.go 31-35）+getDB：parsePhaseGateFromMessages 依赖，New() 写入，多实例并发存在覆盖风险。FailNext 字段（PhaseConfig:41）解析但代码无任何使用引用（定义未被消费）。SetPhase 失败重试判定靠 warning 字符串包含 "auto_skill_injection"（agent.go 629），字符串耦合。LoadState 旧格式 visited 恒重置 [currentPhase]（688-695），跨 turn 回退仅新格式保留。compress 不持久化/恢复 wordCountOK：SaveState 未含、Load 未走此路径；SetPhase write 转出字数依赖压缩后重新 SetWordCountOK（但 persistCompression 补回 phaseSkills 保证技能不丢）。prefixHash 只哈希前导 system 全文+工具名（不哈希工具 schema/description，也不哈希尾部 NS），工具 schema 变化不触发前缀告警。批量 write 每章 set_phase 同阶段成功仍注入静态确认"已切换到 [write]"（619-620 刻意静态但会累加历史尾部，不破坏前缀）。RunSubAgent 子 agent opts.Messages 是新切片（subOpts 独立），内部 append 不污染父，但与父共享 backing array 前 len(parentOpts.Messages) 元素（读时不冲突）。

### 8.3 缓存协议一致性
- 全量工具+稳定前缀(identity+always+catalog)+动态 NS 轮末；set_phase 静态确认禁 StatusString；compress 重建顺序与 writeSystemMessages 一致；RunSubAgent fork 完整主历史+尾部子代理层命中主缓存；NS 尾部截断 1500 字符固定窗；toolTokens 计入压缩阈值。

### 8.4 PhaseGate 并发
- pg 全程 Run 局部变量+传入内部函数，不落 Agent 字段->并发会话互不串扰。共享字段仅 prefixHash（mutex 保护 52-54,408-421）。

---

## 9. 重点问题应答索引

1. Agent.Run 主循环 -> §1（初始化 328-421、emit 423-458、token 预算/调 LLM/SSE/压缩/取消 473-858、流结束/自动推进 860-937、退出 939-942、工具执行段 563-808、重试 815-856）。
2. PhaseGate -> §2.1-2.5（CheckToolAllowed 479-518、require 197-204、visited/SetPhase 375-405、Loop 回退、autoAdvancePhase agent.go 292-313、set_phase 分支 agent.go 572-660）。
3. injectPhaseSkills -> §3。
4. persistCompression -> §4.3。
5. tokens.go usage -> §5。
6. agentcfg 系统提示词 -> §7。

---
（行内引用文件均为 internal/agent/.go 或 internal/agentcfg/.go，行号为当前读取版本。）