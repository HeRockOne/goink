# 全架构脆弱性审计 v2（2026-08-26，代码实读版）

> 应用户要求重做：v1 凭会话记忆写成，本版全部结论来自当日对核心链路代码的实际阅读。
> 实读文件：`internal/agent/agent.go`（1320 行，主循环/appendMsg/retry 路径）、`internal/agent/tokens.go`（259 行全量）、`internal/agent/compress.go`（448 行全量）、`internal/agent/cancel.go`（58 行全量）、`internal/agent/phase_gate.go`（CheckToolAllowed/SaveState/LoadState/checkResultGateMet）、`app/chat.go`（chatImpl 组装/cancel 注册/loadOrCreateSession）、`internal/mcp_tools/base.go`（Registry.Execute/OpenAI）。
> 视角不变：创作质量与耗费成本优先。标注 [实证]（有 Ch22-31 会话数据）/ [代码]（本日读码确认）/ [推演]。

## 〇、重大勘误：推翻 v1 的 C1"P1 缓存优化"

**v1 称"注入消息破坏前缀缓存，需重做 PendingInjections（P1，~70 行）"——误诊，撤销。**

证据复核（Ch27 逐轮 usage）：call 8438 cached=48,576 恰好等于上一轮 8434 的完整 prompt（48,593 取整）；miss=9,514 正是当轮新注入的两个技能全文的字节数。后续 8460/8473 同样模式。**前缀始终完整命中，miss 是每轮新增内容的正常一次性成本**——把注入挪到消息尾部不会省任何 token（新增字节总量不变）。已回滚的 commit 6bd0dc6 即使不被回滚也不会有收益。

连带修正：
- v1 的 P1 建议（重做 PendingInjections）作废；
- 8/9 批量命中率从 93% 掉到 86%（phase_gate.go:783 注释记载的旧问题）同理需要重新归因——当时归因给 StatusString 动态消息，方向可能也对也可能同样是统计口径问题，未复核，存疑待查。
- 教训：cached_tokens ≈ 上轮 prompt_tokens 是"前缀健康"的标志，miss > 新增字节量才是真失效。tokens.go:222 已有"偶发全 miss 告警"日志区分厂商驱逐，审计时应先查该日志再下结论。

## 一、代码级发现（本轮新发现）

### N1. 流中重试可重复执行非幂等工具 —— [代码]，中危
`agent.go` EventError 可重试分支（429/402/Retryable）：`continue streamLoop` 重启流，但**只清了 responseBuffer/thinkingBuffer/fullResponse，未清 toolOutputs**。而工具在 EventToolCallEnd 当场执行（registry.Execute 有真实副作用：edit 写文件、create_* 写库）。若流在中断前已执行过工具，重启后模型重新生成响应可能再次调用同类工具 → 文件被写两次/记录建两次，且新旧两批 toolOutputs 全部进历史。
概率评估：429 通常发生在请求起点，mid-stream 死亡较少见；但长流（写章节 400s+）遇上限流并不罕见（MiniMax 402"几分钟内恢复"注释即实证场景）。
建议：retry 分支补 `toolOutputs = nil` + 已执行工具的 tool_call_id 集合去重，或改为"流开始后不可重试，直接失败返回"。~10 行。

### N2. 工具执行无超时 —— [代码]，中危
`base.go Registry.Execute` 直接用 turn ctx 调 `t.Execute`，无 per-tool deadline。LLM 流有 120s idle 兜底（stream.go:294），工具没有任何兜底：web_fetch 抓慢站、git 锁竞争、SQLite busy 都可能挂住整个循环且无法恢复（ctx cancel 可解但用户未必想到点停止）。
建议：Execute 内 `context.WithTimeout(ctx, 120s)`（web 类工具已有自身 30s 不受影响）。~5 行。

### N3. 系统级错误对模型完全屏蔽 + "已禁用"是假禁用 —— [代码]，中危
两层叠加：
1. `Registry.Execute` 把 Go 层 error 统一替换为 `"服务器内部错误，请稍后重试"`（base.go:239）——模型看不到真实原因（DB locked / git 失败等），丧失自纠能力，只会盲目重试；
2. `agent.go:989` 连续 3 次 system 失败注入"已被禁用，请不要再调用"，但 **registry 里什么都没禁用**，工具照常可调——提示词撒谎，浪费轮次。
建议：ErrKind=system 时把 error 原文附进 ToolResult.Error（内部工具无安全泄露面）；failCnt 达阈值时真的在 AllowedTools 里临时摘除该工具，或改措辞为"请换用其他方式"。

### N4. 门禁错误记忆不跨 turn —— [代码]，低危
`SaveState`（phase_gate.go:1017）只持久化 successfulTools/visited/readsByPhase。`consistencyErrors`（本次新加的绕过封堵集合）和 `lastToolResults` 都是 Run 局部——用户中途打断再发"继续"，新 Run 里 ERROR 记忆清零，review→maintain 的硬错误拦截失效一轮。实际影响有限：一章流程通常单 turn 完成；跨 turn 多发生在用户主动中断场景。
建议：consistencyErrors 并入 SaveState JSON。~8 行。同族问题：`wordCountOK` 也不持久化（见 N5）。

### N5. SaveWordCount/LoadWordCount 是死代码 —— [代码]，低危
grep 全仓无调用方。wordCountOK 纯 Run 内存态，靠 handleBatchChapterBoundary 每章重置的设计掩盖了持久化缺失。要么接上（batch 跨 turn 场景），要么删掉防误导。

