# 真机验证测试手册（2026-08-12 LLM 链路审计修复后）

对应修复：`docs/archive/llm-chain-audit-2026-08-12.md`（16 项修复）。本手册验证关键修复在真机的实际效果。

## 前置条件

- 部署 exe 含 2026-08-12 16:55 之后的构建（`goink-20260812-16-55.exe` 起）
- 门禁配置为出厂默认（设置页门禁 tab 未改动，或点"恢复默认"）
- 数据准备：新建一本测试小说（测试完可删）
- 日志：`D:\Goink\goink.log`；DB：`D:\Goink\novel-agent.db`（sqlite3 可读，不影响运行中的程序）

## 通用查看命令

```powershell
$db = "D:\Goink\novel-agent.db"

# 阶段推进与模式（新会话排最前）
& sqlite3 $db "SELECT session_id, current_phase, phase_mode, active_version, datetime(updated_at,'localtime') FROM sessions ORDER BY updated_at DESC LIMIT 5;"

# 技能注入（system 消息含技能头）
& sqlite3 $db "SELECT turn_id, substr(content,1,80) FROM messages WHERE session_id='<会话ID>' AND role='system' AND content LIKE '%--- main-%';"

# set_phase 确认 / 拦截
& sqlite3 $db "SELECT turn_id, substr(content,1,120) FROM messages WHERE session_id='<会话ID>' AND (content LIKE '%已切换到%' OR content LIKE '%门禁拦截%');"

# 命中率
& sqlite3 $db "SELECT model_id, hit_tokens, miss_tokens, ROUND(hit_tokens/(hit_tokens+miss_tokens)*100,1)||'%' FROM model_usage ORDER BY id DESC LIMIT 5;"
```

---

## 场景 1：新书开书（验证 init 阶段可达，修复 3）

**操作**：新建小说 → 新会话发「开始写一本仙侠小说《登天之路》」

**预期**：
- 日志 `阶段门禁已启用 phase=init`（旧 bug 是 prepare）
- AI 主动建角色/世界观/弧线/总纲（init 白名单 create_*）
- require 7 项查询满足后自动推进 prepare

**验证**：
```powershell
& sqlite3 $db "SELECT session_id, current_phase FROM sessions ORDER BY created_at DESC LIMIT 1;"
& sqlite3 $db "SELECT count(*) FROM messages WHERE session_id='<会话ID>' AND role='system' AND content LIKE '%--- main-core-init-phase%';"
```

**失败标志**：日志 phase=prepare 开头 / AI 跳过大纲与建角色直接写作。

---

## 场景 2：单章完整流程（验证 set_phase 技能注入，修复 1）

**操作**：开书后发「开始写第一章」（阶段链 prepare→outline→write→review→maintain）

**预期**：
- 每次真切换阶段，日志出现技能注入（全文或短提醒）
- review 阶段主 agent 可直接调 check_story_consistency（修复 8）
- write 转出 review 前 get_chapter_list 字数校验拦截（不达标会拦）

**验证**：
```powershell
& sqlite3 $db "SELECT turn_id, substr(content,1,100) FROM messages WHERE session_id='<会话ID>' AND content LIKE '%已切换到%';"
& sqlite3 $db "SELECT turn_id, content FROM messages WHERE session_id='<会话ID>' AND content LIKE '%门禁拦截%';"
```

**失败标志**：阶段切换后无技能注入日志 / check_story_consistency 报"工具禁止使用"。

---

## 场景 3：批量创作一轮 + 单章回归（验证 batch 残留，修复 4）

**操作**：发「批量写六章」→ 等一轮完整走完（回到 prepare）→ 发「精修第一章」

**预期**：
- 批量期间 `phase_mode=batch`，write 阶段每章 set_phase("write") 边界声明 + 字数校验
- 一轮回 prepare 后 `phase_mode` 被清除（空）
- 单章消息正常走单章流程，不卡 write

**验证**：
```powershell
& sqlite3 $db "SELECT session_id, current_phase, phase_mode FROM sessions ORDER BY updated_at DESC LIMIT 3;"
& sqlite3 $db "SELECT turn_id, substr(content,1,100) FROM messages WHERE session_id='<会话ID>' AND content LIKE '%门禁拦截%';"
```

**失败标志**：单章消息后 phase_mode 仍是 batch / current_phase 卡在 write / 出现 write 白名单拦截。

---

## 场景 4：压缩触发（验证压缩后技能恢复，修复 9）

**操作**：用 128K 窗口模型（如 mimo）写多章触发压缩，或长对话至 0.95×窗口

**预期**：
- 日志 `开始上下文压缩`
- `active_version` 递增（1→2）
- 压缩后新版本消息序列含当前阶段技能全文（persistCompression 补回）

**验证**：
```powershell
& sqlite3 $db "SELECT session_id, active_version FROM sessions ORDER BY updated_at DESC LIMIT 3;"
& sqlite3 $db "SELECT role, substr(content,1,60) FROM messages WHERE session_id='<会话ID>' AND version=2 ORDER BY id;"
# 序列应为: system(identity) system(always) system(catalog) user(reminder) user(summary) system(phaseSkills) system(NS)
```

**失败标志**：压缩后技能消息缺失（序列无 phaseSkills）。

---

## 场景 5：缓存命中率与并发（修复 5 + 2）

**操作**：正常写 3-5 轮后查命中率；有条件时桌面端 + 移动端（HTTP API）同时各发一条消息

**预期**：
- 命中率 89-93%（校准后口径）
- 并发时两个会话门禁状态互不污染（无拦截串扰/阶段错乱）

**验证**：
```powershell
& sqlite3 $db "SELECT model_id, hit_tokens, miss_tokens, ROUND(hit_tokens/(hit_tokens+miss_tokens)*100,1)||'%' FROM model_usage ORDER BY id DESC LIMIT 5;"
```

**失败标志**：命中率 < 86%（前缀断裂点）/ 并发后某会话阶段状态错乱。

---

## 结果记录

| 日期 | 场景 | 结果（通过/失败） | 关键证据（会话ID/日志行） | 备注 |
|------|------|------------------|--------------------------|------|
| | 1 新书 init | | | |
| | 2 单章流程 | | | |
| | 3 批量+单章回归 | | | |
| | 4 压缩恢复 | | | |
| | 5 命中率/并发 | | | |
