# goink.md 定位收敛为章节指纹账本

> 日期：2026-08-04
> 状态：已实施（commit e8a16f3 / 2c116de）
> 背景：创作实践中发现 goink.md 职责混乱（快照 vs 累积行为漂移），经对账确认其内容全部可被 DB 承载，仅章节指纹无落点。

## 一、问题

实际创作中 goink.md 行为不一致：
- 旧书（小说 8，102 章）：累积式（当前进展覆盖 + 悬念/自主记录/指纹跨章累积）
- 新书（小说 11，7 章）：覆盖式（只有最新状态）

根因：identity.go 提示词写"跨对话状态快照"（覆盖），而 anti-repetition skill 要求"每章在 goink.md 追加指纹"（累积）——**指令冲突**，AI 行为随版本漂移。且 edit 工具无 append 模式，AI 只能 full_replace 重写（易覆盖历史）或 line_range_replace 数行号（易错）。

## 二、内容对账（goink.md 每块 vs DB 替代）

| goink.md 内容 | DB 替代表 | 消费者（门禁必查） |
|--------------|----------|-------------------|
| 当前进展 | `writing_snapshots` | get_writing_context ✅ |
| 角色动态 | `characters` + `character_relations` | get_characters ✅ |
| 开着的悬念 | `timeline_entries` | get_timeline ✅ |
| 推理链/世界观认知 | `lore_entries` | get_lore / search_lore ✅ |
| 创作偏好/经验 | `preference_items` | get_preferences ✅ |
| **章节指纹** | **无表** | **无消费者（唯一落点 = goink.md）** |

结论：goink.md 是历史遗留，职责被 DB 表逐个取代，仅指纹无替代载体。

## 三、决策

goink.md 只做一件事：**章节指纹账本**（追加式，每章一行，防重复用）。

- 状态/悬念/设定/偏好一律写 DB（对应工具见上表）
- 指纹写入必须用 edit 的 `change_type=append`（工具层新增），禁止 full_replace 重写
- NovelState 注入 goink.md **尾部**最近 1500 字符（固定窗口，字节稳定符合 P1 缓存协议）——指纹追加在末尾，尾部即最新指纹，供防重复比对
- 完整指纹历史由 AI 用 read(goink.md) 按需读取

## 四、改动清单

| 文件 | 改动 |
|------|------|
| `internal/mcp_tools/rw_tools.go` | edit 新增 `change_type=append`（追加到文件末尾，不覆盖已有内容）；schema 枚举 + editDescription 更新 |
| `internal/mcp_tools/search_replace_test.go` | 新增 TestAppend_ToExistingFile / TestAppend_ToEmptyFile |
| `internal/agentcfg/identity.go` | goink.md 维护规范改为「仅指纹，append 模式，其余写 DB」 |
| `internal/agentcfg/novel_state.go` | NS 注入改尾部 1500 字符（原头部截断会丢掉最新指纹，方向错误） |
| `skills/main-core-writing-kernel.md` | maintain 第 14 项改为「记录章节指纹（append）」 |
| `internal/skill/builtin/main-tech-anti-repetition.md` | 指纹系统明确用 append 模式 |

## 五、验证

- `go build ./...` + `go test ./internal/mcp_tools/ ./internal/agent/ ./app/` 全绿
- TestAppend 验证：追加保留旧内容、空文件直接返回内容
- 生产验证：写一章后检查 goink.md 末尾是否追加指纹行、NS 日志是否含「章节指纹（最近）」段

## 六、备注

- 不彻底删除 goink.md：指纹是其唯一合法用途，文件保留作为人类可读的指纹账本
- 推理链等认知结论明确用 create_lore 沉淀（lore 有 reveal_chapter_id 可表达"认知何时揭示"）
