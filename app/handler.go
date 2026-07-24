package app

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"gorm.io/gorm"

	"novel/internal/agent"
	"novel/internal/approval"
	"novel/internal/logger"
	"novel/internal/chapter"
	"novel/internal/character"
	"novel/internal/config"
	"novel/internal/llm"
	"novel/internal/location"
	"novel/internal/mcp_tools"
	"novel/internal/migrate"
	"novel/internal/novel"
	"novel/internal/rag"
	"novel/internal/reader"
	"novel/internal/rollback"
	"novel/internal/search"
	"novel/internal/session"
	"novel/internal/skill"
	"novel/internal/style"
	"novel/internal/storage"
	"novel/internal/storyarc"
	"novel/internal/timeline"
	"novel/internal/writing"
	webdavpkg "novel/internal/webdav"
	ws "novel/internal/ws"
)

// App 是 Wails 绑定的根对象。前端通过 window.go.main.App 调用其导出方法。
// 各领域方法按文件拆分（novel.go / chapter.go 等），均接收 *App。
type App struct {
	ctx    context.Context
	cancel context.CancelFunc
	logger *slog.Logger

	cfg      *config.AppConfig
	settings *config.AppSettings
	db       *gorm.DB
	frontend *embed.FS
	mobile   *embed.FS

	llmClient     *llm.Client
	agent         *agent.Agent
	cancelMgr     *agent.CancelManager
	registry      *mcp_tools.Registry
	approvals     *approval.Service
	vectorStore   *rag.VectorStore
	searchService atomic.Pointer[search.Service]

	novel      *novel.Store
	chapter    *chapter.Store
	character  *character.Store
	session    *session.Store
	skill      *skill.Store
	style      *style.Store
	timeline   *timeline.Store
	storyarc   *storyarc.Store
	location   *location.Store
	reader     *reader.Store
	turnCommit *rollback.Store
	writing    *writing.Store
	webdav     *webdavpkg.Server
	apiServer  *apiServer
	wsHub      *ws.Hub
	logEnabled bool // 文件日志开关
}

// New 创建 App 实例。初始化在 OnStartup 中完成。
func New(logger *slog.Logger, frontend *embed.FS, mobile *embed.FS) *App {
	return &App{logger: logger, frontend: frontend, mobile: mobile}
}

// ── 生命周期 ──────────────────────────────────────────────

// Logger 返回日志器。
func (a *App) Logger() *slog.Logger {
	return a.logger
}

// SetLoggingEnabled 启用/禁用文件日志，并持久化到 SQLite。
func (a *App) SetLoggingEnabled(enabled bool) {
	a.logEnabled = enabled
	logger.SetFileEnabled(enabled)
	if a.settings != nil {
		a.settings.LogEnabled = enabled
		config.SaveSettings(a.db, a.settings)
	}
}

// GetLoggingEnabled 返回文件日志是否启用。
func (a *App) GetLoggingEnabled() bool {
	return a.logEnabled
}

// SetWSHub 设置 WebSocket Hub，用于双端实时同步。
func (a *App) SetWSHub(hub *ws.Hub) {
	a.wsHub = hub
}

// BroadcastChatEvent 广播对话事件到所有 WebSocket 客户端（移动端）。
func (a *App) BroadcastChatEvent(eventType string, data map[string]any) {
	if a.wsHub == nil {
		return
	}
	ev := map[string]any{"type": eventType, "channel": "chat"}
	for k, v := range data {
		ev[k] = v
	}
	if raw, err := json.Marshal(ev); err == nil {
		a.wsHub.Broadcast(raw)
	}
	// 更新流式状态
	sessionID, _ := data["session_id"].(string)
	d, _ := data["data"].(string)
	switch eventType {
	case "started":
		a.wsHub.UpdateStreamState(sessionID, "", "", true)
	case "thinking":
		a.wsHub.AppendThinking(d)
	case "content":
		a.wsHub.AppendContent(d)
	case "done", "error":
		a.wsHub.UpdateStreamState("", "", "", false)
	}
}
func (a *App) OnStartup(ctx context.Context) {
	a.ctx, a.cancel = context.WithCancel(ctx)

	cfg, err := config.Load()
	if err != nil {
		a.logger.Error("加载配置失败", "err", err)
		return
	}
	a.initWithConfig(cfg)

	// 恢复窗口大小和位置
	a.restoreWindowState()
}

