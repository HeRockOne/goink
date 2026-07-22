package agent

import (
	"context"
	"errors"

	"novel/internal/llm"
)

// FriendlyError 将 LLM 错误转换为用户友好的消息。
// 原始 error 应由调用方另行记录日志。
func FriendlyError(err error) string {
	if errors.Is(err, context.Canceled) {
		return ""
	}
	var apiErr *llm.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 401:
			return "API Key 无效，请在设置中检查"
		case 403:
			return "API Key 无权限"
		case 429:
			return "" // 重试已由 EventRetry 悬浮通知处理，对话面板不再显示
		default:
			if apiErr.StatusCode >= 500 {
				return "AI 服务暂时不可用，请稍后重试"
			}
		}
	}
	return "对话出错，请重试"
}
