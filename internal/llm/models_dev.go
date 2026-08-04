package llm

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ModelSpec 是模型预设参数。
type ModelSpec struct {
	ContextWindow    int
	MaxOutputTokens  int
	SupportsThinking bool
	SupportsVision   bool
	ReasoningLevels  []string
}

const (
	modelsDevURL      = "https://models.dev/api.json"
	modelsDevCacheTTL = 24 * time.Hour
)

type modelsDevProvider struct {
	ID     string                    `json:"id"`
	Name   string                    `json:"name"`
	Models map[string]modelsDevModel `json:"models"`
}

type modelsDevModel struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Reasoning        bool   `json:"reasoning"`
	Attachment       bool   `json:"attachment"`
	Limit            struct {
		Context int `json:"context"`
		Output  int `json:"output"`
	} `json:"limit"`
	Modalities struct {
		Input  []string `json:"input"`
		Output []string `json:"output"`
	} `json:"modalities"`
	ReasoningOptions []struct {
		Type   string   `json:"type"`
		Values []string `json:"values"`
	} `json:"reasoning_options"`
}

type modelsDevCache struct {
	FetchedAt time.Time                    `json:"fetched_at"`
	Providers map[string]modelsDevProvider `json:"providers"`
}

type ModelsDevClient struct {
	cacheDir string
	mu       sync.RWMutex
	cache    *modelsDevCache
}

var (
	globalModelsDev *ModelsDevClient
	modelsDevOnce   sync.Once
)

func GetModelsDevClient(cacheDir string) *ModelsDevClient {
	modelsDevOnce.Do(func() {
		globalModelsDev = &ModelsDevClient{cacheDir: cacheDir}
	})
	return globalModelsDev
}

func (c *ModelsDevClient) LookupModelSpec(modelID string) *ModelSpec {
	c.ensureCache()
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.cache == nil {
		return nil
	}
	for _, provider := range c.cache.Providers {
		for key, m := range provider.Models {
			if key == modelID || m.ID == modelID {
				return c.toSpec(m)
			}
		}
	}
	lowerID := strings.ToLower(modelID)
	for _, provider := range c.cache.Providers {
		for key, m := range provider.Models {
			if strings.Contains(strings.ToLower(key), lowerID) || strings.Contains(strings.ToLower(m.Name), lowerID) {
				return c.toSpec(m)
			}
		}
	}
	return nil
}

