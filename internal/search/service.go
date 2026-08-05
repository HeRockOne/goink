package search

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

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
	if query == "" {
		return nil, nil
	}
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
		for _, c := range chars.Items {
			results = append(results, Result{Type: "character", ID: c.ID, Title: c.Name, PanelID: "characters"})
		}
	}
	// 地点
	if locs, err := s.locStore.ListByNovel(ctx, novelID, location.ListByNovelOptions{Search: query, PageParams: storage.PageParams{Page: 1, Size: EntityLimit}}); err == nil && locs != nil {
		for _, l := range locs.Items {
			results = append(results, Result{Type: "location", ID: l.ID, Title: l.Name, Subtitle: l.LocationType, PanelID: "locations"})
		}
	}
	// 设定
	if loreEntries, err := s.loreStore.Search(ctx, novelID, query, EntityLimit); err == nil {
		for _, e := range loreEntries {
			results = append(results, Result{Type: "lore", ID: e.ID, Title: e.Title, Subtitle: e.Category, PanelID: "world"})
		}
	}
	// 物品
	if items, err := s.itemStore.Search(ctx, novelID, query, EntityLimit); err == nil {
		for _, it := range items {
			results = append(results, Result{Type: "item", ID: it.ID, Title: it.Name, Subtitle: it.ItemType, PanelID: "items"})
		}
	}
	// 时间线
	if tl, err := s.tlStore.SearchByNovel(ctx, novelID, query, EntityLimit); err == nil {
		for _, e := range tl {
			results = append(results, Result{Type: "timeline", ID: e.ID, Title: e.Title, Subtitle: e.Category, ChapterNum: e.TargetChapter, PanelID: "timeline"})
		}
	}
	// 弧线
	if arcs, err := s.arcStore.SearchByNovel(ctx, novelID, query, EntityLimit); err == nil {
		for _, arc := range arcs {
			results = append(results, Result{Type: "storyarc", ID: arc.ID, Title: arc.Name, Subtitle: arc.ArcType, PanelID: "storyarcs"})
		}
	}
	// 章节标题
	if chs, err := s.chapStore.SearchByNovel(ctx, novelID, query, EntityLimit); err == nil {
		for _, ch := range chs {
			results = append(results, Result{Type: "chapter", ID: ch.ID, Title: ch.Title, Subtitle: "标题匹配", ChapterNum: ch.ChapterNumber, FilePath: ch.FilePath, PanelID: "chapters"})
		}
	}
	return results
}

func (s *Service) UpdateCachedChapter(novelID int64, chapterNum int, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cache[novelID] == nil {
		s.cache[novelID] = make(map[int]string)
	}
	s.cache[novelID][chapterNum] = content
}

func (s *Service) searchContent(ctx context.Context, novelID int64, query string) []Result {
	var results []Result
	// 1. 写入缓存降级关键词匹配：正文保存后立即写入 cache，
	//    向量/FTS 尚未刷新完成时也能即时搜到新内容（Type=content）
	results = append(results, s.searchCache(novelID, query)...)
	// 2. 向量语义 + FTS5 关键词 3 路合并（RRF 融合，按 (章节, 位置) 去重）
	if s.vectorStore != nil {
		results = append(results, s.searchVectorRRF(ctx, novelID, query)...)
	}
	return results
}

// searchCache 在写入缓存中做关键词匹配（向量未就绪时的降级源）。
func (s *Service) searchCache(novelID int64, query string) []Result {
	s.mu.RLock()
	chaps := s.cache[novelID]
	s.mu.RUnlock()
	if len(chaps) == 0 {
		return nil
	}
	qRunes := []rune(query)
	var out []Result
	for num, content := range chaps {
		bytePos := 0
		runes := []rune(content)
		for {
			idx := strings.Index(content[bytePos:], query)
			if idx < 0 {
				break
			}
			absByte := bytePos + idx
			pos := utf8.RuneCountInString(content[:absByte])
			prefix := runeSlice(runes, pos-ContextRadius, ContextRadius)
			suffix := runeSlice(runes, pos+len(qRunes), ContextRadius)
			out = append(out, Result{
				Type:          "content",
				Title:         fmt.Sprintf("第 %d 章", num),
				ChapterNum:    num,
				MatchPrefix:   prefix,
				MatchHit:      query,
				MatchSuffix:   suffix,
				MatchPosition: pos,
				MatchLen:      len(qRunes),
				Relevance:     1,
			})
			if len(out) >= ContentLimit {
				return out
			}
			bytePos = absByte + len(query)
		}
	}
	return out
}

// runeSlice 从 runes 中取以 start 为中心的前后共 radius 的窗口（负偏移安全）。
func runeSlice(runes []rune, start, radius int) string {
	from := start - radius
	if from < 0 {
		from = 0
	}
	to := start + radius
	if to > len(runes) {
		to = len(runes)
	}
	return string(runes[from:to])
}

// searchVectorRRF 向量 + FTS5 两路合并检索。
func (s *Service) searchVectorRRF(ctx context.Context, novelID int64, query string) []Result {
	var vecResults, ftsResults []rag.SearchResult
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		r, err := s.vectorStore.Search(ctx, novelID, query, RagTopK, nil)
		if err != nil {
			s.logger.Warn("vector search failed", "err", err)
			return
		}
		vecResults = r
	}()
	go func() {
		defer wg.Done()
		r, err := s.vectorStore.FtsSearch(ctx, novelID, query, RagTopK*2)
		if err != nil {
			s.logger.Warn("fts search failed", "err", err)
			return
		}
		ftsResults = r
	}()
	wg.Wait()

	merged := mergeRRF(vecResults, ftsResults, RagTopK)
	var results []Result
	for _, mr := range merged {
		filePath := ""
		if mr.ChapterNumber > 0 {
			filePath = fmt.Sprintf("chapters/%03d.md", mr.ChapterNumber)
		}
		results = append(results, Result{
			Type:          "rag",
			Title:         fmt.Sprintf("第 %d 章", mr.ChapterNumber),
			Subtitle:      mr.Content,
			ChapterNum:    mr.ChapterNumber,
			FilePath:      filePath,
			MatchPosition: mr.StartRunePos,
			MatchLen:      len([]rune(mr.Content)),
			Relevance:     mr.Relevance,
		})
	}
	return results
}

// mergeRRF 用 Reciprocal Rank Fusion 合并向量与 FTS 两路结果，
// 按 (chapter_number, start_position) 去重（同一切块只保留最高分）。
// k 取 60（RRF 标准值），向量相关性（1-distance）作为并列时的排序依据。
func mergeRRF(vec, fts []rag.SearchResult, limit int) []rag.SearchResult {
	type scored struct {
		res   rag.SearchResult
		score float64
	}
	const k = 60.0
	best := map[[2]int]scored{} // (chapter, startPos) → 最高分结果

	rank := func(results []rag.SearchResult, weight float64) {
		for i, r := range results {
			key := [2]int{r.ChapterNumber, r.StartRunePos}
			s := weight / (k + float64(i))
			if cur, ok := best[key]; !ok || s > cur.score {
				best[key] = scored{res: r, score: s}
			}
		}
	}
	rank(vec, 1.0)
	rank(fts, 1.0)

	out := make([]rag.SearchResult, 0, len(best))
	for _, s := range best {
		out = append(out, s.res)
	}
	// 按 RRF 分数降序
	sort.Slice(out, func(i, j int) bool {
		return best[[2]int{out[i].ChapterNumber, out[i].StartRunePos}].score >
			best[[2]int{out[j].ChapterNumber, out[j].StartRunePos}].score
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