// OnShutdown 在 Wails 窗口关闭前调用，释放资源。
func (a *App) OnShutdown(shutdownCtx context.Context) {
	a.logger.Info("应用关闭，释放资源")

	// 保存窗口大小和位置
	a.saveWindowState()

	// 1. 取消根上下文，通知所有运行中的 agent 停止
	if a.cancel != nil {
		a.cancel()
	}

	// 2. 停止 RAG 后台消费者
	if q := rag.GetRefreshQueue(); q != nil {
		q.Stop()
	}
	// 3. 释放 ONNX embedder（非阻塞，避免未初始化时死锁）
	if emb := rag.TryGetEmbedder(); emb != nil {
		_ = emb.Close()
	}

	// 4. 关闭数据库（放在最后，确保上述清理中的 DB 操作已完成）
	if a.db != nil {
		if err := storage.Close(a.db); err != nil {
			a.logger.Error("关闭数据库失败", "err", err)
		}
	}
}

// IsInitialized 返回指针文件是否已加载成功。前端据此决定显示初始化界面还是主界面。
func (a *App) IsInitialized() bool {
	return a.cfg != nil
}

// Initialize 在用户触发首次初始化时调用。
// dataDir 参数保留用于前端兼容，实际数据目录由平台决定。
func (a *App) Initialize(dataDir string) error {
	if err := config.Save(dataDir); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	a.initWithConfig(cfg)
	return nil
}

// initWithConfig 在配置加载成功后初始化所有运行时模块。
// 只有全部步骤成功才会将 a.cfg 设为非 nil，防止半初始化状态下 IsInitialized() 误报。
func (a *App) initWithConfig(cfg *config.AppConfig) {
	config.Set(cfg)

	// 0. 初始化 models.dev 模型数据库（异步获取，不阻塞）
	modelsDevCacheDir := filepath.Join(config.DataDirPath(), "cache")
	llm.GetModelsDevClient(modelsDevCacheDir)

	// 1. 异步加载 ONNX 模型（不阻塞 GUI，尽早调用）
	rag.InitEmbedder(config.ModelsDir(), a.logger)

	// 2. 打开全局数据库
	db, err := storage.Open(config.GlobalDBPath(), a.logger)
	if err != nil {
		a.logger.Error("打开数据库失败", "err", err)
		return
	}
	a.db = db

	// 3. 自动建表
	if err := migrate.Run(db, a.logger); err != nil {
		a.logger.Error("数据库迁移失败", "err", err)
		return
	}

	// 4. 加载运行时配置
	settings, err := config.LoadSettings(db)
	if err != nil {
		a.logger.Error("加载设置失败", "err", err)
		return
	}
	a.settings = settings
	a.logEnabled = settings.LogEnabled
	logger.SetFileEnabled(settings.LogEnabled)

	// 5. 注册操作日志钩子
	storage.RegisterOplogHooks(db)

	// 6. 创建所有领域 store
	a.novel = novel.NewStore(db, a.logger)
	a.chapter = chapter.NewStore(db, a.logger)
	a.character = character.NewStore(db, a.logger)
	a.session = session.NewStore(db, a.logger)
	a.timeline = timeline.NewStore(db, a.logger)
	a.storyarc = storyarc.NewStore(db, a.logger)
	a.location = location.NewStore(db, a.logger)
	a.reader = reader.NewStore(db, a.logger)
	a.turnCommit = rollback.NewStore(db, a.logger)
	a.writing = writing.NewStore(db, a.logger)
	s, err := skill.NewStore(a.logger, config.UserSkillsDir())
	if err != nil {
		a.logger.Error("初始化 skill store 失败", "err", err)
	} else {
		a.skill = s
	}

	// 7. 初始化 MCP 工具注册表
	a.registry = mcp_tools.NewRegistry(a.logger)
	mcp_tools.RegisterAllTools(a.registry)

	// 8. 初始化 LLM 客户端
	userConfig, err := llm.LoadUserConfig(config.LLMConfigPath())
	if err != nil {
		a.logger.Warn("加载 LLM 配置失败，使用空配置", "err", err)
		userConfig = &llm.UserLLMConfig{}
	}
	providers := llm.Merge(llm.Builtin, userConfig)
	a.llmClient = llm.NewClient(providers, a.logger)

	// 9. 初始化审批服务
	a.approvals = approval.NewService(a.logger, a.settings.ApprovalMode)

	// 10. 创建 Agent 实例（全局复用）
	a.cancelMgr = agent.NewCancelManager()
	a.agent = agent.New(a.llmClient, a.registry, a.session, a.db, a.approvals, a.logger, a.skill, a.cancelMgr)

	// 10.5 初始化 style store（全局风格素材）
	a.style = style.NewStore(db, a.logger)

	// 11. 异步初始化向量存储和搜索服务（不阻塞 UI）
	go func() {
		emb, err := rag.GetEmbedder()
		svc := search.NewService(a.logger, a.character, a.location,
			a.timeline, a.storyarc, a.chapter, nil)
		a.searchService.Store(svc)
		a.agent.SetSearchService(svc)
		if err != nil {
			a.logger.Error("获取 Embedder 失败，向量检索不可用", "err", err)
			return
		}
		sqlDB, err := a.db.DB()
		if err != nil {
			a.logger.Error("获取底层 SQL DB 失败，向量检索不可用", "err", err)
			return
		}
		rag.InitVectorStore(sqlDB, emb, a.logger)
		a.vectorStore = rag.GetVectorStore()
		a.logger.Info("向量存储初始化完成")

		// 初始化搜索服务
		svc = search.NewService(a.logger, a.character, a.location,
			a.timeline, a.storyarc, a.chapter, a.vectorStore)
		a.searchService.Store(svc)
		a.agent.SetSearchService(svc)

		// 初始化刷新队列并启动
		rag.InitRefreshQueue(a.vectorStore, a.chapter, a.novel, a.logger)
		rag.GetRefreshQueue().Start()

		// 首次启动全量索引（已有向量则跳过）
		rebuildCtx := context.Background()
		if err := rag.GetRefreshQueue().RebuildAll(rebuildCtx); err != nil {
			a.logger.Error("全量向量索引失败", "err", err)
		}
	}()

	a.cfg = cfg
	a.logger.Info("应用初始化完成", "data_dir", config.DataDirPath())
}

