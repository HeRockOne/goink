package app

import (
	"fmt"

	"novel/internal/lore"
)

type LoreListResult struct {
	Items      []lore.LoreEntry `json:"items"`
	Total      int64             `json:"total"`
	Page       int               `json:"page"`
	Size       int               `json:"size"`
	TotalPages int               `json:"total_pages"`
}

func (a *App) GetLoreList(novelID int64, category, search string, page, size int) (*LoreListResult, error) {
	result, err := a.lore.ListByNovel(a.ctx, novelID, lore.ListOptions{
		Page: page, Size: size, Category: category, Search: search,
	})
	if err != nil {
		return nil, fmt.Errorf("get lore: %w", err)
	}
	return &LoreListResult{
		Items: result.Items, Total: result.Total,
		Page: result.Page, Size: result.Size, TotalPages: result.TotalPages,
	}, nil
}

func (a *App) GetLoreDetail(loreID int64) (*lore.LoreEntry, error) {
	return a.lore.GetByID(a.ctx, loreID, a.settings.LastNovelID)
}

func (a *App) FindLore(novelID int64, query string) ([]lore.LoreEntry, error) {
	return a.lore.Search(a.ctx, novelID, query, 20)
}

func (a *App) CreateLore(novelID int64, title, category, content, summary string, arcID, revealChapterID *int64, isPublic bool) (*lore.LoreEntry, error) {
	if title == "" || category == "" {
		return nil, fmt.Errorf("标题和分类不能为空")
	}
	e := &lore.LoreEntry{
		NovelID: novelID, Title: title, Category: category,
		Content: content, Summary: summary, IsPublic: isPublic,
	}
	if arcID != nil { e.ArcID = arcID }
	if revealChapterID != nil { e.RevealChapterID = revealChapterID }
	if err := a.lore.Create(a.ctx, e); err != nil {
		return nil, fmt.Errorf("create lore: %w", err)
	}
	return e, nil
}

type UpdateLoreInput struct {
	Title           string `json:"title,omitempty"`
	Category        string `json:"category,omitempty"`
	Content         string `json:"content,omitempty"`
	Summary         string `json:"summary,omitempty"`
	ArcID           *int64 `json:"arc_id,omitempty"`
	RevealChapterID *int64 `json:"reveal_chapter_id,omitempty"`
	IsPublic        *bool  `json:"is_public,omitempty"`
	Source          string `json:"source,omitempty"`
}

func (a *App) UpdateLore(loreID int64, input UpdateLoreInput) error {
	e, err := a.lore.GetByID(a.ctx, loreID, a.settings.LastNovelID)
	if err != nil {
		return fmt.Errorf("lore not found: %w", err)
	}
	if input.Title != "" { e.Title = input.Title }
	if input.Category != "" { e.Category = input.Category }
	if input.Content != "" { e.Content = input.Content }
	if input.Summary != "" { e.Summary = input.Summary }
	if input.ArcID != nil { e.ArcID = input.ArcID }
	if input.RevealChapterID != nil { e.RevealChapterID = input.RevealChapterID }
	if input.IsPublic != nil { e.IsPublic = *input.IsPublic }
	if input.Source != "" { e.Source = input.Source }
	return a.lore.Update(a.ctx, e)
}

func (a *App) DeleteLore(loreID int64) error {
	return a.lore.Delete(a.ctx, loreID, a.settings.LastNovelID)
}
