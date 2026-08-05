# 全量代码审计与修复记录（2026-08-05）

> 对应 `archive/audit-repair-summary.md` 的后续轮次：P0/P1/P2/P3 全量修复。

## 审计范围

internal/agent（ReAct 引擎 + 五阶段门禁）、agentcfg、mcp_tools（57 工具）、rag、search、storage、migrate、设定数据 13 模块、llm、skill + skills、app/writing_context.go、frontend/src。300 万字网文规模视角。

## 修复清单（全部已落地）

### P0

| 项 | 修复 | 位置 |
|----|------|------|
| 门禁出厂默认不生效 | 新增 `EnsurePhaseGateConfigSeeded`：首次启动写入默认 single/batch 配置（与 门禁配置示例.md 同步），用户改过不覆盖 | app/default_phase_gate_config.go、app/handler.go |
| 死角色复活无约束 | characters 注入 status 字段；update_character 增加 dead 终态守卫（dead 不可复活）+ status_changed_chapter_id 必填；check_story_consistency 新增 dead_appeared 检查（characters_in 反查）；快照 ActiveChars 校验（ID 存在 + 非 dead） | writing_context_tools.go、app/writing_context.go、character_tools.go、appearance_tools.go、snapshot_tools.go、character/types.go |
| RAG 刷新非原子 | 先插后删（IndexChunks 成功才删旧）；失败退队重试（上限 2 次）；RebuildNovel 失败清空部分索引保证可重试；队列满 2s 等待不静默丢弃 | rag/refresh_queue.go |

### P1

| 项 | 修复 | 位置 |
|----|------|------|
| update_character 全量覆盖 | 改字段级 PATCH（name 空值不再清空）+ dead 守卫 + 状态变化章节记录 | character_tools.go |
| 审稿 16 项缺口 | 扩为 22 项（称呼/外貌/年龄修为/能力边界/关系一致性/性格连续性）；review 流程补 get_character_relations | sub-tech-review-standards.md、identity.go |
| 主 agent 无 OOC 条款 | 核心原则新增「角色行为红线（OOC 禁止）」 | identity.go |
| arc_id=NULL 全局设定失联 | get_writing_context 新增 global_lore 索引；volume_entities 设定查询加 `arc_id IS NULL` | writing_context_tools.go、app/writing_context.go |
| kernel.md 技能冲突/名字错误 | write 步骤 1 对齐 11 技能；`tech-sub-sub-tech-anti-ai-grade` 修正为 `sub-tech-anti-ai-grade` | skills/main-core-writing-kernel.md |
| 压缩触发低估 | 压缩判定计入 tools 定义 token（估算 20-30K） | agent.go |
| 检索 3 路合并缺失 | 新增 FTS5 表（trigram，失败回退 unicode61）+ 向量/FTS RRF 合并 + 按 (章,位置) 去重；写入缓存降级关键词匹配（原死缓存复活为降级源） | rag/vector_store.go、search/service.go |
| 无外键 | PRAGMA foreign_keys=ON；子表 novel_id 级联 FK + scene.chapter_id SET NULL | storage/sqlite.go、11 个 types.go |
| 断点续作 visited 丢失 | SaveState/LoadState 序列化 visited（兼容旧格式） | phase_gate.go |
| batch 循环无硬约束 | Loop 字段生效（batch 允许回退上一阶段），配置加 loop: true | phase_gate.go、default_phase_gate_config.go、门禁配置示例.md |

### P2/P3

update_item 自动写流转记录（owner 变更需 chapter_id）+ 终态不可逆守卫；editRelation 改 append-only；缓存统计修正（cached_tokens 存在即 miss）；429 重试上限 10 次；流式 60s idle 超时；max_tokens 兜底 8192；splitter 句级切分加 overlap；computeStreaks 限 730 天；CheckEditPath 用 path.Match 修复 Windows 分隔符问题；前端保存回流 dirty 保护；前端超期伏笔红色警示条 + 条目红边（新增 i18n 文案）。

## 未落地（依赖外部资源/高风险迁移）

- 向量索引升级 IVF/HNSW（sqlite-vec v0.1.6 不支持，需升级依赖）
- embedding 模型 int8 → fp16（需模型文件）
- 连接池 MaxOpenConns 提为 >1（RAG 与业务共享单连接的读写竞争需先拆分）
- 存量数据库外键迁移（SQLite 不支持 ALTER 加 FK，已有库的表无约束，需整表重建迁移；新库已自动带 FK）
- trigram tokenizer 的中文全文检索效果需真机建书实测（不可用时回退 unicode61 整词匹配）

## 验证（2026-08-05 补充，真机通过）

- 使用 build.ps1 的 CGO 环境（PATH 含 MSYS2 mingw64 + `CGO_CFLAGS=-I<GOMODCACHE>/mattn/go-sqlite3@v1.14.44`）：
  - `go build ./...` 全量编译通过（EXIT 0）
  - `go test ./internal/... ./app/...` 全部通过（含 agent/rag/search/llm/mcp_tools/app/config）
- 真机验证期间修复的存量测试损坏（fork 时就存在，非本次引入）：
  - search 测试引用已不存在的 `searchRAG`/`buildContext`/旧 NewService 签名 → 补 `buildContext`、修签名与调用
  - config 测试引用不存在的 `expandTilde` → 补实现
  - 搜索面板 timeline 分类显示英文（user_directive）→ 改为中文标签（伏笔/用户指令）
- 补充修复（真机验证暴露）：
  - FTS5 全文索引对存量章节缺失：RebuildAll 检测"向量有、FTS 空"触发重建（否则老书升级后关键词检索形同虚设）
  - 流式 idle 超时误杀慢速本地模型：首 token 宽限 300s，流中 120s，每行重置计时
- build.ps1：CGO_CFLAGS 由硬编码路径/版本改为从 go.mod 动态解析 mattn 版本 + `go env GOMODCACHE`，升级后不失效
- AGENTS.md：修正"CGO 编译报错是预期行为"的错误表述，写明完整编译环境
