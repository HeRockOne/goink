package app

import (
	"fmt"

	"novel/internal/chapter"
	"novel/internal/git"
)

// CreateChapterInput 是创建章节的入参。
type CreateChapterInput struct {
	NovelID int64  `json:"novel_id"`
	Title   string `json:"title"`
}

// ── 章节 ──────────────────────────────────────────────────

// GetChapters 返回指定小说的章节列表，含文件路径。
func (a *App) GetChapters(novelID int64) ([]chapter.Chapter, error) {
	chapters, err := a.chapter.ListAllByNovel(a.ctx, novelID)
	if err != nil {
		return nil, err
	}
	return chapters, nil
}

// ListOutlines 返回 outlines/ 目录下的大纲文件列表（章节号升序），侧边栏大纲列表用。
func (a *App) ListOutlines(novelID int64) ([]git.OutlineEntry, error) {
	return git.ListOutlines(novelID)
}

// GetMaxChapterNumber 返回该小说当前最大章节号，无章节时返回 0。前端确定写作进度用。
func (a *App) GetMaxChapterNumber(novelID int64) (int, error) {
	return a.chapter.GetLatestNumber(a.ctx, novelID)
}

// UpdateChapterTitle 更新章节标题。
func (a *App) UpdateChapterTitle(novelID int64, chapterNumber int, title string) error {
	return a.chapter.UpdateTitle(a.ctx, novelID, chapterNumber, title)
}

// DeleteChapter 删除指定章节，同时删除正文文件和数据库记录。
func (a *App) DeleteChapter(novelID int64, chapterNumber int) error {
	if err := git.DeleteFile(novelID, git.ChapterPath(chapterNumber)); err != nil {
		return fmt.Errorf("删除章节文件失败: %w", err)
	}
	if err := a.chapter.Delete(a.ctx, novelID, chapterNumber); err != nil {
		return fmt.Errorf("删除章节记录失败: %w", err)
	}
	return nil
}

// DeleteOutlineFile 删除指定章节的大纲文件。
func (a *App) DeleteOutlineFile(novelID int64, chapterNumber int) error {
	return git.DeleteFile(novelID, git.OutlinePath(chapterNumber))
}

// CreateChapter 创建新章节，章节号自动递增。同时创建空正文文件。
func (a *App) CreateChapter(input CreateChapterInput) (*chapter.Chapter, error) {
	latest, err := a.chapter.GetLatestNumber(a.ctx, input.NovelID)
	if err != nil {
		return nil, fmt.Errorf("failed to create chapter: %w", err)
	}

	ch := chapter.Chapter{
		NovelID:       input.NovelID,
		ChapterNumber: latest + 1,
		Title:         input.Title,
	}

	if err := a.chapter.DB.WithContext(a.ctx).Create(&ch).Error; err != nil {
		return nil, fmt.Errorf("failed to create chapter: %w", err)
	}

	if err := git.WriteFile(input.NovelID, git.ChapterPath(ch.ChapterNumber), ""); err != nil {
		return nil, fmt.Errorf("failed to create chapter: %w", err)
	}

	ch.FilePath = git.ChapterPath(ch.ChapterNumber)

	return &ch, nil
}
