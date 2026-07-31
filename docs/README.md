# Goink 项目状态总览

> 最后更新：2026-07-30
> 本文档是项目的中枢索引，汇总当前任务、完成情况、阻塞项和踩坑记录。

---

## 一、目录结构

```
docs/
├── README.md                       ← 本文档
│
├── 01-architecture.md              ← 系统架构（Wails + Go + React）
├── 02-phase-gate.md                ← 阶段门禁系统
├── 03-competitor-analysis.md       ← 竞品分析
│
├── 10-billing-panel.md             ← 计费面板技术设计
├── 11-billing-test-report.md       ← 计费面板测试报告
├── 12-prompt-caching-optimization.md ← 缓存优化方案
├── 13-token-optimization-plan.md   ← Token 优化计划
│
├── 20-narrative-panel.md           ← 动态叙事面板设计
│
├── 30-mcp-tools-audit.md           ← MCP 工具依赖链审计
├── 31-mcp-schema-audit.md          ← MCP Schema Required 全面审计
├── 32-audit-repair-summary.md      ← 审计修复总结
├── 33-data-integrity-audit.md      ← 数据完整性 + 看板审计
│
├── 40-token-handoff-ai.md          ← Token 优化 AI 交接文档
├── 41-token-project-record.md      ← Token 优化完整讨论记录
│
└── archive/
    ├── billing-bug-report.md        ← 计费 Bug 原始报告（存档）
    ├── billing-fix-audit.md         ← 计费修复审计（存档）
    └── feature-audit.md             ← 功能新增审计（存档）
```

---

## 二、当前任务

### 动态叙事面板布局优化
- **状态**：进行中
- **目标**：画布布局按信息层级排布（当前最大，弧线/伏笔次之，过去/未来/读者标准）
- **阻塞**：DetailTabs 去重（弧线/伏笔/读者重复 tab 待删除）
- **下一步**：修改 DetailTabs 代码，去掉 3 个重复 tab

---

## 三、已完成任务

### 3.1 Token 计费面板（✅ 2026-07-30）
- 缓存字段兼容 OpenAI 标准格式 + DeepSeek 格式
- `acc_completion_tokens` session 级累计输出 token
- 按模型独立累计（`per_model`），面板切换模型时显示对应消耗
- `model_usage` 持久化表
- 每消息存储 API 精确 usage 到 `ExtraMetadata.usage`
- 前端修复双倍累加 bug、fallback 逻辑

### 3.2 个人中心 Token 趋势图（✅ 2026-07-30）
- 日期选择器（开始/结束日期）
- 模型下拉筛选
- SVG 饼图

### 3.3 DOCX 导出（✅ 2026-07-30）
- 纯标准库 `archive/zip` + XML，无外部依赖

### 3.4 输入框引导提示（✅ 2026-07-30）
- 空会话时显示 4 张引导卡片

### 3.5 MCP Schema Required 审计（✅ 2026-07-28）
- 57 个工具全面审计
- 15+ 字段修正
- 所有 P0/P1/P2 问题已修复

### 3.6 数据管线整合审计（✅ 2026-07-27）
- 三层分析：Schema Required → WritingContext → Kanban UI
- 6 个 schema 缺陷 + 4 个数据层 gap + 3 个 UI bug 全部修复

---

## 四、踩坑记录

### 4.1 per_model 双倍累加
- **现象**：每次 EventUsage 调用 `updateUsage`，`m["hit"] += hitTokens` 执行两次
- **原因**：重构时新旧代码重复，没删除旧的累加行
- **修复**：删除重复的 `m["hit"] += hitTokens` 行
- **教训**：改代码后必须检查是否有残留的旧逻辑

### 4.2 UpsertModelUsage 传累计值而非增量
- **现象**：DB 表 `model_usage` 数据重复翻倍
- **原因**：传入 `m["hit"]`（已累计的总值）给 UPSERT 函数，函数再 ADD 到已有值
- **修复**：改为传入 `hitTokens`（当前请求增量）
- **教训**：UPSERT 函数的参数契约要明确是增量还是累计

### 4.3 前端 fallback 到全局合计
- **现象**：切换到未使用过的模型时，面板显示全部模型的累计输出
- **原因**：`per_model[selectedModel]` 不存在时回退到 `acc_completion_tokens`
- **修复**：不存在时显示 0
- **教训**：模型级显示不应该回退全局值

### 4.4 面板柱状图改崩
- **现象**：重写 Panel 趋势图柱状图后整个面板不显示
- **原因**：JSX IIFE 中语法错误导致渲染崩溃
- **修复**：恢复原始方案，改用饼图
- **教训**：前端改动应该在浏览器控制台先测，不能只靠 build

### 4.5 叙事面板画布改 Grid
- **现象**：把 canvas（绝对定位）改成 CSS Grid
- **原因**：主观判断"画布不适合叙事面板"，没理解用户选择画布的设计意图
- **修复**：恢复画布代码
- **教训**：用户的交互设计选择有其理由，不要替用户做此类决策

---

## 五、未完成任务

| 任务 | 优先级 | 阻塞原因 |
|------|--------|---------|
| DetailTabs 去重 | 低 | 等用户确认后改代码 |
| 首次安装新手引导 | 中 | 已有 InitView + HelpDialog 自动弹出，完善引导文案即可 |
| 出版格式导出增强 | 低 | DOCX 已有，排版优化待定 |

---

## 六、技术债

| 项目 | 影响 | 说明 |
|------|------|------|
| ONNX Runtime 缺失 | 低 | 向量搜索不可用，不影响核心写作流程 |
| `goink.log` 日志累积 | 低 | 日志文件会持续增长，需加日志轮转 |
