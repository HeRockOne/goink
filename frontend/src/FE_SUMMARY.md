# Goink 前端源码结构化摘要

技术栈：React 19 + TypeScript + Vite，Tailwind CSS v4，CodeMirror 6（编辑器）+ react-markdown，Wails 生成 bindings（frontend/src/lib/wailsjs/），i18next（zh-CN/en）。

---
## 1. 应用状态管理与路由/视图切换

**无任何状态库（无 Redux/Zustand/Context 全局 store）**。全部状态为 React 组件本地 state + props 逐层下传（props-drilling）。Wails 后端为唯一数据源，前端是“无所不渲染的壳”。

### 1.1 启动流程与顶层视图
- main.tsx（11 行）：createRoot + StrictMode 挂载 <App/>，引入 ./index.css、./i18n。
- App.tsx（56 行）：手工枚举视图状态机 View='loading'|'init'|'workspace'。
  - 挂载时 app.IsInitialized()：true -> app.GetSettings() 取 last_novel_id -> workspace；false -> init；异常 catch -> init（L19-27）。
  - init 渲染 InitView，其 onInitialized（L33-38）取 settings.last_novel_id，置 fromInit=true 切 workspace。
  - workspace 渲染 WorkspaceView，传 initialNovelId 与 initialShowHelp=fromInit。顶层含 .bg-layer 背景 + sonner Toaster。
- views/InitView.tsx（194 行）：首启引导页，主题/语言选择，app.GetPlatform() 取默认数据目录，app.Initialize(dataDir)（L183）。

### 1.2 WorkspaceView——应用中枢（684 行）

持有几乎全部全局 UI 状态，props 下发各面板，并通过 ContentPanelHandle、ChatPanelHandle 两个 useImperativeHandle ref 反向调用。

关键状态（L59-101）：novels/activeNovelId/activePanel/sidebarPanel；一组 *FocusId；searchQuery/searchResults；phaseGateEnabled/gateStatus/tokenUsage/statusBarModel；activeChapterNum/currentVolume；布局（sidebarClosed/narrativeOpen/narrativeWidth/sidePanelWidth/chatPanelWidth）；大量弹窗状态。

路由/切换（L283-321）：
- SPECIAL_PANELS（L283）：characters/locations/storyarcs/timeline/reader/preferences/world/items/stats/profile/git/style-samples/cachesim —— 不走 ContentPanel 的独立渲染面板。
- FULLSCREEN_PANELS={stats,profile,cachesim}（L286）：全屏面板，隐藏 SidePanel。
- renderSpecialPanel（L288-305）switch 映射 panel id 到视图组件。
- 布局（L530-614）：ActivityBar -> SidePanel（可拖宽）-> 中间区（novels 时 BookshelfView，否则 ContentPanel + 常驻挂载 ExtractWorkspaceView(hidden) + renderSpecialPanel）-> 最右 ChatPanel（常驻，profile/cachesim 时 hidden 非卸载以不中断对话，L608-613）。
- StatusBar（L616）接收 content/gateStatus/usage/model。
- NarrativeTimeline（L669-681）以 .narrative-overlay 悬浮，right:chatPanelWidth。大量 Dialog 条件渲染（L618-666）。

SidePanel->ContentPanel 桥接（L226-246）经 contentRef.openFile。handleApprove/handleReject/handleApprovalFileEdit（L250-265）处理 AI 工具审批。首帧布局不稳兜底（L169-178）：强制 reflow + 写 --layout-tick 变量触发编辑器重测。

### 1.3 useApp()——Wails 方法封装

hooks/useApp.ts：useMemo 一次性返回约 130 个 Wails 方法（Key 列于 L133-260），全部从 @/lib/wailsjs/go/app/App 导入后重包一层。空依赖 useMemo([],[]) 返回稳定引用，避免子组件重渲染。签名来自 lib/wailsjs/go/app/App.d.ts。

---
## 2. Chat 面板与 SSE 事件消费（核心）

