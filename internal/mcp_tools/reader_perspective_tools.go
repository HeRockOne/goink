package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"

	"novel/internal/reader"
)

// ── get_reader_perspective ──────────────────────────────

// GetReaderPerspectiveArgs 支持按类型过滤和计数模式。
type GetReaderPerspectiveArgs struct {
	Type         string `json:"type" jsonschema:"description=过滤类型：known/suspense/misconception，空=返回全部"`
	CountsOnly   bool   `json:"counts_only" jsonschema:"description=true=只返回各类型条目数，不返回具体内容（省token）"`
	Search       string `json:"search" jsonschema:"description=按内容关键词定向查找条目（如角色名/事件），只返回匹配条目（省token）"`
	PlantedFrom  int    `json:"planted_from,omitempty" jsonschema:"description=种植章节起始（含），限定条目范围（省token）" validate:"omitempty,min=1"`
	PlantedTo    int    `json:"planted_to,omitempty" jsonschema:"description=种植章节结束（含），限定条目范围（省token）" validate:"omitempty,min=1"`
}

// GetReaderPerspectiveTool 返回读者当前认知状态的三段式摘要。
// known 兜底截断 60 条——完整认知上下文只需最近的关键事实。
type GetReaderPerspectiveTool struct{}

func (t *GetReaderPerspectiveTool) Name() string { return "get_reader_perspective" }
func (t *GetReaderPerspectiveTool) Description() string {
	return "获取当前小说的读者认知状态：已知信息、活跃悬念、读者误知。" +
		"每条条目末尾的 `[entry_id:X]` 是该条目的唯一标识，更新或回收时填入 entry_id。" +
		"尽量合并同类信息到已有条目，减少重复创建。只记录读者一定会在意，后续创作需要考虑的条目。" +
		"【省token指令】counts_only=true 只返回各类型数量，不返回具体内容。" +
		"【省token指令】type=known/suspense/misconception 只获取需要的类型。" +
		"【省token指令】search=关键词 / planted_from~planted_to=种植章节范围 定向查目标条目——禁止无过滤全量拉取活跃条目（上限 100 条）。"
}
func (t *GetReaderPerspectiveTool) Category() ToolCategory { return CategoryMemoryRetrieval }

func (t *GetReaderPerspectiveTool) JSONSchema() json.RawMessage {
	return SchemaOf(GetReaderPerspectiveArgs{})
}

func (t *GetReaderPerspectiveTool) ExposeToLLM() bool { return true }
func (t *GetReaderPerspectiveTool) NewArgs() any      { return &GetReaderPerspectiveArgs{} }

func (t *GetReaderPerspectiveTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*GetReaderPerspectiveArgs)
	rs := reader.NewStore(tc.DB, slog.Default())

	hasFilter := a.Type != ""
	countsOnly := a.CountsOnly

	// 如果指定了类型，只查该类型
	var knownItems []reader.ReaderPerspective
	if !hasFilter || a.Type == "known" {
		if err := tc.DB.WithContext(ctx).
			Where("novel_id = ? AND type = ?", tc.NovelID, reader.TypeKnown).
			Order("planted_chapter DESC").
			Limit(60).
			Find(&knownItems).Error; err != nil {
			return nil, fmt.Errorf("query known perspectives: %w", err)
		}
	}

	var suspenses []reader.ReaderPerspective
	var misconceptions []reader.ReaderPerspective
	if !hasFilter || a.Type == "suspense" || a.Type == "misconception" {
		// 定向过滤（search/planted 范围）优先：替代全量拉取后自己筛，省 token
		if a.Search != "" || a.PlantedFrom > 0 || a.PlantedTo > 0 {
			active, err := rs.ListActiveFiltered(ctx, tc.NovelID, a.Search, a.PlantedFrom, a.PlantedTo)
			if err != nil {
				return nil, fmt.Errorf("query active perspectives: %w", err)
			}
			for _, e := range active {
				if hasFilter && e.Type != a.Type {
					continue
				}
				switch e.Type {
				case reader.TypeSuspense:
					suspenses = append(suspenses, e)
				case reader.TypeMisconception:
					misconceptions = append(misconceptions, e)
				}
			}
		} else {
			active, err := rs.ListActive(ctx, tc.NovelID)
			if err != nil {
				return nil, fmt.Errorf("query active perspectives: %w", err)
			}
			for _, e := range active {
				if hasFilter && e.Type != a.Type {
					continue
				}
				switch e.Type {
				case reader.TypeSuspense:
					suspenses = append(suspenses, e)
				case reader.TypeMisconception:
					misconceptions = append(misconceptions, e)
				}
			}
		}
	}

	counts := map[string]int{
		"known":         len(knownItems),
		"suspense":      len(suspenses),
		"misconception": len(misconceptions),
	}

	if countsOnly {
		return &ToolResult{
			Success: true,
			Data:    map[string]any{"counts": counts},
		}, nil
	}

	formatted := formatReaderPerspective(knownItems, suspenses, misconceptions)

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"content": formatted,
			"counts":  counts,
		},
	}, nil
}

