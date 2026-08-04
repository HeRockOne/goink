# 大纲按需加载 + 防越界实施方案

> 日期：2026-08-04
> 状态：待实施
> 关联：`docs/design/cache-hit-fix-implementation.md`（NS 落库）、`main-tech-book-outline.md`（三级大纲 skill）、`main-core-writing-kernel.md`（创作调度）

## 一、问题

现状三层大纲体系（main-tech-book-outline.md）只有 skill 定义，缺落点与强制：

| 层级 | 现状 | 缺口 |
|------|------|------|
| 总纲（全书 1000-3000 字） | skill 要求写，无落点文件 | ❌ AI 可能不写或乱写位置 |
| 卷纲（每卷 500-1000 字） | 在 `story_arcs.detail_json`（DB），get_writing_context 已返回 | ⚠️ 无强制先读卷纲再写章纲 |
| 章纲（每章 50-200 字） | `outlines/NNN.md` ✅ | 无 |
| 进度锚点 | get_writing_context 返回 volume.start/end_chapter | ❌ 无"禁止越界"约束注入 |

风险：AI 不读总纲/卷纲直接写章 → 方向漂移；AI 看到后面章节规划提前写 → 剧透/抢戏。

## 二、方案总览

**不改架构**，在现有结构上补 3 件事：

1. **总纲落点**：新增 `book-outline.md` 文件（仓库根目录，与 goink.md 同级），init 阶段强制写入，prepare 阶段随 NovelState 注入摘要。
2. **卷纲强制读取**：outline 阶段 kernel 硬约束 + 门禁 require 扩展（在 edit 章纲前必须已调用 get_writing_context——已满足，因 prepare 必查；补"卷纲内容进上下文"的机制）。
3. **进度锚点防越界**：get_writing_context 输出加"当前进度定位"字段 + NovelState 注入一句显式约束。

## 三、具体改动

### 改动 1：总纲落点 `book-outline.md`

**文件约定**：
- 路径：`{novel_dir}/book-outline.md`（小说仓库根，与 goink.md 同级）
- 内容：按 main-tech-book-outline.md「第一级：总纲」的 5 要素（核心矛盾/主角成长弧线/主题立意/结局方向/篇幅规划）

**代码改动**：
1. `internal/git/rw.go`：新增 `func BookOutlinePath() string { return "book-outline.md" }`
2. `internal/mcp_tools/rw_tools.go`：`pathRe` 正则加 `book-outline\.md`；`validPath` 错误消息同步；edit/read 的 Description 同步加路径说明
3. 门禁配置 `outline` 阶段 edit_paths 加 `book-outline.md`（总纲在 outline 阶段可修改）
4. `main-core-init-phase.md`：创建顺序表加第 0 步「写总纲 → edit(book-outline.md)」（在创建弧线之前），验证清单加「总纲已写入」

### 改动 2：卷纲强制读取

现状：`get_writing_context` 已返回当前卷的 name/description/detail_json/start_chapter/end_chapter + volume_entities（ID 列表）。AI 在 prepare 必查后上下文里已有卷纲。缺口是**没强调"必须按卷纲展开"**。

**改动**：
1. `main-core-writing-kernel.md` outline 阶段指令改为：
   ```
   ### outline
   1. 先确认 get_writing_context 返回的当前卷（volume）信息：本卷核心事件、主角状态变化、爽点位置、收尾钩子、需回收伏笔
   2. 加载技能（...同现状...）
   3. edit(outlines/NNN.md)（required）— 写本章大纲，必须：
      - 承接当前卷纲（不超出本卷 start_chapter ~ end_chapter 范围）
      - 标注本章类型（对照 book-outline 章节类型表）
      - 只规划本章事件，禁止展开后续章节情节
   4. set_phase("write")
   ```
2. `main-tech-book-outline.md` 大纲调整规则加一条：「章纲必须落在当前卷 start_chapter/end_chapter 范围内；跨卷内容属于下一卷纲，不得提前写入本章」。

### 改动 3：进度锚点防越界（含总纲在场机制）

**核心原则**：约束单向传递——总纲约束卷纲、卷纲约束章纲。进度锚点必须**同时携带总纲方向**，否则 AI 只看卷范围会偏离全书主线。总纲必须每次写作在场（不能只在 init 写一次）。

**改动 A — get_writing_context 加总纲摘要 + 进度锚点**（`internal/mcp_tools/writing_context_tools.go`）：

