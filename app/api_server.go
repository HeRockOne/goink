package app

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	wails "github.com/wailsapp/wails/v2/pkg/runtime"

	"novel/internal/cert"
	"novel/internal/chapter"
	"novel/internal/character"
	"novel/internal/config"
	"novel/internal/netutil"
	"novel/internal/platform"
	"novel/internal/git"
	"novel/internal/item"
	"novel/internal/itemoccurrence"
	"novel/internal/llm"
	"novel/internal/location"
	"novel/internal/lore"
	"novel/internal/novel"
	"novel/internal/reader"
	"novel/internal/scene"
	"novel/internal/session"
	"novel/internal/stats"
	"novel/internal/storage"
	"novel/internal/storyarc"
	"novel/internal/timeline"
	"novel/internal/writing"
)

// generateToken 生成随机 API token。
func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

type apiServer struct {
	port     int
	useHTTPS bool
	app      *App
	logger   *slog.Logger
	server   *http.Server
	mux      *http.ServeMux
	mu       sync.Mutex
	running  bool
	frontend *embed.FS
	mobile   *embed.FS
}

func newAPIServer(port int, useHTTPS bool, app *App, logger *slog.Logger, frontend *embed.FS, mobile *embed.FS) *apiServer {
	s := &apiServer{port: port, useHTTPS: useHTTPS, app: app, logger: logger, frontend: frontend, mobile: mobile}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/api/info", s.handleServerInfo)
	s.mux.HandleFunc("/api/sync/state", s.handleSyncState)
	s.mux.HandleFunc("/api/novels", s.handleNovels)
	s.mux.HandleFunc("/api/novels/", s.handleNovelChapters)
	s.mux.HandleFunc("/api/chapters/", s.handleChapterContent)
	s.mux.HandleFunc("/api/characters", s.handleCharacters)
	s.mux.HandleFunc("/api/timeline", s.handleTimeline)
	s.mux.HandleFunc("/api/arcs", s.handleArcs)
	s.mux.HandleFunc("/api/reader", s.handleReader)
	s.mux.HandleFunc("/api/preferences", s.handlePreferences)
	s.mux.HandleFunc("/api/locations", s.handleLocations)
	s.mux.HandleFunc("/api/lore", s.handleLore)
	s.mux.HandleFunc("/api/items", s.handleItems)
	s.mux.HandleFunc("/api/read", s.handleReadFile)
	s.mux.HandleFunc("/api/scenes", s.handleScenes)
	s.mux.HandleFunc("/api/stats", s.handleStats)
	s.mux.HandleFunc("/api/writing-snapshot", s.handleWritingSnapshot)
	s.mux.HandleFunc("/api/item-occurrences", s.handleItemOccurrences)
	s.mux.HandleFunc("/api/character-relations", s.handleCharacterRelations)
	s.mux.HandleFunc("/api/location-relations", s.handleLocationRelations)
	s.mux.HandleFunc("/api/phase-gate-config", s.handlePhaseGateConfig)
	s.mux.HandleFunc("/api/writing-context", s.handleWritingContextAPI)
	s.mux.HandleFunc("/api/search-memory", s.handleSearchMemory)
	s.mux.HandleFunc("/api/chat", s.handleChat)
	s.mux.HandleFunc("/api/sessions", s.handleSessions)
	s.mux.HandleFunc("/api/sessions/", s.handleSessionMessages)
	s.mux.HandleFunc("/api/arc-nodes", s.handleArcNodes)
	s.mux.HandleFunc("/api/chat/cancel", s.handleChatCancel)
	s.mux.HandleFunc("/api/settings/model", s.handleModelSettings) // 模型切换
	s.mux.HandleFunc("/api/settings/pricing", s.handlePricing)   // 价格设置
	s.mux.HandleFunc("/api/settings/phase-gate-enabled", s.handlePhaseGateEnabled) // 门禁开关
	s.mux.HandleFunc("/api/settings/approval-mode", s.handleApprovalMode)           // 审批模式
	s.mux.HandleFunc("/api/settings/reasoning-effort", s.handleReasoningEffort)     // 思考深度
	s.mux.HandleFunc("/api/ws", s.handleWebSocket)                                   // 双端同步 WebSocket
	// 前端静态文件
	if s.frontend != nil {
		sub, err := fs.Sub(s.frontend, "frontend/dist")
		if err == nil {
			s.mux.Handle("/", http.FileServer(http.FS(sub)))
			s.logger.Info("前端静态文件已挂载")
		}
	}
	// 移动端 Web 前端
	if s.mobile != nil {
		sub, err := fs.Sub(s.mobile, "mobile")
		if err == nil {
			s.mux.Handle("/mobile/", http.StripPrefix("/mobile/", http.FileServer(http.FS(sub))))
			s.logger.Info("移动端 Web 前端已挂载", "path", "/mobile/")
		}
	}
	return s
}

