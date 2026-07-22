# 回滚设计文档

## 语义

**回滚 = 回到某个 turn 开始之前的状态。**

回滚 Turn N 意味着：Turn N 期间 AI 做的所有 DB 变更，以及 Turn N 之后用户对同一实体的所有后续编辑，全部按时间逆序撤销。

### 核心原则

- **AI 操作过的实体**：用户后续编辑继承 AI 的 session/turn，回滚时一起撤销。
  - 理由：用户修正的是 AI 的内容，既然不要 AI 的了，用户自己改的也没意义了。
- **纯用户创建的实体**（AI 从未操作过）：`turn_id=0`，永不被回滚。
  - 理由：这是"用户所有"的实体，不应被 AI 回滚影响。

### 链式继承

`resolveTurnForEntity` 查询 operation_log 中同一实体的最近一条记录，继承其 `session_id`/`turn_id`。
链式继承是自洽的：

```
AI 编辑 Bob (turn 5)           → {turn_id:5}
用户编辑 Bob                   → 查历史，最近=turn 5 → {turn_id:5}
用户再编辑 Bob                 → 查历史，最近=上一条用户编辑(turn 5) → {turn_id:5}
```

回滚 turn 5 时，三条记录都在范围内，按 id DESC 逆序回放，最终 Bob = AI 编辑前的值。

## 场景示例

### 场景 1：AI 创建 → 用户修正 → 回滚

1. AI 在 turn 5 创建 Alice → `{turn_id:5, op:create}`
2. 用户编辑 Alice 修正描述 → 继承 `{turn_id:5, op:update}`
3. 回滚 turn 5 → 逆序：先 reverse update，再 reverse create（删除 Alice）
4. 结果：Alice 被删除 ✓

### 场景 2：AI 编辑 → 用户修正 → 回滚

1. AI 在 turn 5 编辑 Bob → `{turn_id:5, op:update}`
2. 用户编辑 Bob 修正描述 → 继承 `{turn_id:5, op:update}`
3. 回滚 turn 5 → 逆序：先 reverse 用户编辑，再 reverse AI 编辑
4. 结果：Bob = AI 编辑前的值，用户修正也丢失 ✓
   - 理由：用户修正基于 AI 编辑后的内容，AI 编辑被撤销后修正无意义

### 场景 3：用户创建 → AI 编辑 → 回滚

1. 用户创建 Charlie → `{turn_id:0, op:create}`（纯用户实体）
2. AI 在 turn 5 编辑 Charlie → `{turn_id:5, op:update}`
3. 用户编辑 Charlie → 继承 `{turn_id:5, op:update}`
4. 回滚 turn 5 → 逆序：先 reverse 用户编辑，再 reverse AI 编辑
5. 结果：Charlie 保留（create 不在回滚范围），值 = 用户创建时的值 ✓

### 场景 4：纯用户操作

1. 用户创建 Dave → `{turn_id:0, op:create}`
2. 用户编辑 Dave → 继承 `{turn_id:0, op:update}`
3. 任何回滚都不影响 Dave ✓

## 实现机制

### 回调 fallback（`resolveTurnForEntity`）

当 GORM 回调（afterCreate/afterUpdate/afterDelete）检测到 context 中无 TurnInfo 时（前端 CRUD 场景）：

1. 查询 `operation_log` 中同一 `(table_name, entity_id)` 的最近一条记录
2. 继承其 `session_id`/`turn_id`
3. 无历史记录则 fallback 为 `turn_id=0, session_id=""`（不被回滚）

```sql
SELECT session_id, turn_id
FROM operation_log
WHERE table_name = ? AND entity_id = ?
ORDER BY id DESC
LIMIT 1
```

走 `idx_oplog_entity (table_name, entity_id)` 索引。

### PatchAndSave

替代 `Model().Where().Updates(&input)` 模式，解决 GORM 回调不触发的问题：

1. `First` 加载当前实体
2. `json.Marshal(input)` + `json.Unmarshal` 合并非零值字段（PATCH 语义）
3. `Save` 触发 GORM 回调（包括 operation_log 记录）

### 回滚查询

`RollbackInTx` 查询条件不变：

```sql
WHERE session_id = ? AND turn_id >= ? AND turn_id <= ?
ORDER BY id DESC
```

`turn_id=0` 的记录自然不在范围内，不被回滚。

## 已知限制

### 当前未覆盖

详细记录见 [rollback-known-issues.md](../../docs/todo/rollback-known-issues.md)

| 场景 | 原因 | 影响 | 计划 |
|------|------|------|------|
| ClearParent（`Model().Where().Update`） | Dest 无 PK，回调跳过 | parent_location_id 不记录 | P1：并入 PatchAndSave |
| 级联删除关系 | 批量 Delete，Dest 无 PK | 关系记录不追踪 | P2：per-row 循环 |
| PatchAndSave PK 防御 | 隐式假设 Input 不含 ID | 未来可能踩坑 | P2：反射重置 PK |
| 索引列顺序 | (turn_id, session_id) 应为 (session_id, turn_id) | 性能可忽略 | P3：调换顺序 |
| omitempty 清空字段 | PATCH 语义固有限制 | 无法清空 string/int 为零值 | P3：独立追踪 |

### 性能

- 每次前端 CRUD 多一次 SELECT（resolveTurnForEntity），走索引，毫秒级
- 每次写操作多一次 SELECT（fetchOldRow）+ 一次 INSERT（写日志）
- operation_log 持续增长，需定期清理已关闭 session 的记录

## 索引

| 索引名 | 列 | 用途 |
|--------|---|------|
| `idx_oplog_rollback` | `(session_id, turn_id)` | 加速 RollbackInTx 查询 |
| `idx_oplog_entity` | `(table_name, entity_id)` | 加速 resolveTurnForEntity 查询 |
