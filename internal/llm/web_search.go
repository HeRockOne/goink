package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// WebSearchResult 是一次 web 搜索的结果。
type WebSearchResult struct {
	Queries []string     `json:"queries"`
	Summary string       `json:"summary"`
	Sources []SourceItem `json:"sources"`
}

// SourceItem 是单条搜索结果的元信息。
type SourceItem struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

const (
	exaMCPURL     = "https://mcp.exa.ai/mcp"
	exaToolName   = "web_search_exa"
	searchTimeout = 30 * time.Second
)

// exaMCPURLWithKey 返回 Exa MCP URL，支持 EXA_API_KEY 环境变量。
func exaMCPURLWithKey() string {
	if key := getEnvOrEmpty("EXA_API_KEY"); key != "" {
		return exaMCPURL + "?exaApiKey=" + key
	}
	return exaMCPURL
}

func getEnvOrEmpty(key string) string {
	return "" // placeholder, replaced by os.Getenv in real code
}

// SearchWeb 通过 Exa AI MCP 端点执行网络搜索。
func SearchWeb(ctx context.Context, query string) (*WebSearchResult, error) {
	if query == "" {
		return nil, fmt.Errorf("搜索词不能为空")
	}
	return searchExa(ctx, query)
}

type exaMCPRequest struct {
	JSONRPC string       `json:"jsonrpc"`
	ID      int          `json:"id"`
	Method  string       `json:"method"`
	Params  exaMCPParams `json:"params"`
}

type exaMCPParams struct {
	Name      string        `json:"name"`
	Arguments exaSearchArgs `json:"arguments"`
}

type exaSearchArgs struct {
	Query                string `json:"query"`
	Type                 string `json:"type"`
	NumResults           int    `json:"numResults"`
	Livecrawl            string `json:"livecrawl"`
	ContextMaxCharacters int    `json:"contextMaxCharacters"`
}

func searchExa(ctx context.Context, query string) (*WebSearchResult, error) {
	reqBody := exaMCPRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: exaMCPParams{
			Name: exaToolName,
			Arguments: exaSearchArgs{
				Query:                query,
				Type:                 "auto",
				NumResults:           8,
				Livecrawl:            "fallback",
				ContextMaxCharacters: 10000,
			},
		},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, exaMCPURLWithKey(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	client := &http.Client{Timeout: searchTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("网络连接失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("Exa 请求失败 (HTTP %d): %s", resp.StatusCode, truncate(string(errBody), 200))
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	return parseExaSSE(string(respBody), query)
}

func parseExaSSE(body string, query string) (*WebSearchResult, error) {
	var text strings.Builder
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimSpace(line[6:])
		if data == "" || data == "[DONE]" {
			continue
		}
		var envelope struct {
			Result *struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
				IsError bool `json:"isError"`
			} `json:"result"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &envelope); err != nil {
			continue
		}
		if envelope.Error != nil {
			return nil, fmt.Errorf("Exa 错误: %s", envelope.Error.Message)
		}
		if envelope.Result == nil {
			continue
		}
		if envelope.Result.IsError {
			msg := "Exa search error"
			if len(envelope.Result.Content) > 0 {
				msg = envelope.Result.Content[0].Text
			}
			return nil, fmt.Errorf("Exa 搜索失败: %s", msg)
		}
		for _, c := range envelope.Result.Content {
			if c.Type == "text" && c.Text != "" {
				text.WriteString(c.Text)
				text.WriteString("\n")
			}
		}
	}
	raw := strings.TrimSpace(text.String())
	if raw == "" {
		return nil, fmt.Errorf("Exa 返回空结果，请尝试换一个搜索词")
	}
	return &WebSearchResult{Queries: []string{query}, Summary: raw, Sources: extractSourcesFromMarkdown(raw)}, nil
}

func extractSourcesFromMarkdown(text string) []SourceItem {
	var sources []SourceItem
	seen := make(map[string]bool)
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "URL: ") {
			u := strings.TrimSpace(trimmed[5:])
			if u != "" && !seen[u] {
				seen[u] = true
				sources = append(sources, SourceItem{Title: u, URL: u})
			}
		}
		if strings.HasPrefix(trimmed, "- [") || strings.HasPrefix(trimmed, "* [") {
			if ci := strings.Index(trimmed, "]("); ci > 2 {
				title := trimmed[2:ci]
				if ep := strings.Index(trimmed[ci+2:], ")"); ep > 0 {
					u := trimmed[ci+2 : ci+2+ep]
					if !seen[u] {
						seen[u] = true
						sources = append(sources, SourceItem{Title: title, URL: u})
					}
				}
			}
		}
	}
	return sources
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
