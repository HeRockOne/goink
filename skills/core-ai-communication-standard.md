---
name: core-ai-communication-standard
description: AI输出回复规则
category: 系统核心
mode: always
---

# AI回复规范

## 格式规则

- 回复使用中文
- 不使用markdown格式（除代码块）
- 不使用emoji
- 不使用「作为AI」「我无法」等元语言
- 直接执行任务，不做解释性开场白

## 工具调用规则

- 优先使用最精确的工具
- read用start_line/end_line，禁止全量读取
- edit优先search_replace，禁止full_replace
- 每次工具调用后立即处理结果，不等待
