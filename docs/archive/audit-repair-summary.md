# goink 审计问题修复报告

**修复日期**：2026-07-28
**审计范围**：Schema Required 缺失 + 看板筛选规则漏洞
**状态**：P0/P1/P2 问题全部修复

---

## 一、修复清单

### P0 - 立即修复（已完成）

| # | 问题 | 文件 | 修复内容 |
|---|------|------|---------|
| P0-1 | create_item_occurrence required=[] | `item_occurrence_tools.go` | 确认 `ItemID`/`ChapterID`/`Action` 已有 jsonschema required |
| P0-2 | create_timeline_entry required=[] | `timeline_tools.go` | 确认 `Category`/`TargetChapter` 已有 jsonschema required |
| P0-3 | 读者卡片筛选范围过窄 | `writing_context.go` | 改为展示所有未揭示条目 |
| P0-4 | 当前卡片漏掉"本章应收" | `NarrativeTimeline.tsx` | `target_chapter >= current` |

### P1 - 短期修复（已完成）

| # | 问题 | 文件 | 修复内容 |
|---|------|------|---------|
| P1-5 | create_scene chapter_id 非必填 | `scene_tools.go` | 确认 chapter_id 已有 required |
| P1-6 | create_character location_id 非必填 | `character_tools.go:178` | 新增 `validate:"required"` |
| P1-7 | 文档漏写 scenes | `writing_context.go` | 新增 `Scenes []WritingSceneBrief` 字段 |
| P1-8 | 角色地点是静态值 | `NarrativeTimeline.tsx` | 增加 `title="角色静态存储位置..."` 提示 |

### P2 - 中期优化（已完成）

| # | 问题 | 文件 | 修复内容 |
|---|------|------|---------|
| P2-11 | 弧线缺少当前节点高亮 | `NarrativeTimeline.tsx:180` | 当前节点增加 `← 当前` 标记和高亮背景 |
| P2-12 | 章纲解析覆盖率 85% | `OutlineParser.ts` | 支持多种格式变体（注：后续改为 react-markdown 直接渲染原始大纲，不再依赖语义解析） |

---

## 二、修复详情

### 2.1 P0 修复

#### 2.1.1 后端 Schema Required

通过代码审查确认以下工具的 required 标注正确：

- `create_item_occurrence`: `ItemID`/`ChapterID`/`Action` 已有 `jsonschema:"required"`
- `create_timeline_entry`: `Category`/`TargetChapter` 已有 `jsonschema:"required"`

#### 2.1.2 看板筛选规则

**writing_context.go 修复**：
```go
// 修复前：只展示最近2章
minChapter := chapterNum - 1
a.db.WithContext(ctx).Where("novel_id = ? AND planted_chapter >= ?", novelID, minChapter)

// 修复后：展示所有未揭示条目
a.db.WithContext(ctx).Where("novel_id = ? AND (revealed_chapter = 0 OR revealed_chapter IS NULL)", novelID).
    Order("planted_chapter DESC, id DESC").Limit(20).Find(&recentEntries)
```

**NarrativeTimeline.tsx 修复**：
```tsx
// 修复前
Object.entries(pbc).filter(([k]) => +k === ch.num + 1 || +k === ch.num + 2)

// 修复后
Object.entries(pbc).filter(([k]) => +k >= ch.num)
```

### 2.2 P1 修复

#### 2.2.1 create_character location_id 加 required

**character_tools.go**：
```go
// 修复前
LocationID  *int64  `json:"location_id" jsonschema:"description=角色当前所在地点ID"`

// 修复后
LocationID  *int64  `json:"location_id" jsonschema:"required,description=角色当前所在地点ID" validate:"required"`
```

#### 2.2.2 writing_context 添加 scenes 字段

**writing_context.go**：
```go
type WritingSceneBrief struct {
    ID           int64  `json:"id"`
    SceneNumber  int    `json:"scene_number"`
    Title        string `json:"title"`
    LocationName string `json:"location_name,omitempty"`
    Summary      string `json:"summary,omitempty"`
}
```

### 2.3 P2 修复

#### 2.3.1 弧线卡片当前节点高亮

```tsx
// 当前节点添加 ← 当前 标记和高亮背景
const isCurrent = isNext && !(a.nodes || []).some((nn: any) =>
    nn.status !== 'completed' && nn.target_chapter > n.target_chapter && nn.target_chapter <= ch.num);
return <div ... style={{
    borderLeft: isCurrent ? '3px solid var(--primary)' : '2px solid var(--primary)',
    background: isCurrent ? 'color-mix(in oklab, var(--primary) 8%, transparent)' : undefined
}}>
    {n.title}{isCurrent && ' ← 当前'}
</div>
```

#### 2.3.2 章纲解析覆盖率提升

**OutlineParser.ts 改进（已过时，未来卡片改用 react-markdown 渲染原始大纲）**：
1. `splitByHeaders`: 支持 `**字段名**：格式（无 ## 标题的情况）
2. `extractScenes`: 支持 `1. **标题**（情绪功能：描述）` 格式
3. `extractListItems`: 支持更多列表前缀格式
4. `extractEmotionalDesign`: 支持 `- **锚点**：格式`
5. `extractForeshadowing`: 支持 `- 回收：`、`- 埋下：`、`- 推进：` 格式

---

## 三、构建验证

```bash
.\build.ps1
```

---

**修复完成，所有 P0/P1/P2 问题已解决**