### 2.1 事件类型（components/chat/types.ts）
- AgentEventType enum（L4-14）与 Go 端 internal/agent/events.go 一一对应：Thinking=0, ThinkingDone=1, Content=2, ToolCall=3, Usage=4, Error=5, Compression=6, Retry=7, PhaseGate=8。
- AgentEvent（L27-50）：turn_id、seq（乱序重排用）、type、data、tool_name/id、phase、metadata、usage、compression_phase、retry_*、phase_gate、timestamp。
- Turn（L100-108）：{id,turnId,userMessage,segments[],status:'streaming'|'done'|'failed'|'interrupted'|'stopped',compressionOnly?}。
- TurnSegment（L53-80）：{type:'text'|'tool'|'subagent'|'compression', thinking内容, tool状态, approval, subagent嵌套}。
- rebuildTurns(messages)（L110-268）把 session.Message[] 重建成 Turn：中断标记（user_stopped/system_interrupted）按 turn_id 回设状态并 continue；compression 独立成压缩 Turn，带 sub_task_id 的压缩塞入子 agent 缓存段；子 agent 消息（agent_type!=='main' && sub_task_id）进 subagentCache，等 run_subagent 的 tool_display 出现时插回正确位置。parseToolDisplays(extra_metadata)（L279-290）解析 extra_metadata JSON 的 tool_displays。

### 2.2 ChatPanel 事件消费（1405 行）

双通道事件：
- 桌面 SSE（agent:turnID + chat:started）主路径：handleSend（L951-1053）先插本地 Turn（id:'turn_N',turnId=0），订阅 EventsOn('chat:started')（L984-1000），回调里拿真实 session_id、turn_id 回填，再动态订阅 EventsOn('agent:'+turn_id, handleAgentEvent(turn_id))，然后 await app.Chat({session_id,novel_id,message,provider_name,model_id,reasoning_effort})。finally 把 streaming Turn 标 done 并清理订阅（L1017-1052）。中途插话：activeCountRef>1 时先 app.CancelChat 并把旧 streaming Turn 标 stopped（L954-963）。
- 移动端同步：chat:done/chat:api_done 刷新会话（L165-183）；chat:api_event 实时流逐步拼 Turn（L185-293）；model:changed 同步模型/推理（L296-309）。

事件 seq 乱序重排（L38, L40-44, L810-867）：EventQueue{nextSeq,pending:Map<seq,event>,flushTimer}，eventQueuesRef=Map<turnId,EventQueue>。handleAgentEvent：有 seq 进 pending，按 nextSeq flush；乱序缺失时 EVENT_REORDER_TIMEOUT=120ms 定时器强制按 seq 排序 flush(force)。无 seq 直接 applyAgentEvent。

applyAgentEvent(turnId,event)（L439-808）按 type 分发：Usage->onUsage；Error->turn failed；Retry->setRetryInfo；Compression->更新/插入 compression segment（带 sub_task_id 时更新子 agent 卡内压缩段 L467-530）；PhaseGate->onPhaseGate；Thinking/Content->文本段增量合并（收尾段 isStreaming 则追加否则新建 L640-688）；ToolCall（L690-802）：set_phase 跳过显示，run_subagent 维护/新建 subagent segment 并移除同 toolId 残留 tool 段，普通工具按 toolId 查更新态/文本，awaiting_approval 取 metadata，file_edit 审批时构造 title 并 onApprovalFileEditRef 通知 ContentPanel 开 diff tab（L776-799），子 Agent 事件走 L543-634 分支路由到对应 subagent segment。

其他：handleSend finally（L1024-1052）清空 eventQueues、streaming->done、清订阅；会话列表 GetSessions page1 size5（L148）；历史 GetSessionMessages（L324）；handleCompress（L900-945）先插“压缩中”Turn，await CompressContext 后回填真实 turnId，经 useImperativeHandle 暴露 compress() 给 StatusBar；自动滚动仅用户未上滚（L373-379）；面板只拖左边（L334-356）；分页 visibleTurnCount 默认 30 每次 +50。