func (s *apiServer) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	// 杀掉占用端口的旧进程
	killPort(s.port)

	addr := fmt.Sprintf(":%d", s.port)
	s.server = &http.Server{Addr: addr, Handler: withCORS(withAuth(s.mux, s))}

	if !s.useHTTPS {
		s.logger.Info("移动端 API 服务器启动 (HTTP)", "port", s.port)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Warn("API 服务器已停止", "err", err)
		}
		return
	}

	// 尝试生成/加载 HTTPS 证书
	dataDir := config.DataDirPath()
	certFile, keyFile, ip, err := cert.EnsureCert(dataDir)
	if err != nil {
		s.logger.Warn("HTTPS 证书生成失败，使用 HTTP", "err", err)
		s.logger.Info("移动端 API 服务器启动 (HTTP)", "port", s.port)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Warn("API 服务器已停止", "err", err)
		}
		return
	}

	s.logger.Info("移动端 API 服务器启动 (HTTPS)", "port", s.port, "ip", ip)
	s.logger.Info("📱 iPhone 用户首次访问需信任证书：设置 → 通用 → 关于本机 → 证书信任设置")
	s.logger.Info("访问地址", "url", fmt.Sprintf("https://%s:%d/mobile/", ip, s.port))

	if err := s.server.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
		s.logger.Warn("API 服务器已停止", "err", err)
	}
}

// killPort 杀掉占用指定端口的进程（仅 Windows）。
func killPort(port int) {
	cmd := exec.Command("netstat", "-ano", "-p", "tcp")
	platform.SetPlatformAttr(cmd)
	out, err := cmd.Output()
	if err != nil {
		return
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if !strings.Contains(line, fmt.Sprintf(":%d", port)) {
			continue
		}
		if !strings.Contains(line, "LISTENING") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		pid := fields[len(fields)-1]
		if pid == "0" || pid == "4" {
			continue
		}
		slog.Info("杀掉旧进程", "pid", pid, "port", port)
		kill := exec.Command("taskkill", "/F", "/PID", pid)
		platform.SetPlatformAttr(kill)
		kill.Run()
	}
}

// handleSyncState 返回当前桌面端流式会话状态，供移动端中途加入时查询。
func (s *apiServer) handleSyncState(w http.ResponseWriter, r *http.Request) {
	if s.app.wsHub == nil {
		writeJSON(w, map[string]any{"active": false})
		return
	}

	active, sessionID, thinking, content := s.app.wsHub.GetSyncState()
	if !active {
		writeJSON(w, map[string]any{"active": false})
		return
	}

	writeJSON(w, map[string]any{
		"active":     true,
		"session_id": sessionID,
		"thinking":   thinking,
		"content":    content,
	})
}

// ensureAPIToken 确保 API token 存在，不存在则自动生成。
func (s *apiServer) ensureAPIToken() string {
	if s.app.settings == nil {
		return ""
	}
	if s.app.settings.APIToken == "" {
		s.app.settings.APIToken = generateToken()
		config.SaveSettings(s.app.db, s.app.settings)
		s.logger.Info("已生成 API token", "token", s.app.settings.APIToken)
	}
	return s.app.settings.APIToken
}

func (s *apiServer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.running = false
	if s.server != nil {
		s.server.Shutdown(context.Background())
	}
}

func (s *apiServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"status": "ok"})
}

func (s *apiServer) handleServerInfo(w http.ResponseWriter, r *http.Request) {
	ip := getLocalIP()
	writeJSON(w, map[string]any{"ip": ip, "port": s.port, "url": fmt.Sprintf("http://%s:%d", ip, s.port)})
}