### N6. AgentLoopResult.FinalText 只含最后一轮文本 —— [代码]，低危
`agent.go:1127` 每轮末清空 fullResponse，RunSubAgent 返回的 FinalText = 子代理最后一个 turn 的输出。若审稿子代理输出报告正文后又调了一次工具（如 submit_review 在正文之后），FinalText 为空 → ParseReport 回退拿到空串、Record.Report 为空。submit_review 结构化路径不受影响（参数从转录扫描），但面板 report 列可能为空。
建议：RunSubAgent 场景拼接各轮文本，或规范"先 submit_review 后正文"顺序（standards.md 已如此要求，恰好规避）。

### N7. cancel 注册泄漏 —— [代码]，极低危
`chat.go:117` 仅 ctx 未取消时 Unregister；经 Cancel() 取消的注册项残留（CancelManager.Cancel 会删，但 chat.go defer 路径不走它）。IsRunning() 对已结束会话可能误报 true。一行修复（defer 无条件 Unregister）。

## 二、验证通过的环节（本轮读码确认 ✓）

- **pg 是 Run 局部变量**（agent.go:474 注释 + 代码证实），并发会话门禁不串扰；Ch28"pg 变 nil"之谜实为子代理本就无门禁，set_phase 走 L829 "门禁未启用" 分支——架构正确，非 bug。
- **压缩事务性**（compress.go persistCompression）：版本递增 + 全部写入单事务；NS 重落库、当前阶段必读技能重建补回（compress.go:110-121 专门处理了"压缩后同阶段不再触发注入"的坑）；子代理纯内存压缩不动系统头。
- **usage 双格式兼容**（tokens.go:120-146 OpenAI cached_tokens 优先 + DeepSeek 格式 fallback，键存在即计 miss 防"命中率虚高"）；UpdateMessageUsage 在 appendMsg 之后调用避免错位（agent.go:1084 注释）。
- **流半开检测**（stream.go 120s idle / 300s 首行宽限）✓。
- **工具 panic 兜底**（base.go:226 recover → ErrKind=system）✓。
- **拦截降噪**（blockCount ≥3 停止重复注入提醒）+ 成功后重置计数 ✓。
- **死循环检测**（isStuckLoop 8 轮窗口）✓。
- **set_phase 门禁空重建**（agent.go:741-753 从消息重解析）✓。
- **chatImpl panic recovery**（chat.go:59，修复过"对话静默停止"）✓。

## 三、会话实证项复核（维持 v1 结论的部分）

| 项 | v1 编号 | 复核结果 |
|----|---------|---------|
| 扩写挤牙膏循环 | C2 | 维持。模型规划力问题，write note + kernel 纪律已缓解，无架构级解 |
| 思考时长失控 | C3 | 维持。provider 思考预算参数未接通，可选 |
| pacing 债务无强制偿还 | Q1→**升为 P1** | 维持且升级：C1 撤销后这是最大质量缺口。type_drift/pacing_gap WARNING 与 review #23 打分都触发了，但"下一章必须补偿"没有机制。建议 write 进场时查最近 type_drift 结果，连续 ≥3 章 WARNING 则在方向锚提醒追加硬性条目（~10 行） |
| search_replace 丢内容无直接检测 | Q4→P2 | 维持。get_chapter_list 加环比骤降 WARNING（<上章×0.75 且达标 → 提示 read 复核，~8 行） |
| 弱模型身份崩塌 | Q2 | 免疫声明 + 清单去重已修，效果待下次弱模型测试验证 |
| submit_review 调用率 | Q3 | 本日落地，待真机验证（看下条 review_record dims≠-1） |
| 方向锚缺失静默跳过 | Q5 | 维持低危缓办 |
| 前端单流假设 | F1 | 维持（多端并发前需改造） |
| 上游行为可变 | F3 | 维持。可选：duration_ms>60s 且 thinking 空时 log WARN |
| ~~注入破坏缓存~~ | ~~C1~~ | **撤销**，见〇 |

## 四、修订后优先级

| 级别 | 项 | 动作 | 规模 |
|------|----|----|------|
| P1 | Q1 节奏债 | write 进场按 type_drift 结果注入补偿硬提醒 | ~10 行 |
| P2 | N3 错误屏蔽+假禁用 | system 错误透传原文 + 真禁用或改措辞 | ~15 行 |
| P3 | N1 重试重复执行 | retry 清 toolOutputs / 禁止流中重试 | ~10 行 |
| P4 | N2 工具超时 | Execute 包 120s WithTimeout | ~5 行 |
| P5 | N4 跨turn错误记忆 | consistencyErrors 入 SaveState | ~8 行 |
| 观察 | Q2/Q3/N6 | 下次真机测试验收 | 0 |

## 五、结论

主循环、压缩、usage、门禁状态机的工程质量经逐行检验高于预期（局部变量隔离、事务化压缩、双格式兼容、panic 兜底都做对了）。真正的脆弱点集中在三处：**异常路径的重试语义**（N1/N3）、**工具执行缺资源护栏**（N2）、**跨章节奏债务只有检测没有强制**（Q1）。另记一条方法论教训：缓存类结论必须先用"cached ≈ 上轮 prompt"公式核对再下判断——本次撤销的 C1 就是凭聚合命中率下降直接归因代码的反面教材。