### 2.3 chat/ 子组件清单
| 组件 | 职责 |
|---|---|
| ChatInput(218) | 自动增高 textarea，Enter 发送，/ 触发 SlashMenu，停止按钮，字符计数 |
| SlashMenu(143) | createPortal 到 body 的斜杠浮层，模糊匹配(score)，类型图标(auto/manual/always)，键盘导航 |
| MessageBubble(112) | memo，user/assistant 气泡，Markdown，复制/编辑/重试悬浮按钮 |
| ChatControls(85) | 模型选择(PopSelect)+推理深度+自动审批开关 |
| ThinkingBlock(65) | 思考块折叠，流式结束自动收回 |
| ToolCallCard(~200) | 工具卡执行/完成/失败三态 + 审批视图 |
| WebSearchCard(103) | web_search 富文本：queries/sources/summary 折叠，外部链接确认 |
| WebFetchCard(87) | web_fetch：title/url/字数/内容折叠 |
| SubagentCard(157) | 子 agent(memory📝/review🔍)卡，嵌套 segments，流式自动展开/完成1s折叠 |
| CompressionBlock(43) | 压缩占位，compressing 计时/完成分隔线 |
| ContextRing(410) | token 用量圆环/条状 + 成本估算(cache/miss/out, per_model+selectedModel)、压缩阈值滑条、价格输入 |
| PhaseGateBar(68) | 六阶段进度条(init/prepare/outline/write/review/maintain) + 错误提示 |
| SessionHistory(308) | 会话历史：搜索/分页/批量选择删除/导出 Markdown |
| RecentSessions(62) | 最近会话列表 |
| RetryNotification(35) | 429 限流重试提示(retryWait 后消失) |
| types(290) | 事件/Turn 类型 + rebuildTurns |
| PopSelect(95) | 通用向上弹出下拉 |

---
## 3. 主要功能视图

### 3.1 书柜 BookshelfView（159 行）
纯展示+回调，novels 来自 WorkspaceView。网格卡片(BookCover)，悬浮封面更换(hidden file input)，hover 导出/编辑/删除。所有动作上抛回调。

### 3.2 内容编辑 components/content/
- ContentPanel.tsx（607 行）：forwardRef 暴露 {openFile,openFileWithHighlight,clearHighlight,closeAllTabs,openDiffTab,handleDiffApprove,handleDiffReject}。Tab 来自 useEditorTabs。激活 tab 上报 onContentChange/onDirtyChange。三种渲染：file tab(content/outline/preview/edit viewMode)、diff tab(ReactDiffViewer for chapter, Markdown for outline)。保存：handleEditorChange 500ms 防抖 doSave（L244-258），Ctrl+S 立即保存（L262-271），SaveContent({novel_id,path,content})。file:changed 事件（L296-345）用 ref 读 tabs 并跳过 isDirty。搜索高亮 openFileWithHighlight+doHighlight（rune 偏移转 line/col），pending 在编辑器 mount 消费。Ctrl+Shift+V 切换技能/goink 预览。THEME_MAP={light:'light',dark:'dark'} 传 CodeMirror theme（L34），theme change 时 dispatch({}) 刷新。
- content/types.ts：EditorTab(file|diff)、路径函数(chapterPath/outlinePath/goinkPath/isContentPath/isOutlinePath/isSkillPath/skillNameFromPath)、splitFrontmatter、chapterNumFromPath。
- hooks/useEditorTabs.ts（136 行）：Tab 状态+localStorage(goink_tabs_all 按 novelId 分组合并 TabMeta，beforeunload 写 L39-47)。novelId 切换标签集（L50-69）。模块级 let idSeq=0（L4）生成全局递增 id。

