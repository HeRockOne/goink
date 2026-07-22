package main

import (
	"context"
	"database/sql"
	"embed"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	ort "github.com/yalue/onnxruntime_go"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"novel/app"
	"novel/internal/config"
	"novel/internal/logger"
	"novel/internal/platform"
	"novel/internal/ws"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed all:mobile
var mobileAssets embed.FS

// readLogEnabledFromDB 在创建 logger 前从 DB 读取 log_enabled 开关值。
// 避免启动阶段的日志（如 ONNX 库检测）写入文件。
func readLogEnabledFromDB() {
	dbPath := config.GlobalDBPath()
	if dbPath == "" {
		return
	}
	if _, err := os.Stat(dbPath); err != nil {
		return // DB 文件不存在，保持默认 true
	}
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return
	}
	defer db.Close()
	var enabled bool
	if err := db.QueryRow("SELECT log_enabled FROM app_config WHERE id = 1").Scan(&enabled); err != nil {
		return
	}
	logger.SetFileEnabled(enabled)
}

func main() {
	// 在创建 logger 前从 DB 读取 log_enabled，避免启动日志也写入文件
	readLogEnabledFromDB()

	log := logger.Default()

	if lib, err := platform.ResolveOnnxLib(); err == nil {
		ort.SetSharedLibraryPath(lib)
		log.Info("ONNX 运行库已设置", "path", lib)
	} else {
		log.Warn("未找到 ONNX Runtime 库，向量检索将不可用", "err", err)
	}

	wapp := app.New(log, &assets, &mobileAssets)

	// WebSocket Hub
	wsHub := ws.NewHub(log)
	go wsHub.Run()
	wapp.SetWSHub(wsHub)

	err := wails.Run(&options.App{
		Title:     "Goink",
		Width:     1200,
		Height:    750,
		MinWidth:  900,
		MinHeight: 600,
		Frameless: runtime.GOOS != "darwin",
		AssetServer: &assetserver.Options{
			Assets: assets,
			Middleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// WebSocket
					if r.URL.Path == "/ws" {
						ws.HandleWS(wsHub, nil, log).ServeHTTP(w, r)
						return
					}
					// 封面
					if idStr, ok := strings.CutPrefix(r.URL.Path, "/covers/"); ok {
						novelID, _ := strconv.ParseInt(idStr, 10, 64)
						if novelID > 0 {
							p := filepath.Join(config.DataDirPath(), "novels", strconv.FormatInt(novelID, 10), "cover.jpg")
							http.ServeFile(w, r, p)
							return
						}
					}
					// 头像
					if r.URL.Path == "/avatar" {
						p := filepath.Join(config.DataDirPath(), "user", "avatar.jpg")
						http.ServeFile(w, r, p)
						return
					}
					next.ServeHTTP(w, r)
				})
			},
		},
		OnStartup: func(ctx context.Context) {
			wapp.OnStartup(ctx)
			// 启动独立 API 服务器（可自定义端口）
			wapp.StartAPIServer()
		},
		OnShutdown: func(ctx context.Context) {
			wapp.StopAPIServer()
			wapp.OnShutdown(ctx)
		},
		Bind: []any{wapp},
	})
	if err != nil {
		slog.Error("应用退出", "err", err)
		os.Exit(1)
	}
}