// ── 格式化 ──────────────────────────────────────────────

func formatReaderPerspective(known, suspenses, misconceptions []reader.ReaderPerspective) string {
	var sections []string

	ref := func(e reader.ReaderPerspective) string {
		return fmt.Sprintf(" `[entry_id:%d]`", e.ID)
	}

	// 已知信息
	if len(known) > 0 {
		lines := []string{"### 已知信息"}
		for _, e := range known {
			lines = append(lines, fmt.Sprintf("- %s [第%d章起]%s", e.Content, e.PlantedChapter, ref(e)))
		}
		sections = append(sections, strings.Join(lines, "\n"))
	}

	// 活跃悬念
	if len(suspenses) > 0 {
		lines := []string{"### 活跃悬念"}
		for _, e := range suspenses {
			lines = append(lines, fmt.Sprintf("- %s（第%d章种下）%s", e.Content, e.PlantedChapter, ref(e)))
		}
		sections = append(sections, strings.Join(lines, "\n"))
	}

	// 读者误知
	if len(misconceptions) > 0 {
		lines := []string{"### 读者误知"}
		for _, e := range misconceptions {
			truth := ""
			if e.RelatedTruth != "" {
				truth = fmt.Sprintf(" → 实际：%s", e.RelatedTruth)
			}
			lines = append(lines, fmt.Sprintf("- %s%s%s", e.Content, truth, ref(e)))
		}
		sections = append(sections, strings.Join(lines, "\n"))
	}

	if len(sections) == 0 {
		return "暂无读者认知数据。"
	}
	return strings.Join(sections, "\n\n")
}

// ── create_reader_perspective_entry ──────────────────────

// CreateReaderPerspectiveEntryItem 是 create_reader_perspective_entry 的单条参数。
type CreateReaderPerspectiveEntryItem struct {
	Type           string `json:"type" jsonschema:"required,description=条目类型,enum=known,enum=suspense,enum=misconception" validate:"required,oneof=known suspense misconception"`
	Content        string `json:"content" jsonschema:"required,description=内容描述"          validate:"required"`
	PlantedChapter int    `json:"planted_chapter" jsonschema:"required,description=种下的章节号"    validate:"required,min=1"`
	RelatedTruth   string `json:"related_truth" jsonschema:"description=仅 misconception：真实情况是什么"`
}

// CreateReaderPerspectiveEntryArgs 是 create_reader_perspective_entry 的参数。
type CreateReaderPerspectiveEntryArgs struct {
	Entries []CreateReaderPerspectiveEntryItem `json:"entries" jsonschema:"required,description=要创建的读者认知条目（1-10个）" validate:"required,min=1,max=10,dive"`
}

// CreateReaderPerspectiveEntryTool 创建一条读者认知条目。
type CreateReaderPerspectiveEntryTool struct{}

func (t *CreateReaderPerspectiveEntryTool) Name() string { return "create_reader_perspective_entry" }
func (t *CreateReaderPerspectiveEntryTool) Description() string {
	return "批量添加读者认知条目（1-10个）。保证原子性，失败时返回具体条目原因。三种类型：\n" +
		"- known：读者在某章之后知道了什么\n" +
		"- suspense：读者当前在等待解答的悬念\n" +
		"- misconception：读者以为的情况（用于未来反转）\n" +
		"每章写完后如有新揭露的信息或新种下的悬念，应主动添加。"
}
func (t *CreateReaderPerspectiveEntryTool) Category() ToolCategory { return CategoryWritingAssistant }

func (t *CreateReaderPerspectiveEntryTool) JSONSchema() json.RawMessage {
	return SchemaOf(CreateReaderPerspectiveEntryArgs{})
}

func (t *CreateReaderPerspectiveEntryTool) ExposeToLLM() bool { return true }
func (t *CreateReaderPerspectiveEntryTool) NewArgs() any      { return &CreateReaderPerspectiveEntryArgs{} }

func (t *CreateReaderPerspectiveEntryTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*CreateReaderPerspectiveEntryArgs)

	// 预校验：misconception 必须提供 related_truth
	for _, item := range a.Entries {
		if item.Type == reader.TypeMisconception && item.RelatedTruth == "" {
			return &ToolResult{Success: false, Error: "misconception 类型必须提供 related_truth（实际真相）"}, nil
		}
	}

	var ids []int64
	var failedName string
	var failedErr error
	err := tc.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range a.Entries {
			rp := reader.ReaderPerspective{
				NovelID:        tc.NovelID,
				Type:           item.Type,
				Content:        item.Content,
				PlantedChapter: item.PlantedChapter,
				RelatedTruth:   item.RelatedTruth,
			}
			if err := tx.Create(&rp).Error; err != nil {
				failedName = item.Content
				if len(failedName) > 20 {
					failedName = failedName[:20]
				}
				failedErr = err
				return err
			}
			ids = append(ids, rp.ID)
		}
		return nil
	})
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("创建读者认知条目 [%s] 失败: %s", failedName, failedErr),
		}, nil
	}

	return &ToolResult{
		Success: true,
		Data:    map[string]any{"ids": ids, "count": len(ids)},
	}, nil
}