// LookupProviderModels 从 chat URL 推断服务商名，从 models.dev 查找该服务商的全部模型。
// 返回 ModelInfo 列表（含从 models.dev 获取的精确上下文窗口等参数）或 nil。
func (c *ModelsDevClient) LookupProviderModels(chatURL string) []ModelInfo {
	c.ensureCache()
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.cache == nil || chatURL == "" {
		return nil
	}

	// 从 URL 中提取候选服务商名（如 "api.stepfun.com" → "stepfun"）
	domain := extractDomain(chatURL)
	if domain == "" {
		return nil
	}
	candidates := []string{domain}
	// 去掉常见前缀：api. → ""；api.stepfun.com → stepfun
	if strings.HasPrefix(domain, "api.") {
		candidates = append(candidates, domain[4:])
	}
	// 去掉后缀：stepfun.com → stepfun
	if idx := strings.Index(domain, "."); idx > 0 {
		candidates = append(candidates, domain[:idx])
		// 再去掉 api. 前缀: api.stepfun → stepfun
		if strings.HasPrefix(domain[:idx], "api.") {
			candidates = append(candidates, domain[:idx][4:])
		}
	}

	lower := func(s string) string { return strings.ToLower(s) }
	for _, pname := range c.cache.Providers {
		matched := false
		for _, cand := range candidates {
			if cand == "" {
				continue
			}
			if lower(pname.Name) == lower(cand) || lower(pname.ID) == lower(cand) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		// 找到匹配的服务商，将其模型转换为 ModelInfo
		models := make([]ModelInfo, 0, len(pname.Models))
		for _, m := range pname.Models {
			mi := ModelInfo{
				ID:               m.ID,
				Name:             m.Name,
				ContextWindow:    m.Limit.Context,
				MaxOutputTokens:  m.Limit.Output,
				SupportsThinking: m.Reasoning,
				SupportsVision:   containsAny(m.Modalities.Input, "image", "video", "pdf"),
			}
			for _, ro := range m.ReasoningOptions {
				if ro.Type == "effort" && len(ro.Values) > 0 {
					mi.ReasoningLevels = append([]string{}, ro.Values...)
					break
				}
			}
			models = append(models, mi)
		}
		if len(models) > 0 {
			return models
		}
	}
	return nil
}

// extractDomain 从 URL 中提取域名部分（不含端口和路径）。
func extractDomain(rawURL string) string {
	// 先去掉协议
	s := rawURL
	if idx := strings.Index(s, "://"); idx >= 0 {
		s = s[idx+3:]
	}
	// 去掉路径和端口
	if idx := strings.Index(s, "/"); idx >= 0 {
		s = s[:idx]
	}
	if idx := strings.Index(s, ":"); idx >= 0 {
		s = s[:idx]
	}
	return s
}

func (c *ModelsDevClient) toSpec(m modelsDevModel) *ModelSpec {
	spec := &ModelSpec{
		ContextWindow:    m.Limit.Context,
		MaxOutputTokens:  m.Limit.Output,
		SupportsThinking: m.Reasoning,
		SupportsVision:   containsAny(m.Modalities.Input, "image", "video", "pdf"),
	}
	for _, ro := range m.ReasoningOptions {
		if ro.Type == "effort" && len(ro.Values) > 0 {
			spec.ReasoningLevels = append([]string{}, ro.Values...)
			break
		}
	}
	return spec
}

func containsAny(slice []string, items ...string) bool {
	for _, s := range slice {
		for _, item := range items {
			if s == item {
				return true
			}
		}
	}
	return false
}

func (c *ModelsDevClient) ensureCache() {
	c.mu.RLock()
	if c.cache != nil && time.Since(c.cache.FetchedAt) < modelsDevCacheTTL {
		c.mu.RUnlock()
		return
	}
	c.mu.RUnlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cache != nil && time.Since(c.cache.FetchedAt) < modelsDevCacheTTL {
		return
	}
	if c.loadFromDisk() {
		return
	}
	c.fetchFromNetwork()
}

func (c *ModelsDevClient) loadFromDisk() bool {
	cachePath := filepath.Join(c.cacheDir, "models.dev.cache.json")
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return false
	}
	var cache modelsDevCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return false
	}
	if time.Since(cache.FetchedAt) > modelsDevCacheTTL {
		return false
	}
	c.cache = &cache
	return true
}

func (c *ModelsDevClient) fetchFromNetwork() {
	client := newHTTPClient(30 * time.Second)
	resp, err := client.Get(modelsDevURL)
	if err != nil {
		fmt.Printf("[models.dev] 获取模型数据失败: %v\n", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		fmt.Printf("[models.dev] 请求失败: HTTP %d\n", resp.StatusCode)
		return
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("[models.dev] 读取响应失败: %v\n", err)
		return
	}
	var providers map[string]modelsDevProvider
	if err := json.Unmarshal(body, &providers); err != nil {
		fmt.Printf("[models.dev] 解析数据失败: %v\n", err)
		return
	}
	cache := &modelsDevCache{FetchedAt: time.Now(), Providers: providers}
	c.cache = cache
	c.saveToDisk(cache)
	modelCount := 0
	for _, p := range providers {
		modelCount += len(p.Models)
	}
	fmt.Printf("[models.dev] 模型数据更新完成: %d providers, %d models\n", len(providers), modelCount)
}

func (c *ModelsDevClient) saveToDisk(cache *modelsDevCache) {
	if err := os.MkdirAll(c.cacheDir, 0700); err != nil {
		return
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return
	}
	cachePath := filepath.Join(c.cacheDir, "models.dev.cache.json")
	os.WriteFile(cachePath, data, 0600)
}
