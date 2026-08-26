# 全架构脆弱性审计 v2（2026-08-26，代码实读版）

## 0. 审计说明

| 项 | 内容 |
|----|------|
| 方法 | 逐行实读核心链路代码（v1 凭会话记忆写成，已作废重做） |
| 实读范围 | `internal/agent/agent.go`（1320行）、`tokens.go`、`compress.go`（448行）、`cancel.go`、`phase_gate.go`（状态持久化/CheckToolAllowed/checkResultGateMet）、`app/chat.go`（chatImpl）、`internal/mcp_tools/base.go`（Registry） |
| 视角 | 创作质量与耗费成本优先 |
| 标注 | [实证]=有 Ch22-31 会话数据支撑；[代码]=本日读码确认；[推演]=结构分析 |

---

## 1. 重大勘误

### 1.1 撤销 v1-C1："注入消息破坏前缀缓存"

**结论：误诊，撤销。**

| 证据 | 数据（Ch27 逐轮 usage） |
|------|------------------------|
| call 8438 | cached=48,576 ≈ 上轮完整 prompt（48,593） |
| 当轮 miss=9,514 | 恰好 = 新注入的两个技能全文字节数 |
| call 8460 / 8473 | 同样模式：cached ≈ 上轮 prompt 总量 |

**前缀始终完整命中。** miss 是每轮新增内容的正常一次性成本，把注入挪到消息尾部不省任何 token。

连带修正：

- ~~P1 重做 PendingInjections~~ → 作废
- 已回滚的 commit 6bd0dc6 即使保留也无收益
- 批量模式 93%→86%（phase_gate.go:783 注释）归因给 StatusString 动态消息——未复核，存疑待查
- **方法论教训**：`cached ≈ 上轮 prompt_tokens` = 前缀健康；miss > 新增字节量才是真失效。下结论前先查 tokens.go:222 的全 miss 告警日志

---

## 2. 新发现（代码级）

### N1. 流中重试可重复执行非幂等工具

| 项 | 内容 |
|----|------|
| 级别 | 中危 |
| 位置 | `agent.go` EventError 可重试分支 |
| 现象 | 重试只清 response/thinking buffer，**不清 toolOutputs**；工具在 EventToolCallEnd 当场执行（有真实副作用）；重启流后模型可能再次调用同类工具 → 文件写两次/记录建两次，新旧 toolOutputs 全进历史 |
| 触发条件 | 长流（写章节 400s+）中途遇 429/402（MiniMax 402 数分钟恢复是实证场景） |
| 建议 | retry 分支清 toolOutputs + 按 tool_call_id 去重，或流开始后禁止重试直接失败。~10 行 |

### N2. 工具执行无超时

| 项 | 内容 |
|----|------|
| 级别 | 中危 |
| 位置 | `base.go` Registry.Execute |
| 现象 | LLM 流有 120s idle 兜底（stream.go:294），工具没有任何兜底——web_fetch 慢站/git 锁/SQLite busy 可挂住整个循环 |
| 建议 | Execute 包 `context.WithTimeout(ctx, 120s)`。~5 行 |

### N3. 系统错误屏蔽 + 假禁用

| 项 | 内容 |
|----|------|
| 级别 | 中危 |
| 位置 | `base.go:239` + `agent.go:989` |
| 现象 | ① Go 层 error 统一替换为"服务器内部错误"，模型看不到真实原因，丧失自纠只会盲目重试；② 连续 3 次失败提示"已被禁用"，但 registry 未做任何禁用，工具照常可调 |
| 建议 | ErrKind=system 时透传 error 原文到 ToolResult.Error；达阈值真摘除工具或改措辞。~15 行 |

### N4. 门禁错误记忆不跨 turn

| 项 | 内容 |
|----|------|
| 级别 | 低危 |
| 位置 | `phase_gate.go` SaveState(1017) |
| 现象 | 只持久化 successfulTools/visited/reads；consistencyErrors 和 lastToolResults 是 Run 局部——用户打断再续，ERROR 记忆清零，review→maintain 硬错误拦截失效一轮 |
| 缓解 | 一章流程通常单 turn 完成，跨 turn 多为用户主动中断 |
| 建议 | consistencyErrors 并入 SaveState JSON。~8 行 |

### N5. 死代码：SaveWordCount/LoadWordCount

| 项 | 内容 |
|----|------|
| 级别 | 低危 |
| 位置 | `phase_gate.go:1039/1093` |
| 现象 | 全仓无调用方；wordCountOK 纯 Run 内存态，被每章重置设计掩盖 |
| 建议 | 接上（batch 跨 turn）或删除 |

