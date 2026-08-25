# 全架构脆弱性审计（2026-08-26）

> 视角：Goink 是 AI 创作小说的软件，创作质量与耗费成本是核心关注。
> 审计范围：LLM 上游 / 审稿链路 / 上下文缓存 / 工具 schema / 门禁 / 数据 / 前端会话。
> 依据：Ch22-31 共 10 次创作测试会话（dots3/mimo-v2.5/MiniMax-M2.7/MiniMax-M3/hy3）DB 实证 + 本日代码审查。每条标注 [实证]（有会话数据支撑）或 [推演]（结构分析）。

## 一、成本脆弱性（按浪费量排序）

### C1. 注入消息破坏前缀缓存 —— 最大已确认未回收项 [实证]
- 现象：注入消息（阶段清单/方向锚/auto_skill/差量提示）经 appendMsg 追加到 `opts.Messages` 中段，下一轮前缀失配 → cache miss。Ch27 实测 miss 97,695 tokens/章，命中率 94.27%（历史基线 97.5%+）。
- 根因已定位（agent.go appendMsg 路径），PendingInjections 方案已实现过一次（commit 6bd0dc6），因同期出现 thinking 显示回归被误判关联而回滚（git reset 1e033ff）。后经排查确认 thinking 回归是 mimo 上游行为变更，与本方案无关。
- 影响：~30K tokens/章 ≈ 单章成本 15%；8 章 ≈ 24 万 tokens。
- 建议：**重新实施 PendingInjections**（设计已验证：injectMsg 持库+挂 PendingInjections 尾部、LLM 调用点拼接、子代理合并 parentOpts.PendingInjections 保前缀一致）。实施前先在新会话确认 mimo thinking 显示现状，避免再次误判归因。

### C2. 扩写挤牙膏循环 [实证]
- 全部模型均出现（dots3/mimo/MiniMax）：初稿 1700-2100 字 → 5-10 轮 search_replace 补字 → 达标。Ch30 最惨：1218→2409 共 9 轮；Ch26 出现 search_replace 反而丢内容至 1639。
- 已做缓解：write 阶段 note"按场景分配字数、一次写完"、kernel"缺口×1.2 一次扩到位"。mimo Ch27 后初稿达标率提升但未根除——这是模型规划能力问题，架构只能降低频率。
- 成本本质：每轮补写 = 重发全量 prompt（cached 便宜但 completion+延迟照付）。9 轮 ≈ 多花 5-8 分钟墙钟时间。
- 残余风险：无硬约束阻止分多次 edit 写正文。

### C3. 思考时长失控 [实证]
- mimo Ch26 单次思考最长 483s；MiniMax-M2.7 写 Ch25 全程 ~15min 思考。completion token 计费但主要损失是时间。
- kernel 已有执行纪律（thinking 只做规划/一轮决策），效果未量化。无架构级手段（思考预算是 provider 参数，未接通）。

### C4. 子代理 fork 缓存设计良好 ✓
- 子代理首轮命中主会话缓存，miss 仅身份+技能+NS+指令（~3-5K）。审稿单轮成本 ~70K prompt 其中 95%+ 命中。无需改动。

## 二、创作质量脆弱性

### Q1. pacing 检测到了但没有纠正执行闭环 —— 当前最大质量缺口 [实证]
- Ch23-31 连续低密度章：review #7 打出 pacing=5.0（历史最低）、#11 报告"连续5章零动作"、type_drift/pacing_gap 持续 WARNING——检测层全部正确触发。
- 但 WARNING 不拦转场（设计如此，ERROR 才拦），review 建议写进报告后主代理下一章是否执行全凭自觉。Ch28 换 MiniMax-M3 直接崩到 6.4/fail。
- 本质：**跨章节奏债务没有强制偿还机制**。方向锚有"未兑现爽点"（beat 层面），但没有"连续 N 章无高密度场景 → 下章必须补偿"的注入。
- 建议（P2，~10 行）：autoAdvancePhase 进入 write 时查最近 type_drift 结果，若连续 ≥3 章 WARNING 则在方向锚提醒中追加硬性条目"本章必须安排 ≥300 字高密度对抗场景，否则审稿将打回"。

### Q2. 弱模型身份崩塌：免疫声明效果未验证 [实证根因/缓解待验证]
- hy3/MiniMax-M3 fork 主历史后被阶段清单诱导模仿父代理（Ch31 七回合堕落螺旋，review_records #12 解析全失败）。
- 已修：instruction 尾部免疫声明（近端最高对抗力）+ 清单去重（C 修复消除了转场 ×2 的重复诱导源）。
- 待验证：下次弱模型测试观察子代理第一动作。若仍崩，B 方案（过滤 fork 历史 <system-reminder>，代价 ~80-90K miss/次）或改隔离式子代理（Claude Code 默认路线，放弃缓存命中换确定性）二选一。

