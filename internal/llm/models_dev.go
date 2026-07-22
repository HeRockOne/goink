package llm

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	client := &http.Client{Timeout: 30 * time.Second}
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
