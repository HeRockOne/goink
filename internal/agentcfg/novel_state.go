package agentcfg

import (
	"fmt"
	"strings"

	"gorm.io/gorm"

	"novel/internal/config"
	"novel/internal/git"
	"novel/internal/novel"
	"novel/internal/outline"
	"novel/internal/volume"
)

// NovelStatePrefix 是 NovelState 内容的固定开头，用于识别落库的 NS 快照消息
// （缓存协议：NS 落库到轮次末尾，识别/清理依赖此前缀，勿随意改动格式）。
const NovelStatePrefix = "【小说基础信息】\n"

// NovelState 快照消息在 extra_metadata 内的标记，供识别与过期清理（见 app/chat.go、internal/agent/compress.go）。
// NS 按"永不清理、由压缩兜底"协议落库（chat.go 注释：上一轮完整请求必须是下一轮请求的前缀）。
const (
	NovelStateKind     = "novel_state"
	NovelStateKindJSON = `{"kind":"novel_state"}`
	NovelStateKindLike = `%"kind":"novel_state"%`
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

	// 字数范围（从设置读取，每轮重建，DB 改了下轮自动生效）
	if settings, err := config.LoadSettings(db); err == nil {
		minW, maxW := 2500, 4000
		if settings.MinChapterWords > 0 {
			minW = settings.MinChapterWords
		}
		if settings.MaxChapterWords > 0 {
			maxW = settings.MaxChapterWords
		}
		b = append(b, fmt.Sprintf("字数范围：每章 %d-%d 字（硬约束，不足或超出都会被门禁拦截）\n", minW, maxW)...)
	}

	// 进度锚点：当前章节号（轮末动态字节，符合 P1 缓存协议）
	var totalChapters int64
	if err := db.Table("chapters").Where("novel_id = ?", novelID).Count(&totalChapters).Error; err == nil && totalChapters > 0 {
		b = append(b, fmt.Sprintf("当前进度：第 %d 章。创作须服务于全书总纲（outlines + outline_beats 表），只展开本卷情节，后续卷设定不得提前使用。\n", totalChapters)...)
		b = append(b, buildDirectionAnchor(db, novelID, int(totalChapters))...)
	}

	state, err := git.ReadFile(novelID, git.GoinkPath())
	if err == nil && state != "" {
		// goink.md 只记录章节指纹（追加式，最新在末尾）。
		// 注入最近 maxGoinkChars 字符（尾部截断，保留最新指纹供防重复，
		// 固定窗口字节稳定，符合 P1 缓存协议）。完整内容由 AI 用 read 按需读取。
		b = append(b, "\n【章节指纹（最近）】\n"...)
		const maxGoinkChars = 1000
		r := []rune(state)
		if len(r) > maxGoinkChars {
			b = append(b, string(r[len(r)-maxGoinkChars:])...)
			b = append(b, "\n…（更早指纹已截断，如需完整内容用 read(goink.md)）\n"...)
		} else {
			b = append(b, state...)
		}
	}

	return string(b), nil
}

// buildDirectionAnchor 构建方向锚区块：卷范围红线 + 类型承诺 + 未兑现爽点 + 活跃禁忌。
// 对抗长程创作中的渐进式设定漂移（每轮固定重建于动态区尾部，符合 P1 缓存协议；
// 各项数据缺失时静默跳过，不产生空行噪音）。书本设定优先于类型技能模板的仲裁规则固定附尾。
func buildDirectionAnchor(db *gorm.DB, novelID int64, currentChapter int) string {
	var lines []string

	// 本卷章节范围红线
	var vol volume.Volume
	if err := db.Where("novel_id = ? AND start_chapter <= ? AND end_chapter >= ?",
		novelID, currentChapter, currentChapter).Order("start_chapter").First(&vol).Error; err == nil {
		lines = append(lines, fmt.Sprintf("本卷红线：《%s》第%d-%d章，本章事件不得超出该范围、不得提前消耗后续卷冲突线", vol.Name, vol.StartChapter, vol.EndChapter))
	}

	// 类型承诺（总纲核心冲突截断）
	var ol outline.Outline
	if err := db.Where("novel_id = ?", novelID).First(&ol).Error; err == nil && ol.CoreConflict != "" {
		lines = append(lines, "类型承诺（总纲核心矛盾）："+truncateRunes(ol.CoreConflict, 80))
	}

	// 未兑现大爽点（接下来 3 个）
	var beats []outline.OutlineBeat
	if err := db.Where("novel_id = ? AND chapter > ?", novelID, currentChapter).
		Order("chapter").Limit(3).Find(&beats).Error; err == nil && len(beats) > 0 {
		parts := make([]string, 0, len(beats))
		for _, bt := range beats {
			parts = append(parts, fmt.Sprintf("Ch%d %s", bt.Chapter, truncateRunes(bt.Description, 30)))
		}
		lines = append(lines, "未兑现爽点（到期必须兑现，禁止替换或顺延）："+strings.Join(parts, "｜"))
	}

	// 活跃禁忌 top3
	var taboos []novel.PreferenceItem
	if err := db.Where("status = 'active' AND (is_global = ? OR novel_id = ?) AND category LIKE ?", true, novelID, "%禁忌%").
		Order("id").Limit(3).Find(&taboos).Error; err == nil && len(taboos) > 0 {
		parts := make([]string, 0, len(taboos))
		for _, tb := range taboos {
			parts = append(parts, truncateRunes(tb.Content, 50))
		}
		lines = append(lines, "活跃禁忌（违反=致命）：\n- "+strings.Join(parts, "\n- "))
	}

	if len(lines) == 0 {
		return ""
	}
	return "\n【方向锚】\n" + strings.Join(lines, "\n") +
		"\n冲突规则：以上书本设定（outlines/volumes/preferences/lore）优先于任何类型技能模板建议。\n"
}

// DirectionAnchor 导出方向锚内容，供 agent.go 在阶段推进/门禁提醒中引用。
func DirectionAnchor(db *gorm.DB, novelID int64, currentChapter int) string {
	return buildDirectionAnchor(db, novelID, currentChapter)
}

// truncateRunes 按字符数截断并加省略号。
func truncateRunes(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max]) + "…"
}