```go
// 总纲摘要（方向层）：从 book-outline.md 提取核心 4 要素
result["outline"] = map[string]any{
    "core_conflict":  "主角与最终对手的根本冲突",
    "growth_end":     "主角弧线终点（性格/能力/关系三线）",
    "ending_direction": "大结局方向",
    "total_volumes":  4,
}
// 进度锚点（位置层）：带终局方向，不只卷范围
result["progress"] = map[string]any{
    "current_chapter": chapNum,
    "volume_start":    vol.StartChapter,
    "volume_end":      vol.EndChapter,
    "toward_ending":   "当前处于全书中期，距终局（核心矛盾解决）约 N 卷",
    "rule": "只展开本卷情节；后续卷设定不得提前使用；所有章节事件必须服务于 outline 的核心矛盾与结局方向",
}
```

- 总纲摘要实现：init 阶段写入 `book-outline.md` 后，`get_writing_context` 读取该文件前 400 字做摘要注入（`git.ReadFile(novelID, git.BookOutlinePath())`），文件不存在则输出空提示。
- **缓存安全**：get_writing_context 输出落库为 tool 消息，总纲/卷纲不变则字节不变 → 缓存连续命中；总纲极少修改，偶尔断链成本仅几 KB。
- **约束单向**：rule 字段显式声明"服务总纲"，防止 AI 把卷纲当最终约束。

**改动 B — NovelState 注入进度锚点**（`internal/agentcfg/novel_state.go`）：
- NovelState 含书名/类型/简介 + 进度 + goink.md（已收敛为章节指纹账本，注入尾部最近 1500 字符，见 `goink-fingerprint-ledger.md`）。在进度行后加动态进度锚点（轮末动态字节，符合 P1 协议）：
  ```
  【当前进度】第 N 章（本卷 X~Y 章）。创作须服务于全书总纲（book-outline.md），只展开本卷情节。
  ```

**改动 C — 门禁 require 防"跳过总纲"**：
- init 阶段 require 加一项强制：无法直接校验"总纲已写"（require 是工具级）。折中：init 阶段 tools 列表已有 edit，把「总纲必须写入」作为 kernel 硬约束 + 验证清单（get 阶段查 book-outline.md 是否存在）。可在 init 门禁 require 里加一个专门校验工具，但成本高。
- **推荐轻量方案**：`main-core-init-phase.md` 验证清单加「read(book-outline.md) 非空」作为 set_phase("prepare") 的前置；kernel 硬约束写死。不扩展门禁配置。

## 四、兼容性

| 影响面 | 说明 |
|--------|------|
| 已有小说 | 无 book-outline.md → read 返回不存在，AI 可在 outline 阶段补写；不影响已有章节 |
| 缓存协议 | progress 字段在 get_writing_context 输出中（历史重放，字节稳定）；NovelState 锚点在轮末（P1 协议允许每轮变化）✓ |
| 门禁配置 | 不新增工具，不改 require 结构（init 验证清单走 skill 层） |
| 前端 | 无界面改动（book-outline.md 走既有 git 文件流，可在 git 面板看到） |

## 五、验证

1. `go build ./...` + `go test ./internal/...`（pathRe 改动涉及 rw_tools，跑 mcp_tools 测试）
2. 新建测试小说跑 init：确认 AI 写 book-outline.md（人工看 git 仓库）
3. 跑一轮 prepare→outline：确认上下文里出现 progress 锚点 + 卷纲（日志 grep "progress"）
4. 用 cacheprobe 回归：确认改动未破坏 NS 缓存链（`go run ./cmd/cacheprobe compare` miss 不劣化）

## 六、改动清单汇总

| 文件 | 改动 |
|------|------|
| `internal/git/rw.go` | +BookOutlinePath() |
| `internal/mcp_tools/rw_tools.go` | pathRe + book-outline.md；Description 文案 |
| `internal/mcp_tools/writing_context_tools.go` | result["progress"] 进度锚点 |
| `internal/agentcfg/novel_state.go` | NS 加「当前进度」行（轮末动态，不破坏缓存） |
| `internal/skill/builtin/main-core-init-phase.md` | 创建顺序加总纲；验证清单加 read(book-outline.md) |
| `internal/skill/builtin/main-tech-book-outline.md` | 调整规则加"章纲不越卷" |
| `skills/main-core-writing-kernel.md` | outline 阶段指令强化（先消费卷纲再写章纲）；init 硬约束 |
| 门禁配置（用户级） | outline edit_paths 加 book-outline.md（需用户在设置面板更新或放小说级配置） |