func (s *apiServer) handleNovels(w http.ResponseWriter, r *http.Request) {
	if s.app.db == nil {
		writeJSON(w, map[string]any{"novels": []any{}})
		return
	}
	switch r.Method {
	case "GET":
		ns := novel.NewStore(s.app.db, s.logger)
		result, err := ns.List(r.Context(), novel.ListNovelsOptions{})
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, map[string]any{"novels": result.Items, "total": result.Total})
	case "POST":
		s.handleCreateNovel(w, r)
	case "DELETE":
		s.handleDeleteNovel(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *apiServer) handleDeleteNovel(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, map[string]any{"error": "无效的请求体"})
		return
	}
	if input.ID == 0 {
		writeJSON(w, map[string]any{"error": "小说ID不能为空"})
		return
	}
	// 验证标题匹配
	var n novel.Novel
	if err := s.app.db.Where("id = ?", input.ID).First(&n).Error; err != nil {
		writeJSON(w, map[string]any{"error": "小说不存在"})
		return
	}
	if input.Title != n.Title {
		writeJSON(w, map[string]any{"error": "标题不匹配"})
		return
	}
	if err := s.app.DeleteNovel(input.ID); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *apiServer) handleCreateNovel(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Title       string `json:"title"`
		Genre       string `json:"genre"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, map[string]any{"error": "无效的请求体"})
		return
	}
	if input.Title == "" {
		writeJSON(w, map[string]any{"error": "小说名称不能为空"})
		return
	}
	n, err := s.app.CreateNovel(CreateNovelInput{
		Title:       input.Title,
		Genre:       input.Genre,
		Description: input.Description,
	})
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"novel": n})
}

func (s *apiServer) handleNovelChapters(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/novels/")
	parts := strings.SplitN(path, "/chapters", 2)
	if len(parts) < 2 || s.app.db == nil {
		writeJSON(w, map[string]any{"chapters": []any{}})
		return
	}
	novelID, _ := strconv.ParseInt(parts[0], 10, 64)
	cs := chapter.NewStore(s.app.db, s.logger)
	result, err := cs.ListByNovel(r.Context(), novelID, chapter.ListByNovelOptions{
		Order:      "desc",
		PageParams: storage.PageParams{Page: 1, Size: 9999},
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"chapters": result.Items, "total": result.Total})
}

func (s *apiServer) handleChapterContent(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/chapters/")
	chapterID, _ := strconv.ParseInt(path, 10, 64)
	if chapterID == 0 || s.app.db == nil {
		writeJSON(w, map[string]any{"error": "invalid chapter id"})
		return
	}
	var ch chapter.Chapter
	if err := s.app.db.Where("id = ?", chapterID).First(&ch).Error; err != nil {
		writeError(w, err)
		return
	}
	filePath := filepath.Join(config.DataDirPath(), "novels",
		strconv.FormatInt(ch.NovelID, 10), git.ChapterPath(ch.ChapterNumber))
	content := ""
	data, err := os.ReadFile(filePath)
	if err == nil {
		content = string(data)
	}
	writeJSON(w, map[string]any{
		"id": ch.ID, "chapter_number": ch.ChapterNumber, "title": ch.Title,
		"word_count": ch.WordCount, "content": content, "file_path": filePath,
	})
}

func (s *apiServer) handleCharacters(w http.ResponseWriter, r *http.Request) {
	novelID := parseIntQuery(r, "novel_id")
	if novelID == 0 || s.app.db == nil {
		writeJSON(w, map[string]any{"characters": []any{}})
		return
	}
	cs := character.NewStore(s.app.db, s.logger)
	result, err := cs.ListByNovel(r.Context(), novelID, character.ListByNovelOptions{})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"characters": result.Items, "total": result.Total})
}

func (s *apiServer) handleTimeline(w http.ResponseWriter, r *http.Request) {
	novelID := parseIntQuery(r, "novel_id")
	if novelID == 0 || s.app.db == nil {
		writeJSON(w, map[string]any{"entries": []any{}})
		return
	}
	ts := timeline.NewStore(s.app.db, s.logger)
	entries, err := ts.ListByNovel(r.Context(), novelID, timeline.ListByNovelOptions{
		PageParams: storage.PageParams{Page: 1, Size: 9999},
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"entries": entries})
}

func (s *apiServer) handleArcs(w http.ResponseWriter, r *http.Request) {
	novelID := parseIntQuery(r, "novel_id")
	if novelID == 0 || s.app.db == nil {
		writeJSON(w, map[string]any{"arcs": []any{}})
		return
	}
	as := storyarc.NewStore(s.app.db, s.logger)
	arcs, err := as.ListByNovel(r.Context(), novelID, storyarc.ListByNovelOptions{
		PageParams: storage.PageParams{Page: 1, Size: 9999},
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"arcs": arcs})
}

func (s *apiServer) handleReader(w http.ResponseWriter, r *http.Request) {
	novelID := parseIntQuery(r, "novel_id")
	if novelID == 0 || s.app.db == nil {
		writeJSON(w, map[string]any{"entries": []any{}})
		return
	}
	rs := reader.NewStore(s.app.db, s.logger)
	entries, err := rs.ListByNovel(r.Context(), novelID, reader.ListByNovelOptions{
		PageParams: storage.PageParams{Page: 1, Size: 9999},
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"entries": entries})
}

func (s *apiServer) handlePreferences(w http.ResponseWriter, r *http.Request) {
	novelID := parseIntQuery(r, "novel_id")
	if novelID == 0 || s.app.db == nil {
		writeJSON(w, map[string]any{"preferences": []any{}})
		return
	}
	ns := novel.NewStore(s.app.db, s.logger)
	items, err := ns.ListPreferences(r.Context(), novelID)
	if err != nil {
		writeError(w, err)
		return
	}
	// 转为 map 格式
	prefs := make([]map[string]any, 0, len(items))
	for _, item := range items {
		prefs = append(prefs, map[string]any{
			"id":        item.ID,
			"novel_id":  item.NovelID,
			"is_global": item.IsGlobal,
			"category":  item.Category,
			"content":   item.Content,
		})
	}
	writeJSON(w, map[string]any{"preferences": prefs})
}

func (s *apiServer) handleLocations(w http.ResponseWriter, r *http.Request) {
	novelID := parseIntQuery(r, "novel_id")
	if novelID == 0 || s.app.db == nil {
		writeJSON(w, map[string]any{"locations": []any{}})
		return
	}
	ls := location.NewStore(s.app.db, s.logger)
	locations, err := ls.ListByNovel(r.Context(), novelID, location.ListByNovelOptions{})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"locations": locations})
}

func (s *apiServer) handleLore(w http.ResponseWriter, r *http.Request) {
	novelID := parseIntQuery(r, "novel_id")
	if novelID == 0 || s.app.db == nil {
		writeJSON(w, map[string]any{"lore": []any{}, "total": 0})
		return
	}
	ls := lore.NewStore(s.app.db, s.logger)
	opts := lore.ListOptions{
		Page:     int(parseIntQuery(r, "page")),
		Size:     int(parseIntQuery(r, "size")),
		Category: r.URL.Query().Get("category"),
		Search:   r.URL.Query().Get("search"),
	}
	if opts.Page <= 0 {
		opts.Page = 1
	}
	if opts.Size <= 0 {
		opts.Size = 9999
	}
	result, err := ls.ListByNovel(r.Context(), novelID, opts)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"lore": result.Items, "total": result.Total})
}

func (s *apiServer) handleReadFile(w http.ResponseWriter, r *http.Request) {
	novelID := parseIntQuery(r, "novel_id")
	path := r.URL.Query().Get("path")
	if path == "" {
		writeJSON(w, map[string]any{"error": "path required"})
		return
	}
	content, err := git.ReadFile(novelID, path)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"content": content, "path": path})
}
func (s *apiServer) handleItems(w http.ResponseWriter, r *http.Request) {
	novelID := parseIntQuery(r, "novel_id")
	if novelID == 0 || s.app.db == nil {
		writeJSON(w, map[string]any{"items": []any{}, "total": 0})
		return
	}
	is := item.NewStore(s.app.db, s.logger)
	opts := item.ListOptions{
		Page:     int(parseIntQuery(r, "page")),
		Size:     int(parseIntQuery(r, "size")),
		ItemType: r.URL.Query().Get("type"),
		Status:   r.URL.Query().Get("status"),
		Search:   r.URL.Query().Get("search"),
	}
	if opts.Page <= 0 {
		opts.Page = 1
	}
	if opts.Size <= 0 {
		opts.Size = 9999
	}
	result, err := is.ListByNovel(r.Context(), novelID, opts)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": result.Items, "total": result.Total})
}

func (s *apiServer) handleScenes(w http.ResponseWriter, r *http.Request) {
	novelID := parseIntQuery(r, "novel_id")
	chapterID := parseIntQuery(r, "chapter_id")
	if novelID == 0 || s.app.db == nil {
		writeJSON(w, map[string]any{"scenes": []any{}})
		return
	}
	ss := scene.NewStore(s.app.db, s.logger)
	scenes, err := ss.ListByChapter(r.Context(), novelID, chapterID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"scenes": scenes})
}

func (s *apiServer) handleStats(w http.ResponseWriter, r *http.Request) {
	novelID := parseIntQuery(r, "novel_id")
	if novelID == 0 || s.app.db == nil {
		writeJSON(w, map[string]any{"error": "invalid novel_id"})
		return
	}
	ss := stats.NewStore(s.app.db, s.logger)
	result, err := ss.GetNovelStats(r.Context(), novelID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"stats": result})
}

func (s *apiServer) handleWritingSnapshot(w http.ResponseWriter, r *http.Request) {
	novelID := parseIntQuery(r, "novel_id")
	if novelID == 0 || s.app.db == nil {
		writeJSON(w, map[string]any{"error": "invalid novel_id"})
		return
	}
	ws := writing.NewSnapshotStore(s.app.db, s.logger)
	result, err := ws.Get(r.Context(), novelID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"writing_snapshot": result})
}

func (s *apiServer) handleItemOccurrences(w http.ResponseWriter, r *http.Request) {
	novelID := parseIntQuery(r, "novel_id")
	itemID := parseIntQuery(r, "item_id")
	if novelID == 0 || s.app.db == nil {
		writeJSON(w, map[string]any{"occurrences": []any{}})
		return
	}
	store := itemoccurrence.NewStore(s.app.db, s.logger)
	if itemID > 0 {
		items, err := store.ListAllByItem(r.Context(), novelID, itemID)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, map[string]any{"occurrences": items})
	} else {
		result, err := store.ListByNovel(r.Context(), novelID, storage.PageParams{Page: 1, Size: 9999})
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, map[string]any{"occurrences": result.Items, "total": result.Total})
	}
}

func (s *apiServer) handleCharacterRelations(w http.ResponseWriter, r *http.Request) {
	novelID := parseIntQuery(r, "novel_id")
	if novelID == 0 || s.app.db == nil {
		writeJSON(w, map[string]any{"relations": []any{}})
		return
	}
	cs := character.NewStore(s.app.db, s.logger)
	rels, err := cs.ListCurrentByNovel(r.Context(), novelID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"relations": rels})
}

func (s *apiServer) handleLocationRelations(w http.ResponseWriter, r *http.Request) {
	novelID := parseIntQuery(r, "novel_id")
	if novelID == 0 || s.app.db == nil {
		writeJSON(w, map[string]any{"relations": []any{}})
		return
	}
	ls := location.NewStore(s.app.db, s.logger)
	rels, err := ls.ListRelationsByNovel(r.Context(), novelID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"relations": rels})
}

// ── get_writing_context 辅助函数 ─────────────────────────

func (s *apiServer) getChapterBrief(ctx context.Context, novelID int64, chapNum int) map[string]any {
	r := map[string]any{"num": chapNum, "title": "", "id": 0}
	var ch chapter.Chapter
	if err := s.app.db.WithContext(ctx).Where("novel_id = ? AND chapter_number = ?", novelID, chapNum).First(&ch).Error; err == nil {
		r["id"] = ch.ID
		r["title"] = ch.Title
		r["word_count"] = ch.WordCount
	}
	return r
}
func (s *apiServer) getRecentChapters(ctx context.Context, novelID int64) []map[string]any {
	cs := chapter.NewStore(s.app.db, s.logger)
	chs, _ := cs.ListByNovel(ctx, novelID, chapter.ListByNovelOptions{Order: "desc", PageParams: storage.PageParams{Page: 1, Size: 5}})
	var out []map[string]any
	if chs != nil {
		for _, ch := range chs.Items {
			out = append(out, map[string]any{"num": ch.ChapterNumber, "title": ch.Title, "summary": ch.Summary, "key_events": ch.KeyEvents, "word_cnt": ch.WordCount})
		}
	}
	return out
}
func (s *apiServer) getCharacterBriefs(ctx context.Context, novelID int64) []map[string]any {
	var chars []character.Character
	s.app.db.WithContext(ctx).Where("novel_id = ?", novelID).Find(&chars)
	var out []map[string]any
	for _, ch := range chars {
		loc := map[string]any{"id": 0, "name": ""}
		if ch.LocationID != nil {
			var l location.Location
			if s.app.db.WithContext(ctx).First(&l, *ch.LocationID).Error == nil {
				loc = map[string]any{"id": l.ID, "name": l.Name}
			}
		}
		var ic int64
		s.app.db.WithContext(ctx).Model(&item.Item{}).Where("owner_id = ? AND novel_id = ?", ch.ID, novelID).Count(&ic)
		out = append(out, map[string]any{"id": ch.ID, "name": ch.Name, "location": loc, "item_count": ic})
	}
	return out
}
func (s *apiServer) getActiveArcs(ctx context.Context, novelID int64) []map[string]any {
	var arcs []storyarc.StoryArc
	s.app.db.WithContext(ctx).Where("novel_id = ? AND status = 'active'", novelID).Find(&arcs)
	var out []map[string]any
	for _, a := range arcs {
		t, d := int64(0), int64(0)
		s.app.db.WithContext(ctx).Model(&storyarc.ArcNode{}).Where("story_arc_id = ?", a.ID).Count(&t)
		s.app.db.WithContext(ctx).Model(&storyarc.ArcNode{}).Where("story_arc_id = ? AND status = 'completed'", a.ID).Count(&d)
		out = append(out, map[string]any{"id": a.ID, "name": a.Name, "type_zh": arcTypeZh(a.ArcType), "nodes_total": t, "nodes_done": d})
	}
	return out
}
func (s *apiServer) getTimelineBrief(ctx context.Context, novelID int64, chapNum int) map[string]any {
	ts := timeline.NewStore(s.app.db, s.logger)
	// Size 对齐桌面版 GetWritingContext（100）：20 会被 resolved 占满导致 pending 全部丢失
	entries, _ := ts.ListByNovel(ctx, novelID, timeline.ListByNovelOptions{PageParams: storage.PageParams{Page: 1, Size: 100}})
	pending, resolved, overdue := make([]map[string]any, 0), make([]map[string]any, 0), make([]map[string]any, 0)
	if entries != nil {
		for _, e := range entries.Items {
			item := map[string]any{"id": e.ID, "title": e.Title, "status": e.Status, "target_chapter": e.TargetChapter, "importance": e.Importance, "resolved_chapter": e.ResolvedChapterID}
			if e.Status == "resolved" {
				resolved = append(resolved, item)
			} else {
				pending = append(pending, item)
				if chapNum > 0 && e.TargetChapter > 0 && e.TargetChapter < chapNum {
					overdue = append(overdue, map[string]any{"id": e.ID, "title": e.Title, "target_chapter": e.TargetChapter, "importance": e.Importance, "overdue_by": chapNum - e.TargetChapter})
				}
			}
		}
	}
	return map[string]any{"pending": pending, "resolved": resolved, "overdue": overdue}
}
func (s *apiServer) getReaderCounts(ctx context.Context, novelID int64) map[string]int {
	var k int64
	s.app.db.WithContext(ctx).Model(&reader.ReaderPerspective{}).Where("novel_id = ? AND type = 'known'", novelID).Count(&k)
	var sCount, mCount int
	var active []reader.ReaderPerspective
	s.app.db.WithContext(ctx).Where("novel_id = ? AND (revealed_chapter = 0 OR revealed_chapter IS NULL)", novelID).Find(&active)
	for _, e := range active {
		if e.Type == "suspense" {
			sCount++
		}
		if e.Type == "misconception" {
			mCount++
		}
	}
	return map[string]int{"known": int(k), "suspense": sCount, "misconception": mCount}
}
func (s *apiServer) getSnap(ctx context.Context, novelID int64) map[string]any {
	ws := writing.NewSnapshotStore(s.app.db, s.logger)
	snap, _ := ws.Get(ctx, novelID)
	if snap == nil {
		return nil
	}
	return map[string]any{"last_chapter_num": snap.LastChapterNum, "current_arc_id": snap.CurrentArcID, "current_location": snap.CurrentLocation, "active_chars": snap.ActiveChars}
}
func (s *apiServer) getNovelStats(ctx context.Context, novelID int64) map[string]any {
	var tc int64
	s.app.db.WithContext(ctx).Model(&chapter.Chapter{}).Where("novel_id = ?", novelID).Count(&tc)
	snap, _ := writing.NewSnapshotStore(s.app.db, s.logger).Get(ctx, novelID)
	st := map[string]any{"total_chapters": tc}
	if snap != nil {
		st["last_chapter_num"] = snap.LastChapterNum
	}
	return st
}

func arcTypeZh(t string) string {
	switch t {
	case "main":
		return "主线"
	case "sub":
		return "支线"
	case "character":
		return "角色弧"
	case "background":
		return "背景"
	case "volume":
		return "卷纲"
	default:
		return t
	}
}

func (s *apiServer) handlePhaseGateConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadSettings(s.app.db)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"config": cfg.PhaseGateConfig, "enabled": cfg.PhaseGateEnabled != nil && *cfg.PhaseGateEnabled})
}

func (s *apiServer) handleWritingContextAPI(w http.ResponseWriter, r *http.Request) {
	novelID := parseIntQuery(r, "novel_id")
	chapNum := int(parseIntQuery(r, "current_chapter"))
	if novelID == 0 {
		writeJSON(w, map[string]any{"error": "novel_id required"})
		return
	}
	// 用已有 API 端点各自获取数据，客户端自己拼
	r1 := s.getChapterBrief(r.Context(), novelID, chapNum)
	r2 := s.getRecentChapters(r.Context(), novelID)
	r3 := s.getCharacterBriefs(r.Context(), novelID)
	r4 := s.getActiveArcs(r.Context(), novelID)
	r5 := s.getTimelineBrief(r.Context(), novelID, chapNum)
	r6 := s.getReaderCounts(r.Context(), novelID)
	r7 := s.getSnap(r.Context(), novelID)
	r8 := s.getNovelStats(r.Context(), novelID)
	writeJSON(w, map[string]any{"chapter": r1, "recent_chapters": r2, "characters": r3, "active_arcs": r4, "timeline": r5, "reader": r6, "writing_snapshot": r7, "stats": r8})
}

func (s *apiServer) handleSearchMemory(w http.ResponseWriter, r *http.Request) {
	novelID := parseIntQuery(r, "novel_id")
	query := r.URL.Query().Get("query")
	svc := s.app.searchService.Load()
	if novelID == 0 || query == "" || svc == nil {
		writeJSON(w, map[string]any{"results": []any{}})
		return
	}
	results, err := svc.SearchAll(r.Context(), novelID, query)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"results": results})
}
func (s *apiServer) handleSessions(w http.ResponseWriter, r *http.Request) {
	novelID := parseIntQuery(r, "novel_id")
	if novelID == 0 || s.app.session == nil {
		writeJSON(w, map[string]any{"items": []any{}, "total": 0})
		return
	}
	page := int(parseIntQuery(r, "page"))
	size := int(parseIntQuery(r, "size"))
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 50
	}
	result, err := s.app.session.ListSessions(r.Context(), novelID, session.ListSessionsOptions{
		PageParams: storage.PageParams{Page: page, Size: size},
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": result.Items, "total": result.Total})
}

func (s *apiServer) handleSessionMessages(w http.ResponseWriter, r *http.Request) {
	// /api/sessions/{session_id}/messages
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 {
		writeJSON(w, map[string]any{"error": "invalid path"})
		return
	}
	sessionID := parts[3]
	if s.app.session == nil {
		writeJSON(w, map[string]any{"messages": []any{}})
		return
	}
	msgs, err := s.app.session.GetAllMessages(r.Context(), sessionID)
	if err != nil {
		writeError(w, err)
		return
	}
	// 过滤掉系统消息和工具消息，只返回用户和助手消息
	result := make([]map[string]any, 0)
	for _, m := range msgs {
		if m.Role == "user" || m.Role == "assistant" {
			result = append(result, map[string]any{
				"role":             m.Role,
				"content":          m.Content,
				"thinking_content": m.ThinkingContent,
				"created_at":       m.CreatedAt,
			})
		}
	}
	writeJSON(w, map[string]any{"messages": result})
}

func (s *apiServer) handleArcNodes(w http.ResponseWriter, r *http.Request) {
	novelID := parseIntQuery(r, "novel_id")
	if novelID == 0 || s.app.db == nil {
		writeJSON(w, map[string]any{"nodes": []any{}})
		return
	}
	as := storyarc.NewStore(s.app.db, s.logger)
	nodes, err := as.ListNodesByChapterRange(r.Context(), novelID, 0, 9999)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"nodes": nodes, "total": len(nodes)})
}

func (s *apiServer) handleChatCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err)
		return
	}
	if req.SessionID == "" {
		writeJSON(w, map[string]any{"error": "missing session_id"})
		return
	}
	s.app.agent.Cancel(req.SessionID)
	writeJSON(w, map[string]any{"ok": true})
}

func (s *apiServer) handleChat(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			s.logger.Error("Chat panic recovered", "err", rec)
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "data: {\"type\":\"error\",\"error\":\"服务器内部错误\"}\n\n")
		}
	}()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Message   string `json:"message"`
		NovelID   int64  `json:"novel_id"`
		Model     string `json:"model"`
		Provider  string `json:"provider"`
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err)
		return
	}
	if req.NovelID == 0 || req.Message == "" {
		writeJSON(w, map[string]any{"error": "missing novel_id or message"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	events := make(chan map[string]any, 64)
	// 不 close events：客户端断开由 ctx.Done 驱动退出，chatImpl 侧可能仍在写入，
	// close 后 send 会 panic；channel 随 goroutine 结束被 GC 回收。
	go func() {
		s.app.ChatFromAPI(r.Context(), req.Message, req.NovelID, req.Model, req.Provider, events, req.SessionID)
	}()

	ctx := r.Context()
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			jsonBytes, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", string(jsonBytes))
			flusher.Flush()
		case <-ctx.Done():
			return
		}
	}
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
}

func parseIntQuery(r *http.Request, key string) int64 {
	v := r.URL.Query().Get(key)
	if v == "" {
		return 0
	}
	id, _ := strconv.ParseInt(v, 10, 64)
	return id
}

func getLocalIP() string {
	return netutil.GetLocalIP()
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// withAuth API 认证中间件。豁免：health、静态文件、token 获取接口。
func withAuth(next http.Handler, s *apiServer) http.Handler {
	// 不需要认证的路径前缀
	exemptPaths := []string{"/api/health", "/mobile/", "/assets/", "/wails/"}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// 豁免检查（仅静态文件）
		for _, ep := range exemptPaths {
			if strings.HasPrefix(path, ep) || path == "/" {
				next.ServeHTTP(w, r)
				return
			}
		}
		// 验证 token
		token := s.ensureAPIToken()
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		qToken := r.URL.Query().Get("token")
		if strings.HasPrefix(auth, "Bearer ") {
			auth = strings.TrimPrefix(auth, "Bearer ")
		}
		if auth != token && qToken != token {
			writeJSON(w, map[string]any{"error": "unauthorized"})
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleWebSocket 处理移动端 WebSocket 连接，用于桌面端对话实时同步到移动端。
func (s *apiServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if s.app.wsHub == nil {
		http.Error(w, "WebSocket not available", http.StatusServiceUnavailable)
		return
	}
	s.app.wsHub.HandleWS(s.logger)(w, r)
}

// handleModelSettings 处理模型设置的 GET/POST 请求。
// GET  返回当前模型和可用模型列表
// POST 切换模型并同步到桌面端
func (s *apiServer) handleModelSettings(w http.ResponseWriter, r *http.Request) {
	if s.app.settings == nil {
		writeError(w, fmt.Errorf("settings not loaded"))
		return
	}

	switch r.Method {
	case http.MethodGet:
		// 获取当前模型 + 可用模型列表
		models := []any{}
		if s.app.llmClient != nil {
			for _, m := range llm.Models(s.app.llmClient.Providers()) {
				models = append(models, map[string]any{
					"key":      m.Key,
					"name":     m.ModelName,
					"provider": m.ProviderName,
					"thinking": m.SupportsThinking,
				})
			}
		}
		writeJSON(w, map[string]any{
			"selected_model_key": s.app.settings.SelectedModelKey,
			"reasoning_effort":   s.app.settings.ReasoningEffort,
			"models":             models,
		})

	case http.MethodPost:
		var req struct {
			ModelKey        string `json:"model_key"`
			ReasoningEffort string `json:"reasoning_effort"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.ModelKey == "" {
			writeError(w, fmt.Errorf("model_key is required"))
			return
		}
		// 保存设置
		s.app.settings.SelectedModelKey = req.ModelKey
		s.app.settings.ReasoningEffort = req.ReasoningEffort
		if err := config.SaveSettings(s.app.db, s.app.settings); err != nil {
			writeError(w, err)
			return
		}
		// 通过 WebSocket 广播模型变更到移动端
		s.app.BroadcastChatEvent("model_changed", map[string]any{
			"model_key":        req.ModelKey,
			"reasoning_effort": req.ReasoningEffort,
		})
		// 同时通过 Wails 事件通知桌面端前端刷新模型选择
		if s.app.ctx != nil {
			wails.EventsEmit(s.app.ctx, "model:changed", map[string]any{
				"selected_model_key": req.ModelKey,
				"reasoning_effort":   req.ReasoningEffort,
			})
		}
		writeJSON(w, map[string]any{"ok": true, "model_key": req.ModelKey})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handlePricing 处理价格设置的 GET 请求。
func (s *apiServer) handlePricing(w http.ResponseWriter, r *http.Request) {
	if s.app.settings == nil {
		writeError(w, fmt.Errorf("settings not loaded"))
		return
	}

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, map[string]any{
		"price_input":  s.app.settings.PriceInput,
		"price_output": s.app.settings.PriceOutput,
		"cache_price":  s.app.settings.CachePrice,
	})
}

