package mcp_tools

import (
	"context"
	"encoding/json"
)

// WebSearchArgs 是 web_search 工具的参数。
type WebSearchArgs struct {
	Prompt string `json:"prompt" jsonschema:"required,description=发给搜索 AI 的指令，用自然语言描述你的问题和背景，这不是传统的搜索关键词，是具体的搜索指令" validate:"required,min=1,max=500"`
}

// WebSearchTool 通过 DeepSeek Anthropic 端点执行网络搜索，返回模型总结和来源链接。
type WebSearchTool struct{}

func (t *WebSearchTool) Name() string           { return "web_search" }
func (t *WebSearchTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *WebSearchTool) ExposeToLLM() bool      { return true }

func (t *WebSearchTool) Description() string {
	return "联网搜索真实信息（返回 {queries=搜索词列表, summary=综合答案, sources=[{title, url, snippet}]}），返回综合答案和参考来源（数量由搜索服务决定，一般数个到十几个）。适用于需要实时数据、新闻、技术文档或超出模型知识范围的内容。" +
		"搜索结果已由 AI 综合分析，可直接引用返回的 summary；sources 为来源 URL 列表。如需查看某个来源的原文细节，可用 web_fetch 抓取。" +
		"【注意】创作场景只在确需外部事实（真实地名/历史/专业知识）时使用，不要用搜索结果替代故事设定。"
}

func (t *WebSearchTool) JSONSchema() json.RawMessage {
	return SchemaOf(WebSearchArgs{})
}

func (t *WebSearchTool) NewArgs() any { return &WebSearchArgs{} }

func (t *WebSearchTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*WebSearchArgs)

	if tc.WebSearch == nil {
		return &ToolResult{Error: "网络搜索未启用：请先在 LLM 设置中配置 DeepSeek"}, nil
	}

	result, err := tc.WebSearch(ctx, a.Prompt)
	if err != nil {
		return &ToolResult{Error: "搜索失败: " + err.Error()}, nil
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"queries": result.Queries,
			"summary": result.Summary,
			"sources": result.Sources,
		},
	}, nil
}

// RegisterWebSearchTools 注册网络搜索工具。
func RegisterWebSearchTools(r *Registry) {
	r.Register(&WebSearchTool{})
}