// ── update_reader_perspective_entry ──────────────────────

// UpdateReaderPerspectiveEntryArgs 是 update_reader_perspective_entry 的参数。
type UpdateReaderPerspectiveEntryArgs struct {
	EntryID         int    `json:"entry_id" jsonschema:"required,description=要更新的条目 ID" validate:"required,min=1"`
	Content         string `json:"content" jsonschema:"description=更新后的完整内容描述"`
	RevealedChapter int    `json:"revealed_chapter" jsonschema:"description=实际揭露或回收的章节号（设置后该条目不再出现在活跃列表中）" validate:"omitempty,min=0"`
	PlantedChapter  int    `json:"planted_chapter" jsonschema:"description=在哪章种下的章节号" validate:"omitempty,min=1"`
	RelatedTruth    string `json:"related_truth" jsonschema:"description=作者视角的真实情况（支持所有类型）"`
	Type            string `json:"type" jsonschema:"description=条目类型,enum=known,enum=suspense,enum=misconception"`
}

// UpdateReaderPerspectiveEntryTool 更新读者认知条目。
type UpdateReaderPerspectiveEntryTool struct{}

func (t *UpdateReaderPerspectiveEntryTool) Name() string { return "update_reader_perspective_entry" }
func (t *UpdateReaderPerspectiveEntryTool) Description() string {
	return "更新一条读者认知条目。常见用途：\n" +
		"- 回收悬念：设置 revealed_chapter（悬念在剧情中解答后必须回收，否则读者认知滞后）\n" +
		"- 揭露误知：设置 revealed_chapter\n" +
		"【使用时机】每章维护阶段：悬念回收/误知揭露/已知信息更新时同步。" +
		"【注意】只记录读者实际会感知的信息——不要用作者全知视角污染 known 条目（信息越界会破坏反转设计）。"
}
func (t *UpdateReaderPerspectiveEntryTool) Category() ToolCategory { return CategoryWritingAssistant }

func (t *UpdateReaderPerspectiveEntryTool) JSONSchema() json.RawMessage {
	return SchemaOf(UpdateReaderPerspectiveEntryArgs{})
}

func (t *UpdateReaderPerspectiveEntryTool) ExposeToLLM() bool { return true }
func (t *UpdateReaderPerspectiveEntryTool) NewArgs() any      { return &UpdateReaderPerspectiveEntryArgs{} }

func (t *UpdateReaderPerspectiveEntryTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpdateReaderPerspectiveEntryArgs)

	if a.RevealedChapter == 0 && a.PlantedChapter == 0 && a.Content == "" && a.Type == "" && a.RelatedTruth == "" {
		return &ToolResult{Success: false, Error: "至少需要提供一个要修改的字段"}, nil
	}

	var entry reader.ReaderPerspective
	if err := tc.DB.WithContext(ctx).
		Where("id = ? AND novel_id = ?", a.EntryID, tc.NovelID).
		First(&entry).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &ToolResult{Success: false, Error: fmt.Sprintf("条目 %d 不存在", a.EntryID)}, nil
		}
		return nil, fmt.Errorf("query perspective entry: %w", err)
	}

	json.Unmarshal(tc.RawArgs, &entry)

	if a.Content != "" {
		entry.Content = a.Content
	}
	if a.RevealedChapter > 0 {
		entry.RevealedChapter = a.RevealedChapter
	}
	if a.PlantedChapter > 0 {
		entry.PlantedChapter = a.PlantedChapter
	}
	if a.RelatedTruth != "" {
		entry.RelatedTruth = a.RelatedTruth
	}
	if a.Type != "" {
		entry.Type = a.Type
	}

	if err := tc.DB.WithContext(ctx).Save(&entry).Error; err != nil {
		return nil, fmt.Errorf("save perspective entry: %w", err)
	}

	return &ToolResult{
		Success: true,
		Data:    map[string]any{"id": entry.ID, "revealed_chapter": entry.RevealedChapter},
	}, nil
}

// ── 注册 ──────────────────────────────────────────────

// RegisterReaderPerspectiveTools 注册读者认知类工具。
func RegisterReaderPerspectiveTools(r *Registry) {
	r.Register(&GetReaderPerspectiveTool{})
	r.Register(&CreateReaderPerspectiveEntryTool{})
	r.Register(&UpdateReaderPerspectiveEntryTool{})
}
