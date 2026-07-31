package llm

// deepseekBuildRequest 适配 DeepSeek 与标准 OpenAI 格式的差异（官方 api-docs.deepseek.com）：
// - reasoning_effort 归一化：low/medium → high，xhigh → max（DeepSeek V4 只支持 high/max）
// - thinking.type=disabled 时必须移除 reasoning_effort（官方规定两者不能同时发送）
// - thinking.type=enabled 时保留 reasoning_effort（官方 V4 原生参数，无需转换）
func deepseekBuildRequest(payload map[string]any) map[string]any {
	reasoningEffort, hasEffort := payload["reasoning_effort"].(string)
	if !hasEffort || reasoningEffort == "" {
		return payload
	}

	thinkingDisabled := false
	if thinking, ok := payload["thinking"].(map[string]string); ok {
		thinkingDisabled = thinking["type"] == "disabled"
	}

	switch {
	case thinkingDisabled:
		delete(payload, "reasoning_effort")
	case reasoningEffort == "low" || reasoningEffort == "medium":
		payload["reasoning_effort"] = "high"
	case reasoningEffort == "xhigh":
		payload["reasoning_effort"] = "max"
	}

	return payload
}