### Q3. submit_review 新链路未经真机验证 [推演]
- 结构化提交 + 回退链（工具→正则）逻辑已测，但真实模型是否记得调用未知。观察指标：下一条 review_record 的 dims 是否非 -1、verdict 是否非 unknown。
- 若调用率低：把 submit_review 加进 review 阶段 kernel 必做清单（提示词强化），不建议做成门禁 require（管不到子代理内部）。

### Q4. search_replace 内容丢失无直接检测 [实证事故/兜底间接]
- Ch26：search_replace 扩写反而丢内容 2055→1639 字。dup_paragraph 只能查逐字重复段，查不出"丢了内容"；get_chapter_list 字数校验只能发现跌破 2400 下限的情况——从 2800 丢到 2500 不会触发任何告警。
- 建议（P3，~8 行）：get_chapter_list 返回环比信息，本章字数 < 上一章×0.75 且仍达标时附 WARNING"字数骤降，疑似编辑丢失内容，请 read 复核"。

### Q5. 方向锚数据缺失静默跳过 [推演，低危]
- buildDirectionAnchor 各项数据缺失时静默返回空（novel_state.go:112）。开书早期（无卷纲/无 beats）锚为空是正常的，但中途数据损坏也会静默变空——模型失去护栏而无感知。
- 建议：锚为空且 chapter > 3 时在 NS 附一行"【方向锚】数据缺失，请检查 volumes/outlines/preferences 表"。暂缓——novel-2 十章实测未出现中途缺失。

### Q6. 已闭环项确认 ✓（本轮审计验证，无需动作）
- 台账防腐：timeline resolved 未来值拒绝 + ledger_integrity JOIN 校验（chapter_id/PK 混淆不再误报）
- 整段重复：dup_paragraph 第12检查（Ch28 line_range_replace 内容复制类 fatal 可拦截）
- check_types 缩窄绕过：consistencyErrors 指纹集合封堵
- tags 数组/old_content 别名/batch 统一：参数形态三类已修
- 会话切换续流：streamInfoRef 机制（单流假设，见 F1）
- 审稿评分：submit_review 代码算分（本日落地）

## 三、其他层脆弱点

### F1. 前端流式订阅单流假设 [推演，中危]
- ChatPanel streamInfoRef 是单值：桌面端会话 A 流式中，移动端发起会话 B，双端并发流的事件路由可能串扰。cacheprobe 曾因包级状态串扰翻车（已改 Run 局部变量），前端同款隐患。
- 用户当前单窗口使用习惯下低危。做多端并发前需改造为 per-session 订阅表。

### F2. verdict unknown 不拦门禁 [设计取舍]
- ParseReport 回退失败 → unknown 放行。理由：不能因解析问题卡死创作流程。接受，但建议加观测：review_records 连续 2 条 unknown 时 goink.log WARN（一行日志，便于发现问题率上升）。

### F3. 上游 API 行为可变 [实证，不可控]
- mimo reasoning_content 消失事件证明：上游可以在不通知的情况下改变响应形态。客户端只解析 delta["reasoning_content"]，思考被吞时无告警。
- 可选缓解：duration_ms > 60s 且 thinking_content 为空时 log WARN（检测异常模式）。优先级低——计费和功能不受影响，仅 UI 展示缺失。

### F4. 审稿 instruction 无模板强制 [低危]
- kernel 已给标准化模板，主代理即兴发挥空间仍在。records #4-#11 显示质量尚可。暂不动。

## 四、优先级汇总

| 级别 | 项 | 动作 | 规模 |
|------|----|----|------|
| P1 | C1 缓存 | 重做 PendingInjections（先定性 thinking 现状） | ~70 行 |
| P2 | Q1 节奏债 | write 进场注入高密度场景硬提醒 | ~10 行 |
| P3 | Q4 丢内容 | get_chapter_list 环比骤降 WARNING | ~8 行 |
| P4 | F2/Q3 观测 | unknown 连续计数 WARN 日志 | ~5 行 |
| 观察 | Q2 崩塌/Q3 调用率 | 下次弱模型真机测试验证 | 0 |

## 五、结论

架构六层闭环（检测/拦截/门禁/审稿/持久化/缓存协议）经 10 章实测成立，错误数 27→0→3 的趋势证明修复有效。剩余脆弱性集中在两处结构性欠账：**注入破坏缓存**（钱）与**节奏债务无强制偿还**（质量）。两者都有低成本方案，见上表 P1/P2。