// handlePhaseGateEnabled 处理门禁开关的 GET/POST 请求。
func (s *apiServer) handlePhaseGateEnabled(w http.ResponseWriter, r *http.Request) {
	if s.app.settings == nil {
		writeError(w, fmt.Errorf("settings not loaded"))
		return
	}

	switch r.Method {
	case http.MethodGet:
		enabled := s.app.settings.PhaseGateEnabled == nil || *s.app.settings.PhaseGateEnabled
		writeJSON(w, map[string]any{"enabled": enabled})

	case http.MethodPost:
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if err := s.app.SetPhaseGateEnabled(req.Enabled); err != nil {
			writeError(w, err)
			return
		}
		// 广播状态变更
		s.app.BroadcastChatEvent("phase_gate_enabled_changed", map[string]any{
			"enabled": req.Enabled,
		})
		if s.app.ctx != nil {
			wails.EventsEmit(s.app.ctx, "settings:phase_gate_enabled_changed", map[string]any{
				"enabled": req.Enabled,
			})
		}
		writeJSON(w, map[string]any{"ok": true, "enabled": req.Enabled})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleApprovalMode 处理审批模式的 GET/POST 请求。
func (s *apiServer) handleApprovalMode(w http.ResponseWriter, r *http.Request) {
	if s.app.approvals == nil {
		writeError(w, fmt.Errorf("approval service not available"))
		return
	}

	switch r.Method {
	case http.MethodGet:
		mode := s.app.approvals.GetMode()
		writeJSON(w, map[string]any{"mode": mode})

	case http.MethodPost:
		var req struct {
			Mode string `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Mode != "auto" && req.Mode != "manual" {
			writeError(w, fmt.Errorf("mode must be 'auto' or 'manual'"))
			return
		}
		s.app.approvals.SetMode(req.Mode)
		s.app.settings.ApprovalMode = req.Mode
		if err := config.SaveSettings(s.app.db, s.app.settings); err != nil {
			writeError(w, err)
			return
		}
		// 广播状态变更
		s.app.BroadcastChatEvent("approval_mode_changed", map[string]any{
			"mode": req.Mode,
		})
		if s.app.ctx != nil {
			wails.EventsEmit(s.app.ctx, "settings:approval_mode_changed", map[string]any{
				"mode": req.Mode,
			})
		}
		writeJSON(w, map[string]any{"ok": true, "mode": req.Mode})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleReasoningEffort 处理思考深度的 POST 请求。
func (s *apiServer) handleReasoningEffort(w http.ResponseWriter, r *http.Request) {
	if s.app.settings == nil {
		writeError(w, fmt.Errorf("settings not loaded"))
		return
	}

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ReasoningEffort string `json:"reasoning_effort"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Errorf("invalid request body: %w", err))
		return
	}
	// 验证值
	validEfforts := map[string]bool{"": true, "low": true, "medium": true, "high": true, "max": true}
	if !validEfforts[req.ReasoningEffort] {
		writeError(w, fmt.Errorf("reasoning_effort must be one of: low, medium, high, max, or empty"))
		return
	}
	s.app.settings.ReasoningEffort = req.ReasoningEffort
	if err := config.SaveSettings(s.app.db, s.app.settings); err != nil {
		writeError(w, err)
		return
	}
	// 广播状态变更
	s.app.BroadcastChatEvent("reasoning_effort_changed", map[string]any{
		"reasoning_effort": req.ReasoningEffort,
	})
	if s.app.ctx != nil {
		wails.EventsEmit(s.app.ctx, "settings:reasoning_effort_changed", map[string]any{
			"reasoning_effort": req.ReasoningEffort,
		})
	}
	writeJSON(w, map[string]any{"ok": true, "reasoning_effort": req.ReasoningEffort})
}
