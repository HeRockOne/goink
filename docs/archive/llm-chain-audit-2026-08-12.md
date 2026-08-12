# LLM 链路全量审计（2026-08-12）

范围：系统提示词 / 工具定义与 schema / skill 注入 / 缓存优化 / 阶段门禁 / 上下文压缩。
方法：代码逐段核对 + 与 ADR-0001、agent/DESIGN.md、phase-gate.md、token-injection.md、kernel skill 交叉比对。

## 一、本次已修复（高/中严重度）

| # | 问题 | 修复 |
|---|------|------|
| 1 | **set_phase 路径技能注入条件恒假**（agent.go）：`SetPhase` 成功时已把 currentPhase 置为 target，`from := CurrentPhase()` 在调用后读取 → `from != target` 永假，LLM 主动切换阶段时必读技能全文从不注入（门禁自动推进路径正常，但主动切换是 kernel 要求的主通道） | SetPhase 前记录 from（agent.go set_phase 分支） |
| 2 | **phaseGate 并发污染**（agent.go）：门禁实例存 Agent 共享字段，多会话并发（桌面+移动端）互相覆盖——拦截/记录/持久化串扰，子 agent 运行期间其他会话 getPG()=nil 门禁全失效 | Run 内局部 `pg` 变量（41 处调用点局部化），injectPhaseSkills/Compress/buildDisplay/flushInterruptedTools/RunSubAgent 全部显式传参 |
| 3 | **新会话起点硬编码 prepare，init 不可达**（chat.go:154）：出厂配置首阶段 init，SetPhase 只允许 next/visited，prepare 起点下 init 两者都不是 → 新书开书引导（类型/总纲/角色/世界观）整体跳过 | 删除强制 prepare，PhaseCurrent 留空让解析器落首阶段（init） |
| 4 | **batch 模式残留清除条件恒假**（agent.go defer）：`VisitedCount() > 1` 恒假（回 prepare 时 visited 已重置为 [prepare]）→ batch 白名单永久残留，后续单章消息卡死在 write（与已修复的 batch 退化 single 同根反向） | PhaseGate 加 `roundCompleted` 标记，SetPhase 重置 visited 时置位，defer 用 `phase=="prepare" && roundCompleted` |
| 5 | **GetMessagesForAPI Limit(1000) 截断最新消息**（session/store.go）：`ORDER BY id ASC LIMIT 1000` 取最早 1000 条，消息超限时本轮 user/NS 被截断，agent 首轮无用户输入 | 改 `ORDER BY id DESC LIMIT 1000` + 反转（保最新、历史顺序不变） |
| 6 | **兜底推进忽略 SetPhase 返回值**（agent.go:878）：失败也注入技能 + 发"已推进"假消息 + reads 污染 | ok 检查后才注入/发消息 |
| 7 | **update_character.Personality 双 required**（character_tools.go）：schema+validate 强制 required 与 PATCH 语义矛盾，maintain 只改状态必失败 | 去掉 required（保留 Execute"至少一个字段"守卫） |
| 8 | **主 agent 白名单缺 check_story_consistency**（identity.go）：kernel review 指令 + 门禁白名单都有，注册表拦截矛盾 | mainAgentTools 补入 |
| 9 | **压缩后当前阶段技能不恢复**（compress.go）：重建不含阶段技能，同阶段 set_phase 不注入 → 创作指导丢失 | persistCompression 重建尾部追加当前阶段 AutoSkillInjection 全文（Compress 传 pg，chat.go 手动压缩从 session 恢复 pg） |
| 10 | **schema required 与实现/描述矛盾**（5 处）：create_lore.arc_id/reveal_chapter_id、create_item.arc_id/narrative_role/owner_id、create_timeline_entry.importance、create_character.location_id 全部 jsonschema required 但 Execute 按 >0 跳过；set_phase.phase 反向缺 jsonschema required | 全部改为与实际语义一致（可选字段去 required，必填字段补 required） |
| 11 | **get_stats 死参数**（stats_tools.go）：include_characters/include_locations 承诺功能但 Execute 忽略 | 删参数 + 描述改指向 get_entity_appearances |
| 12 | **update_item 12 字段无 description + 无空更新守卫**（item_tools.go） | 补全 description + 加"至少一个字段"守卫 |
| 13 | **set_phase 失败也记成功调用**（agent.go） | ok 分支内才 OnToolCall |
| 14 | **CallOptions.AllowedTools 死字段**（llm/types.go + agent.go）：赋值后零读取，注释声称"模型可知白名单"未实现 | 删除字段与赋值 |
| 15 | **KeepNovelStateSnapshots=3 死常量**（agentcfg/novel_state.go）：与"NS 永不清理"协议矛盾 | 删除 |
| 16 | **set_phase 注入口径四方矛盾**（identity.go / set_phase description / kernel / 代码） | 统一为：真切换注入全文，同阶段（批量章边界）不重复注入，压缩重建补回全文 |

## 二、遗留（低严重度，未修）

| # | 位置 | 问题 |
|---|------|------|
| 1 | delete_tools.go:82-90 | delete_record 的 scene/lore/item 分支不走审批/关联检查，与描述"删除前自动检查关联数据"不符，与专用 delete_* 重复（三条路径并存） |
| 2 | appearance_tools.go:97-143 | characterAppearances limit 在 map 迭代后截断，"最近出场"不保证；itemAppearances DESC+反转与前者行为不一致；foreshadowAppearances 忽略 limit |
| 3 | 文档口径 | 内置 skill 实际 43（37 auto+5 manual+1 always），AGENTS.md/kernel 写 42，token-injection.md 写 38auto+4manual；工具 60 个，token-injection.md 写 59 |
| 4 | skills/main-core-writing-kernel.md:209 | "30 个内置 skill 全量调度"标题与实际表不符 |
| 5 | skills/ 双副本 | main-core-writing-kernel.md 在 skills/ 与 internal/skill/builtin/ 双份，无同步校验机制；main-core-ai-communication-standard 仅 skills/，用户未同步时 always 注入缺失无提示 |
| 6 | rw_tools.go:194 | if 块缩进错乱（无功能影响） |
| 7 | scene_tools.go:102 | update_scene.WordCount 无 description，Execute 按 >0 更新无法清零 |
| 8 | chat.go lastNS 查询 | 未限定 version/to_api（当前实现下无害，语义隐患） |
| 9 | agent/tokens.go:51-59 | 压缩后 fixed_prefix_tokens 仍显示压缩前旧值（展示口径） |

## 三、验证通过项（审计确认无问题）

- 工具 description 完整性：60 个工具无精简（创作方法论载体合规）
- 缓存前缀稳定性：每轮追加、NS 按需注入（字节相同跳过）、set_phase 静态确认消息、压缩后一次性重建成本后恢复稳定
- 压缩消息序列：[fp][reminder][summary][retained][phaseSkills][NS][marker] 前缀协议正确
- 门禁自动推进（循环内）正常注入技能（唯一坏路径是 set_phase 主动切换，已修）
- 子 agent 消息 to_api=false 隔离、run_subagent 报告经工具结果回传主 LLM

## 四、验证命令

`go build ./...` + `go test ./internal/agent/ ./internal/mcp_tools/ ./internal/session/ ./app/`（internal/web 测试失败为环境既有问题：内网 IPv6 防护拦截测试地址，与本次改动无关）
