package search

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"novel/internal/chapter"
	"novel/internal/character"
	"novel/internal/item"
	"novel/internal/location"
	"novel/internal/lore"
	"novel/internal/rag"
	"novel/internal/storage"
	"novel/internal/storyarc"
	"novel/internal/timeline"
)

type Service struct {
	logger      *slog.Logger
	charStore   *character.Store
	locStore    *location.Store
	loreStore   *lore.Store
	itemStore   *item.Store
	tlStore     *timeline.Store
	arcStore    *storyarc.Store
	chapStore   *chapter.Store
	vectorStore *rag.VectorStore
	mu          sync.RWMutex
	cache       map[int64]map[int]string
}

func NewService(logger *slog.Logger, charStore *character.Store, locStore *location.Store,
	loreStore *lore.Store, itemStore *item.Store,
	tlStore *timeline.Store, arcStore *storyarc.Store, chapStore *chapter.Store,
	vecStore *rag.VectorStore) *Service {
	return &Service{
		logger: logger, charStore: charStore, locStore: locStore,
		loreStore: loreStore, itemStore: itemStore,
		tlStore: tlStore, arcStore: arcStore, chapStore: chapStore,
		vectorStore: vecStore, cache: make(map[int64]map[int]string),
	}
}

func (s *Service) SearchAll(ctx context.Context, novelID int64, query string) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" { return nil, nil }
	var entityResults, contentResults []Result
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); entityResults = s.searchEntities(ctx, novelID, query) }()
	go func() { defer wg.Done(); contentResults = s.searchContent(ctx, novelID, query) }()
	wg.Wait()
	var results []Result
	results = append(results, entityResults...)
	results = append(results, contentResults...)
	return results, nil
}

func (s *Service) searchEntities(ctx context.Context, novelID int64, query string) []Result {
	var results []Result
	// 角色
	if chars, err := s.charStore.ListByNovel(ctx, novelID, character.ListByNovelOptions{Search: query, PageParams: storage.PageParams{Page: 1, Size: EntityLimit}}); err == nil && chars != nil {
		for _, c := range chars.Items { results = append(results, Result{Type: "character", ID: c.ID, Title: c.Name, PanelID: "characters"}) }
	}
	// 地点
	if locs, err := s.locStore.ListByNovel(ctx, novelID, location.ListByNovelOptions{Search: query, PageParams: storage.PageParams{Page: 1, Size: EntityLimit}}); err == nil && locs != nil {
		for _, l := range locs.Items { results = append(results, Result{Type: "location", ID: l.ID, Title: l.Name, Subtitle: l.LocationType, PanelID: "locations"}) }
	}
	// 设定
	if loreEntries, err := s.loreStore.Search(ctx, novelID, query, EntityLimit); err == nil {
		for _, e := range loreEntries { results = append(results, Result{Type: "lore", ID: e.ID, Title: e.Title, Subtitle: e.Category, PanelID: "world"}) }
	}
	// 物品
	if items, err := s.itemStore.Search(ctx, novelID, query, EntityLimit); err == nil {
		for _, it := range items { results = append(results, Result{Type: "item", ID: it.ID, Title: it.Name, Subtitle: it.ItemType, PanelID: "items"}) }
	}
	// 时间线
	if tl, err := s.tlStore.SearchByNovel(ctx, novelID, query, EntityLimit); err == nil {
		for _, e := range tl { results = append(results, Result{Type: "timeline", ID: e.ID, Title: e.Title, Subtitle: e.Category, ChapterNum: e.TargetChapter, PanelID: "timeline"}) }
	}
	// 弧线
	if arcs, err := s.arcStore.SearchByNovel(ctx, novelID, query, EntityLimit); err == nil {
		for _, arc := range arcs { results = append(results, Result{Type: "storyarc", ID: arc.ID, Title: arc.Name, Subtitle: arc.ArcType, PanelID: "storyarcs"}) }
	}
	// 章节标题
	if chs, err := s.chapStore.SearchByNovel(ctx, novelID, query, EntityLimit); err == nil {
		for _, ch := range chs { results = append(results, Result{Type: "chapter", ID: ch.ID, Title: ch.Title, Subtitle: "标题匹配", ChapterNum: ch.ChapterNumber, FilePath: ch.FilePath, PanelID: "chapters"}) }
	}
	return results
}


func (s *Service) UpdateCachedChapter(novelID int64, chapterNum int, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cache[novelID] == nil { s.cache[novelID] = make(map[int]string) }
	s.cache[novelID][chapterNum] = content
}

func (s *Service) searchContent(ctx context.Context, novelID int64, query string) []Result {
	if s.vectorStore == nil { return nil }
	vecResults, err := s.vectorStore.Search(ctx, novelID, query, RagTopK, nil)
	if err != nil { s.logger.Warn("vector search failed", "err", err); return nil }
	var results []Result
	for _, vr := range vecResults {
		filePath := ""
		if vr.ChapterNumber > 0 {
			filePath = fmt.Sprintf("chapters/%03d.md", vr.ChapterNumber)
		}
		results = append(results, Result{
			Type:      "rag",
			Title:     fmt.Sprintf("第 %d 章", vr.ChapterNumber),
			Subtitle:  vr.Content,
			ChapterNum: vr.ChapterNumber,
			FilePath:  filePath,
			MatchPosition: vr.StartRunePos,
			MatchLen:  len([]rune(vr.Content)),
			Relevance: vr.Relevance,
		})
	}
	return results
}