### N6. FinalText 只含最后一轮

| 项 | 内容 |
|----|------|
| 级别 | 低危 |
| 位置 | `agent.go:1127` |
| 现象 | 每轮末清空 fullResponse；子代理输出报告后又调工具则 FinalText 为空 → Report 字段空。submit_review 路径免疫（参数从转录扫描），standards 要求"先 submit_review 后正文"恰好规避 |
| 建议 | RunSubAgent 场景拼接各轮文本，或维持现有顺序约束 |

### N7. cancel 注册泄漏

| 项 | 内容 |
|----|------|
| 级别 | 极低危 |
| 位置 | `chat.go:117` |
| 现象 | 仅 ctx 未取消时 Unregister；Cancel() 取消的注册项残留，IsRunning 可能误报 true |
| 建议 | defer 无条件 Unregister |

---

## 3. 会话实证项（v1 复核结果）

| 编号 | 项 | 结论 | 备注 |
|------|----|------|------|
| Q1 | pacing 债务无强制偿还 | **维持，升 P1** | Ch23-31 连续低密度，WARNING/review 打分全触发但下章补偿无机制 |
| Q2 | 弱模型身份崩塌 | 缓解待验证 | 免疫声明+清单去重已修，下次弱模型测试见分晓 |
| Q3 | submit_review 调用率 | 待真机验证 | 看下条 review_record dims≠-1 |
| Q4 | search_replace 丢内容无检测 | 维持 P2 | get_chapter_list 加环比骤降 WARNING（<上章×0.75 → 提示复核） |
| C2 | 扩写挤牙膏循环 | 维持 | 模型规划力问题，已有缓解，无架构级解 |
| C3 | 思考时长失控 | 维持 | provider 思考预算未接通，可选 |
| Q5 | 方向锚缺失静默跳过 | 低危缓办 | 十章实测未出现中途缺失 |
| F1 | 前端单流假设 | 多端并发前改造 | |
| F3 | 上游行为可变 | 不可控 | 可选：duration>60s 且 thinking 空 log WARN |
| ~~C1~~ | ~~缓存破坏~~ | **撤销** | 见 §1.1 |

---

## 4. 验证通过的环节 ✓

| 环节 | 证据 |
|------|------|
| 门禁并发隔离 | pg 是 Run 局部变量（agent.go:474）；Ch28 "pg 变 nil" 实为子代理无门禁的正确分支 |
| 压缩事务性 | 版本递增+全写入单事务；NS 重落库；阶段技能重建补回（compress.go:110 专门处理压缩后不触发注入的坑） |
| usage 双格式兼容 | OpenAI cached_tokens 优先 + DeepSeek fallback；键存在即计 miss 防虚高（tokens.go:120-146） |
| 消息 usage 不错位 | UpdateMessageUsage 在 appendMsg 之后调用 |
| 流半开检测 | 120s idle / 300s 首行宽限（stream.go:294） |
| 工具 panic 兜底 | recover → ErrKind=system |
| 拦截降噪 | blockCount ≥3 停止重复注入，成功后重置 |
| 死循环检测 | isStuckLoop 8 轮窗口 |
| chatImpl panic recovery | 修复过"对话静默停止" |

---

## 5. 修订后优先级

| 级别 | 编号 | 动作 | 规模 |
|------|------|------|------|
| **P1** | Q1 | write 进场按 type_drift 结果注入节奏补偿硬提醒 | ~10 行 |
| P2 | N3 | system 错误透传原文 + 真禁用或改措辞 | ~15 行 |
| P3 | N1 | retry 清 toolOutputs / 禁止流中重试 | ~10 行 |
| P4 | N2 | Execute 包 120s WithTimeout | ~5 行 |
| P5 | N4 | consistencyErrors 入 SaveState | ~8 行 |
| 观察 | Q2/Q3/N6 | 下次真机测试验收 | 0 |

---

## 6. 总结

主循环、压缩、usage、门禁状态机经逐行检验质量高于预期。真正的脆弱点集中在三处：

1. **异常路径的重试语义**（N1/N3）
2. **工具执行缺资源护栏**（N2）
3. **跨章节奏债务只有检测没有强制**（Q1）

方法论教训：缓存类判断必须先用 `cached ≈ 上轮 prompt` 公式核对——C1 误诊就是凭聚合命中率下降直接归因代码的反面教材。
