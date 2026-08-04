package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows/registry"
)

// initWindowsProxy 读取 Windows 系统代理设置并写入环境变量 HTTP_PROXY/HTTPS_PROXY。
// Go 的 http.ProxyFromEnvironment 只读环境变量，不读 Windows 注册表。
// 此函数确保 Go 的 HTTP 客户端能走系统代理（如 clash/v2ray 等）。
func initWindowsProxy() {
	if runtime.GOOS != "windows" {
		return
	}
	if os.Getenv("HTTP_PROXY") != "" || os.Getenv("HTTPS_PROXY") != "" {
		return
	}
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		registry.QUERY_VALUE)
	if err != nil {
		return
	}
	defer k.Close()

	enabled, _, err := k.GetIntegerValue("ProxyEnable")
	if err != nil || enabled == 0 {
		return
	}
	proxy, _, err := k.GetStringValue("ProxyServer")
	if err != nil || proxy == "" {
		return
	}
	if !strings.HasPrefix(proxy, "http://") && !strings.HasPrefix(proxy, "https://") {
		proxy = "http://" + proxy
	}
	os.Setenv("HTTP_PROXY", proxy)
	os.Setenv("HTTPS_PROXY", proxy)

	override, _, err := k.GetStringValue("ProxyOverride")
	if err == nil && strings.Contains(override, "<local>") {
		os.Setenv("NO_PROXY", "localhost,127.0.0.1,.local")
	}
}

var initWindowsProxyOnce sync.Once

func ensureWindowsProxy() {
	if runtime.GOOS == "windows" {
		initWindowsProxyOnce.Do(initWindowsProxy)
	}
}

// newHTTPClient 创建带超时和代理的 HTTP 客户端。
func newHTTPClient(timeout time.Duration) *http.Client {
	ensureWindowsProxy()
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}
}

// DiscoverModels 调用 /models 端点自动发现可用模型列表。
// 优先从服务商自己的 /models 端点获取；失败时从 models.dev 回退匹配。
func DiscoverModels(ctx context.Context, chatURL, apiKey string) ([]ModelInfo, error) {
	chatURL = normalizeURL(chatURL)
	baseURL := strings.TrimSuffix(chatURL, "/chat/completions")
	modelsURL := baseURL + "/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		// 请求构造失败，直接回退到 models.dev
		if fallback := tryModelsDevFallback(chatURL); fallback != nil {
			return fallback, nil
		}
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := newHTTPClient(15 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		// 网络错误（超时/代理等），回退到 models.dev
		if fallback := tryModelsDevFallback(chatURL); fallback != nil {
			return fallback, nil
		}
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		// 服务商不支持 /models，回退到 models.dev
		if fallback := tryModelsDevFallback(chatURL); fallback != nil {
			return fallback, nil
		}
		return nil, httpError(resp.StatusCode, errBody)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if fallback := tryModelsDevFallback(chatURL); fallback != nil {
			return fallback, nil
		}
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	if looksLikeHTML(body) {
		if fallback := tryModelsDevFallback(chatURL); fallback != nil {
			return fallback, nil
		}
		return nil, fmt.Errorf("该端点不支持自动发现（服务端返回了网页而非 JSON）")
	}

	var result struct {
		Data []struct {
			ID                string `json:"id"`
			ContextLength     int    `json:"context_length"`
			SupportsImageIn   *bool  `json:"supports_image_in"`
			SupportsVideoIn   *bool  `json:"supports_video_in"`
			SupportsReasoning *bool  `json:"supports_reasoning"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		if fallback := tryModelsDevFallback(chatURL); fallback != nil {
			return fallback, nil
		}
		return nil, fmt.Errorf("解析模型列表失败（该端点可能不支持 /models）: %w", err)
	}

	models := make([]ModelInfo, 0, len(result.Data))
	for _, item := range result.Data {
		if item.ID == "" {
			continue
		}
		m := ModelInfo{
			ID:   item.ID,
			Name: modelIDToName(item.ID),
		}
		if item.ContextLength > 0 {
			m.ContextWindow = item.ContextLength
		}
		if item.SupportsReasoning != nil {
			m.SupportsThinking = *item.SupportsReasoning
		}
		if item.SupportsImageIn != nil {
			m.SupportsVision = *item.SupportsImageIn
		} else if item.SupportsVideoIn != nil && *item.SupportsVideoIn {
			m.SupportsVision = true
		}

		// 从 models.dev 补充缺失字段
		if m.ContextWindow == 0 || m.MaxOutputTokens == 0 {
			if globalModelsDev != nil {
				if spec := globalModelsDev.LookupModelSpec(item.ID); spec != nil {
					if m.ContextWindow == 0 {
						m.ContextWindow = spec.ContextWindow
					}
					if m.MaxOutputTokens == 0 {
						m.MaxOutputTokens = spec.MaxOutputTokens
					}
					if !m.SupportsThinking && spec.SupportsThinking {
						m.SupportsThinking = spec.SupportsThinking
					}
					if !m.SupportsVision && spec.SupportsVision {
						m.SupportsVision = spec.SupportsVision
					}
					if len(m.ReasoningLevels) == 0 && len(spec.ReasoningLevels) > 0 {
						m.ReasoningLevels = append([]string{}, spec.ReasoningLevels...)
					}
				}
			}
		}

		if m.ContextWindow == 0 {
			m.ContextWindow = 128_000
		}
		if m.MaxOutputTokens == 0 {
			m.MaxOutputTokens = 16_384
		}

		models = append(models, m)
	}

	if len(models) == 0 {
		if fallback := tryModelsDevFallback(chatURL); fallback != nil {
			return fallback, nil
		}
	}

	return models, nil
}

// tryModelsDevFallback 从 models.dev 查找服务商模型列表作为回退。
func tryModelsDevFallback(chatURL string) []ModelInfo {
	if globalModelsDev == nil {
		return nil
	}
	return globalModelsDev.LookupProviderModels(chatURL)
}

func looksLikeHTML(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return false
	}
	return trimmed[0] != '{' && trimmed[0] != '['
}

func httpError(code int, body []byte) error {
	switch code {
	case 401:
		return fmt.Errorf("API Key 无效或未配置 (401)")
	case 403:
		if looksLikeHTML(body) {
			return fmt.Errorf("服务端拒绝访问，可能被防火墙拦截，该端点不支持自动发现 (403)")
		}
		return fmt.Errorf("无权访问该端点 (403)")
	case 404:
		return fmt.Errorf("该端点不支持 /models 自动发现 (404)")
	case 429:
		return fmt.Errorf("请求过于频繁，请稍后重试 (429)")
	default:
		msg := string(body)
		if looksLikeHTML(body) {
			msg = "服务端返回了网页，该端点可能不支持自动发现"
		}
		return fmt.Errorf("[%d] %s", code, msg)
	}
}

// modelIDToName 将模型 ID 转为显示名称：首字母大写，- 替换为空格。
func modelIDToName(id string) string {
	s := strings.ReplaceAll(id, "-", " ")
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