// splitModelKey 将 "provider/model" 格式的 key 拆分为 [provider, model]。
func splitModelKey(key string) (string, string) {
	parts := strings.SplitN(key, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", key
}

// ── WebDAV 服务器 ──────────────────────────────────────────

// StartWebDAV 启动局域网 WebDAV 服务器（只读模式）
func (a *App) StartWebDAV(port int) error {
	if a.webdav != nil && a.webdav.IsRunning() {
		return fmt.Errorf("WebDAV 已在运行")
	}

	user := "1"
	pass := "1"
	p := port
	if a.settings != nil {
		if a.settings.WebDAVUser != "" { user = a.settings.WebDAVUser }
		if a.settings.WebDAVPass != "" { pass = a.settings.WebDAVPass }
		if p == 0 && a.settings.WebDAVPort > 0 { p = a.settings.WebDAVPort }
	}
	if p == 0 { p = 12345 }

	// 自动导出所有小说到 outputs 目录
	a.ExportAllToOutputs()

	// 指向导出目录
	rootDir := filepath.Join(config.DataDirPath(), "outputs")
	a.webdav = webdavpkg.New(rootDir, p, user, pass, a.logger)
	return a.webdav.Start()
}

// StopWebDAV 停止 WebDAV 服务器
func (a *App) StopWebDAV() error {
	if a.webdav == nil {
		return nil
	}
	return a.webdav.Stop()
}

// IsWebDAVRunning WebDAV 是否运行中
func (a *App) IsWebDAVRunning() bool {
	if a.webdav == nil {
		return false
	}
	return a.webdav.IsRunning()
}

// GetWebDAVInfo 获取 WebDAV 连接信息
func (a *App) GetWebDAVInfo() string {
	if a.webdav == nil {
		return ""
	}
	return a.webdav.GetURL()
}

// saveWindowState 保存窗口大小和位置到 JSON 文件
func (a *App) saveWindowState() {
	if a.ctx == nil {
		return
	}
	width, height := wailsRuntime.WindowGetSize(a.ctx)
	x, y := wailsRuntime.WindowGetPosition(a.ctx)
	if width <= 0 || height <= 0 {
		return
	}

	state := fmt.Sprintf(`{"width":%d,"height":%d,"x":%d,"y":%d}`, width, height, x, y)
	path := filepath.Join(config.DataDirPath(), "window-state.json")
	os.WriteFile(path, []byte(state), 0644)
}

// restoreWindowState 从 JSON 文件恢复窗口大小和位置
func (a *App) restoreWindowState() {
	if a.ctx == nil {
		return
	}
	path := filepath.Join(config.DataDirPath(), "window-state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var state struct {
		Width  int `json:"width"`
		Height int `json:"height"`
		X      int `json:"x"`
		Y      int `json:"y"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return
	}

	if state.Width > 0 && state.Height > 0 {
		wailsRuntime.WindowSetSize(a.ctx, state.Width, state.Height)
	}
	if state.X != 0 || state.Y != 0 {
		wailsRuntime.WindowSetPosition(a.ctx, state.X, state.Y)
	}
}
