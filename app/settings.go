package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"novel/internal/config"
	"novel/internal/git"
	"novel/internal/novel"
	"novel/internal/rag"
)

// SaveSettingsInput 是保存设置的入参。
type SaveSettingsInput struct {
	// 后续加 LLM 配置字段（provider、模型选择、APIKey 等）
}

// ── 设置 ──────────────────────────────────────────────────

// GetSettings 返回运行时配置。
func (a *App) GetSettings() (*config.AppSettings, error) {
	return a.settings, nil
}

// GetServerInfo 返回服务器连接信息（IP + 端口）。
func (a *App) GetServerInfo() map[string]any {
	ip := getLocalIP()
	port := 0
	if a.ctx != nil {
		// 从 Wails 运行时获取端口
		// Wails v2 在启动时会打印端口，这里用默认值
		port = 9323
	}
	return map[string]any{
		"ip":   ip,
		"port": port,
		"url":  fmt.Sprintf("http://%s:%d", ip, port),
	}
}

// SaveSettings 保存运行时配置。
func (a *App) SaveSettings(input SaveSettingsInput) error {
	return config.SaveSettings(a.db, a.settings)
}

// SetSelectedModel 保存选中的模型 key 和推理程度，持久化到 DB。
func (a *App) SetSelectedModel(key, effort string) error {
	a.settings.SelectedModelKey = key
	a.settings.ReasoningEffort = effort
	return config.SaveSettings(a.db, a.settings)
}

// SetReasoningEffort 单独保存推理程度。
func (a *App) SetReasoningEffort(effort string) error {
	a.settings.ReasoningEffort = effort
	return config.SaveSettings(a.db, a.settings)
}

// SetLastSession 保存上次活跃的会话 ID。
func (a *App) SetLastSession(sessionID string) error {
	a.settings.LastSessionID = sessionID
	return config.SaveSettings(a.db, a.settings)
}

// GetAPIToken 获取 API 认证 token，不存在则自动生成。
func (a *App) GetAPIToken() string {
	if a.settings.APIToken == "" {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err == nil {
			a.settings.APIToken = hex.EncodeToString(b)
			config.SaveSettings(a.db, a.settings)
		}
	}
	return a.settings.APIToken
}

// ResetAPIToken 重新生成 API token。
func (a *App) ResetAPIToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err == nil {
		a.settings.APIToken = hex.EncodeToString(b)
		config.SaveSettings(a.db, a.settings)
	}
	return a.settings.APIToken
}

// SaveUserName 保存用户名称。
func (a *App) SaveUserName(name string) error {
	a.settings.UserName = name
	return config.SaveSettings(a.db, a.settings)
}

// SaveGitConfig 保存 Git user.name 和 user.email，并同步到所有已有仓库。
func (a *App) SaveGitConfig(name, email string) error {
	a.settings.GitName = name
	a.settings.GitEmail = email
	if err := config.SaveSettings(a.db, a.settings); err != nil {
		return err
	}
	result, err := a.novel.List(a.ctx, novel.ListNovelsOptions{})
	if err != nil {
		return fmt.Errorf("save git config: list novels: %w", err)
	}
	var errs []string
	for _, n := range result.Items {
		repo, repoErr := git.New(n.ID, name, email, a.logger)
		if repoErr != nil {
			errs = append(errs, fmt.Sprintf("小说 %d: %v", n.ID, repoErr))
			continue
		}
		if err := repo.SetGitConfig(name, email); err != nil {
			errs = append(errs, fmt.Sprintf("小说 %d: %v", n.ID, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("部分小说配置同步失败:\n%s", strings.Join(errs, "\n"))
	}
	return nil
}

// SaveAvatar 保存用户头像到数据目录。
func (a *App) SaveAvatar(data []byte) error {
	userDir := filepath.Join(config.DataDirPath(), "user")
	if err := os.MkdirAll(userDir, 0700); err != nil {
		return fmt.Errorf("save avatar: %w", err)
	}
	avatarPath := filepath.Join(userDir, "avatar.jpg")
	return os.WriteFile(avatarPath, data, 0644)
}

// RebuildNovelIndex 无条件全量重建指定小说的向量索引，用于数据异常时的手动兜底。
func (a *App) RebuildNovelIndex(novelID int64) error {
	rq := rag.GetRefreshQueue()
	if rq == nil {
		return fmt.Errorf("向量索引服务未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	return rq.RebuildNovel(ctx, novelID)
}

// SetCompressionThreshold 设置上下文压缩阈值（0.3-0.95）。
func (a *App) SetCompressionThreshold(threshold float64) error {
	if threshold < 0.3 {
		threshold = 0.3
	}
	if threshold > 0.95 {
		threshold = 0.95
	}
	a.settings.CompressionThreshold = threshold
	return config.SaveSettings(a.db, a.settings)
}

// SetPhaseGateEnabled 设置阶段门禁开关。
func (a *App) SetPhaseGateEnabled(enabled bool) error {
	a.settings.PhaseGateEnabled = &enabled
	return config.SaveSettings(a.db, a.settings)
}

// SetWebDAVConfig 设置 WebDAV 配置。
func (a *App) SetWebDAVConfig(port int, user, pass string) error {
	a.settings.WebDAVPort = port
	a.settings.WebDAVUser = user
	a.settings.WebDAVPass = pass
	return config.SaveSettings(a.db, a.settings)
}

// SetChapterWordLimit 设置章节字数校验范围。
func (a *App) SetChapterWordLimit(minWords, maxWords int) error {
	a.settings.MinChapterWords = minWords
	a.settings.MaxChapterWords = maxWords
	return config.SaveSettings(a.db, a.settings)
}

// SetAPIPort 设置移动端连接端口并重启 API 服务器。
func (a *App) SetAPIPort(port int) error {
	a.settings.APIPort = port
	if err := config.SaveSettings(a.db, a.settings); err != nil {
		return err
	}
	// 重启 API 服务器
	a.StopAPIServer()
	a.StartAPIServer()
	return nil
}

// StartAPIServer 启动独立 API 服务器。
func (a *App) StartAPIServer() {
	port := a.settings.APIPort
	if port <= 0 {
		port = 9323
	}
	a.restartAPIServer(port)
}

// StopAPIServer 停止 API 服务器。
func (a *App) StopAPIServer() {
	if a.apiServer != nil {
		a.apiServer.Stop()
		a.apiServer = nil
	}
}

// restartAPIServer 重启 API/WebSocket 服务器到指定端口。
func (a *App) restartAPIServer(port int) {
	a.apiServer = newAPIServer(port, a, a.logger, a.frontend, a.mobile)
	go a.apiServer.Start()
}

