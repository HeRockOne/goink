package agentcfg

import (
	"fmt"

	"gorm.io/gorm"

	"novel/internal/git"
	"novel/internal/novel"
)

// NovelStatePrefix 是 NovelState 内容的固定开头，用于识别落库的 NS 快照消息
// （缓存协议：NS 落库到轮次末尾，识别/清理依赖此前缀，勿随意改动格式）。
const NovelStatePrefix = "【小说基础信息】\n"

// NovelState 快照消息在 extra_metadata 内的标记，供识别与过期清理（见 app/chat.go、internal/agent/compress.go）。
const (
	NovelStateKind       = "novel_state"
	NovelStateKindJSON   = `{"kind":"novel_state"}`
	NovelStateKindLike   = `%"kind":"novel_state"%`
	KeepNovelStateSnapshots = 3 // 每轮保留的 NS 快照份数（其余置 to_api=false）
)

// NovelState 构建小说上下文快照（原 System3），每轮对话开头注入。
// 只包含基本信息 + 故事状态。具体数据（角色、时间线等）由 MCP 工具按需提供。
func NovelState(db *gorm.DB, novelID int64) (string, error) {
	var n novel.Novel
	if err := db.First(&n, novelID).Error; err != nil {
		return "", fmt.Errorf("agentcfg: load novel %d: %w", novelID, err)
	}

	var b []byte
	b = append(b, NovelStatePrefix...)
	b = append(b, fmt.Sprintf("书名：%s\n", n.Title)...)
	if n.Genre != "" {
		b = append(b, fmt.Sprintf("类型：%s\n", n.Genre)...)
	}
	if n.Description != "" {
		b = append(b, fmt.Sprintf("简介：%s\n", n.Description)...)
	}

	// 进度锚点：当前章节号（轮末动态字节，符合 P1 缓存协议）
	var totalChapters int64
	if err := db.Table("chapters").Where("novel_id = ?", novelID).Count(&totalChapters).Error; err == nil && totalChapters > 0 {
		b = append(b, fmt.Sprintf("当前进度：第 %d 章。创作须服务于全书总纲（book-outline.md），只展开本卷情节，后续卷设定不得提前使用。\n", totalChapters)...)
	}

	state, err := git.ReadFile(novelID, git.GoinkPath())
	if err == nil && state != "" {
		b = append(b, "\n【故事状态文档】\n"...)
		b = append(b, state...)
	}

	return string(b), nil
}
