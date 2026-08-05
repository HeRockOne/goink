package app

import (
	"fmt"
	"os"
	"strings"
	"time"

	wails "github.com/wailsapp/wails/v2/pkg/runtime"

	"novel/internal/session"
)

// ExportSession 将指定会话导出为 Markdown 文件（用户消息 + AI 回复正文，不含工具调用/系统注入）。
// 弹出保存对话框，用户取消时返回空字符串。
func (a *App) ExportSession(sessionID string) (string, error) {
	var sess session.Session
	if err := a.db.Where("session_id = ?", sessionID).First(&sess).Error; err != nil {
		return "", fmt.Errorf("查询会话失败: %w", err)
	}

	var msgs []session.Message
	if err := a.db.Where("session_id = ? AND version = ? AND to_frontend = ? AND role IN ?",
		sessionID, sess.ActiveVersion, true, []string{"user", "assistant"}).
		Order("id ASC").Find(&msgs).Error; err != nil {
		return "", fmt.Errorf("读取会话消息失败: %w", err)
	}

	var b strings.Builder
	title := sess.Title
	if title == "" {
		title = "会话"
	}
	b.WriteString("# " + title + "\n\n")
	b.WriteString("> 导出时间：" + time.Now().Format("2006-01-02 15:04") + "  ·  会话 ID：`" + sessionID + "`\n\n---\n\n")

	for _, m := range msgs {
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		// 过滤系统注入（<system-reminder> 包裹的提示）与 markdown 标题行
		content = strings.ReplaceAll(content, "<system-reminder>", "> [系统] ")
		content = strings.ReplaceAll(content, "</system-reminder>", "")
		if m.Role == "user" {
			b.WriteString("## 🧑 你\n\n")
		} else {
			b.WriteString("## 🤖 AI\n\n")
		}
		b.WriteString(content + "\n\n---\n\n")
	}

	savePath, err := wails.SaveFileDialog(a.ctx, wails.SaveDialogOptions{
		DefaultFilename:      title + "-" + time.Now().Format("20060102-1504") + ".md",
		Title:                "导出会话",
		Filters:              []wails.FileFilter{{DisplayName: "Markdown 文件 (*.md)", Pattern: "*.md"}},
		CanCreateDirectories: true,
	})
	if err != nil {
		return "", fmt.Errorf("打开保存对话框失败: %w", err)
	}
	if savePath == "" {
		return "", nil // 用户取消
	}
	if err := os.WriteFile(savePath, []byte(b.String()), 0644); err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}
	return savePath, nil
}