### 3.3 叙事面板 components/narrative/
- NarrativeTimeline.tsx（431 行）：7 类可拖拽/缩放/重命名 canvas 卡:current/past/future/arcs/foreshadow/reader/detailtabs。布局存 localStorage(narrative_canvas_layout)，卡标签 narrative_card_labels。数据源 app.GetWritingContext(novelId,ch)（L170-189）返回 ctx(preview 待写预览、writing_snapshot、recent_chapters、characters、timeline{pending,overdue,resolved}、active_arcs、reader、item_occurrences)。useOutlineCache 批量拉未来章大纲原文。自动刷新：EventsOn('file:changed'|'chat:api_done'|'chat:session_created') 300ms 防抖 + invalidateCache + loadContext 用最新章节号推进（L199-225）。手工拖拽/snapTo 8px 吸附/四角 resize/z 置顶。数据加工：activeCharIds 优先 characters_in(事实层)回退 active_chars(状态层)（L231-247）；cleanSummary 去“第N章,”前缀；chapterStatus 推 done/drafting/todo。
- DetailTabs.tsx（207 行）：角色/地点/物品/世界观/场景 5 Tab，GetCharacters/GetLocations/GetItemList/GetLoreList/GetSceneListByNovel；chat:api_done/chat:session_created 防抖刷新（L69-109）。场景按 chapter_id 分组。
- OutlineParser.ts（406 行）：纯正则解析 outlines/*.md。支持 ## 标题分节 + **字段**:兜底(005.md 格式)，提取 tone/字数/开篇策略/场景/关键事件/角色/伏笔(bury/advance/resolve)/情绪设计/章末钩子/金手指。章节标题行被跳过。大量中英全半角正则边界。
- useOutlineCache.ts（37 行）：ref Map 缓存 chapter->outline 原文，invalidateCache(chapterNum?) 全清或单清。

### 3.4 设置 SettingsDialog（80 行）
880x700 弹窗，左侧 4 Tab:general/model/theme/phasegate。Props 带 initialTab(WorkspaceView 传 'general')。遮罩点击关闭。

### 3.5 缓存模拟 components/cachesim/
- CachesimView.tsx（606 行）：写书成本估算。三档预设(逐章精写/批量连写/边写边聊 L79-87)+高级详情(场景编辑器/命中率校准/窗口刻度/3 Tab 对比)。StartCacheSimScenarios(PRESETS.map(...))，结果经 EventsOn('cachesim:batch-done') 推送（L120-132）。CacheSimResult/CacheSimStage 本地重复定义(注释:后端未生成 wails 模型 L7-25)。命中率校准 SetSimHitRateAdjust（L434-446）；价格从 GetSettings 读 price_input/output/cache_price。建议句(L185-203)算最省/最贵档。
- CacheSimDeepDive.tsx（291 行）：单场景深挖(single/batch/mixed)，NumInput(本地字符串 state+blur)，mixed 展示阶段轮次成本表。本地定义结果类型。

### 3.6 其他主要视图
- StatusBar.tsx（171 行）：左下字数/行数(computeStats 统计中文/英文/段落)+门禁阶段条+右侧 ContextRing 条状+dirty 灯+压缩按钮。统计逻辑与 ContentPanel.wordCountText 重复。
- CharacterListView.tsx（307 行）：角色列表+CharacterGraph，本地表单 CRUD，Get/Create/Update/DeleteCharacter。能力数组 parseStringArray(c.abilities) 规整 LLM JSON。
- LocationListView.tsx（395 行）：地点列表+关系图，父子级(parent_location_id+clear_parent)，类型标签配色。
- SearchPanel.tsx（218 行）：SearchAll(novelId,q)(300ms 防抖+reqIdRef 防竞态)，结果按 type 分组(content/character/location/chapter/timeline/storyarc/rag)，键盘导航，章节跳转带 match 高亮、实体跳面板。
- ActivityBar.tsx（73 行）：15 个固定图标导航(search/novels/chapters/preferences/characters/locations/storyarcs/timeline/world/items/git/reader/skills/stats/style-samples)。
- SidePanel.tsx（157 行）：按 activePanel 分发左侧子列表(SearchPanel/NovelList/ChapterList/CharacterList/.../StyleSampleList)，宽度可拖。

---
## 4. Wails Bindings 使用模式

### 4.1 方法调用(App.xxx)
- 全部从 @/lib/wailsjs/go/app/App、@/lib/wailsjs/runtime/runtime 导入。App.d.ts 注明自动生成禁止手改，需 wails generate module 后再 npm run build。
- 统一经 useApp() 包装，返回 Promise。常见形状：app.Chat(input):Promise<app.ChatResult>；CompressContext 返回 {turn_id}；创建/更新类返回实体或 void。
- 窗口控制直接 window.WindowMinimise/ToggleMaximise/Quit(WorkspaceView L505-524)，窗口状态经 useWindowState 用 WindowGetSize/Position/IsMaximised 持久化 localStorage(beforeunload 保存)。

### 4.2 事件订阅(EventsOn)
标准模式 const cleanup=EventsOn('name',cb); ... return ()=>cleanup()，每 effect 各自订阅并注销。命名事件汇总：Agent 流 chat:started、agent:${turn_id}(动态)；完成/同步 chat:done、chat:api_done、chat:api_event、chat:session_created；模型 model:changed；文件 file:changed；导入 import:progress；模式提取 pattern:progress；缓存模拟 cachesim:batch-done、cachesim:done。

### 4.3 类型
所有 Go 结构体在 @/lib/wailsjs/go/models(novel/chapter/character/location/storyarc/timeline/reader/skill/lore/item/stats/session/search/git/config/llm 等)。前端在 useApp re-export 命名空间(L263)。

---
## 5. 主题系统(CSS 变量)

全部在 index.css(1216 行)。@custom-variant dark(&:is([data-theme="dark"] *)) 把 Tailwind dark 变体绑定 data-theme。:root 定义 --radius/--font-body/--font-size。@theme inline 把 CSS 变量注册为 Tailwind 色彩 token(--color-*)并派生 radius 阶梯。

- 浅色 :root,[data-theme=light] “太虚水墨”：青蓝+玉白(--background #eef3f8, --foreground #0e1a26, --primary #6b8fad)。
- 深色 [data-theme=dark] “太虚夜色”：深蓝黑+剑宗青(--background #0a0e17, --foreground #e8eef2, --primary #a1c4d6)。
- 两域都定义完整语义色：card/primary/secondary/muted/accent/destructive/border/input/ring、sidebar、tag-*(blue/green/amber/rose/teal/purple)、tool-*、bubble-*、chart-1..5、contribution-0..4、reader-*、usage-ok/warn/danger 等。
- 自定义 [data-theme^="custom:"] 自动派生：contrast-color() 算文字对比，color-mix(in oklch,...) 派生面板明度阶梯，边框跟随前景透明度。用户注入 <style id=custom-theme-style> 覆盖。

useTheme(hooks/useTheme.ts,121 行)：读 data-theme+localStorage theme；builtin light/dark，custom 存 goink_custom_themes。applyTheme 设 data-theme，custom 注入 <style>。MutationObserver 监听 data-theme 变化同步 state(约 L71-78)；prefers-color-scheme 变化在用户未显式设置时自动跟随(约 L80-87)。暴露 {theme,activeTheme,setTheme,toggle,addCustomTheme,deleteCustomTheme,customThemes}。themeMode 对 custom 取 type(light/dark)。

特效：.bg-layer 固定背景(渐变+斜线纹理)；particle-* 变量存在但注释显示“山峦已移除，仅保留渐变与纹理”(特效模块已移除，effects 字段为兼容旧数据预留)。

---
## 6. 跨模块事实性观察(非建议)
1. 字数统计重复实现：StatusBar.computeStats 与 ContentPanel.wordCountText 逻辑重复。
2. 模块级可变状态：useEditorTabs 的 let idSeq、ChatPanel 大量 useRef、useOutlineCache 的 cacheRef、NarrativeTimeline 的 maxZRef——非 React state，多实例并发下潜在串扰(符合 AGENTS.md 对共享状态警告)。
3. ChatPanel 双通道叠加：桌面 SSE(agent:turnID)与移动端(chat:api_event)是两套独立事件构建；若同一桌面对话两端都在发可能重复/错乱(当前互斥使用)。
4. rebuildTurns 对 run_subagent 的两次 continue：subagent assistant 消息先进 subagentCache，主 agent 的 run_subagent tool_display 处插入，依赖事件顺序。
5. theme change 时 CodeMirror：空 dispatch({}) re-layout 靠 CSS 变量换色。
6. Dirty 保护：ContentPanel 收到 file:changed 跳过 isDirty tab，避免覆盖输入；diff 审批后 handleDiffApprove 拉最新回填。
7. 搜索高亮 rune/col 偏移手写转换。
8. fetch 卡片字数去空白统计。
9. 搜索 result React key 用 r.type-r.id||i-r.chapter_num。
10. zh-CN.json(1167 行)顶层 24 个 key：common/app/init/workspace/shell/sidebar/chat/settings/novel/character/location/storyarc/timeline/reader/preference/search/export/git/skill/styleSample/extract/content/help。
11. 错误处理：多数视图层 .catch(()=>{}) 静默(如 WorkspaceView 更新检查 L196)；关键路径用 toastError(lib/utils)或组件 error state。
12. parseStringArray(utils) 把 LLM 自由格式 JSON 数组规整为字符串数组(对象取 name??description)，防 React key 崩溃。

本文档仅报告事实，未含修复建议。行号基于当前 frontend/src 各文件总行数。