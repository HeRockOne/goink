package llm

// Providers 返回当前所有 provider 的只读引用。
func (c *Client) Providers() map[string]Provider {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.providers
}

// ProviderModel 在指定 provider 中按 model ID 查找模型元信息。
func (c *Client) ProviderModel(providerName, modelID string) (*ModelInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.providers[providerName]
	if !ok {
		return nil, false
	}
	for i := range p.Models {
		if p.Models[i].ID == modelID {
			return &p.Models[i], true
		}
	}
	return nil, false
}
