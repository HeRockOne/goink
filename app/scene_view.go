package app

import (
	"fmt"

	"novel/internal/scene"
)

func (a *App) GetSceneList(novelID, chapterID int64) ([]scene.Scene, error) {
	return a.scene.ListByChapter(a.ctx, novelID, chapterID)
}

func (a *App) GetSceneDetail(sceneID int64) (*scene.Scene, error) {
	return a.scene.GetByID(a.ctx, sceneID, a.settings.LastNovelID)
}

func (a *App) CreateScene(novelID int64, chapterID int64, sceneNumber int, title string, locationID int64, characterIDs string, wordCount int, summary string) (*scene.Scene, error) {
	sc := &scene.Scene{
		NovelID: novelID, ChapterID: chapterID, SceneNumber: sceneNumber,
		Title: title, CharacterIDs: characterIDs, WordCount: wordCount, Summary: summary,
	}
	if locationID > 0 { sc.LocationID = &locationID }
	if err := a.scene.Create(a.ctx, sc); err != nil {
		return nil, fmt.Errorf("create scene: %w", err)
	}
	return sc, nil
}

func (a *App) UpdateScene(sceneID int64, sceneNumber int, title string, locationID int64, characterIDs string, wordCount int, summary string) error {
	sc, err := a.scene.GetByID(a.ctx, sceneID, a.settings.LastNovelID)
	if err != nil { return fmt.Errorf("scene not found: %w", err) }
	if sceneNumber > 0 { sc.SceneNumber = sceneNumber }
	if title != "" { sc.Title = title }
	if locationID > 0 { sc.LocationID = &locationID }
	if characterIDs != "" { sc.CharacterIDs = characterIDs }
	if wordCount > 0 { sc.WordCount = wordCount }
	if summary != "" { sc.Summary = summary }
	return a.scene.Update(a.ctx, sc)
}

func (a *App) DeleteScene(sceneID int64) error {
	return a.scene.Delete(a.ctx, sceneID, a.settings.LastNovelID)
}
