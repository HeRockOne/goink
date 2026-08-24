# Kernel 重构映射（265行→目标~120行）

## 删除（30行，与identity.go完全重复）
- L8-14: skill目录+优先级链 → identity.go L185-192 已覆盖
- L60-69: 卷结构+卷摘要 → identity.go L144-145 已覆盖
- L71-73: 并行工具调用 → identity.go L173 已覆盖

## 压缩（~105行→~30行，gate config覆盖骨架，kernel保留细节）
- L75-81 init: 删"门禁自动注入必读技能"（gate config已做），保留流程步骤
- L83-97 prepare: 删"必读技能已就绪"（injectPhaseSkills已做），保留参数建议
- L99-123 outline: 删"必读技能已就绪"，删format骨架（skill已覆盖），保留7 section列表
- L125-151 write: 删"必读技能已就绪"，压缩技能列表为引用，保留字数规则+write→review边界
- L153-160 review: 压缩为引用reviewAgentSystem1+模板
- L162-198 maintain: 压缩为引用gate config require+精简条件判定

## 保留（~85行，独一份内容）
- L19-58 批量模式规则: 40行→25行（压缩重复表述）
- L139 字数规则: 保留
- L148 写后自审: 保留
- L151 write→review边界: 保留
- L200-230 实体判定标准: 30行→20行（压缩）
- L232-246 技能表: 保留可选技能列表，删除required（gate config已有）
- L248-259 硬约束: 保留

## 新增
- review instruction模板（~5行）
