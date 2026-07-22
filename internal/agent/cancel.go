package agent

import (
	"context"
	"sync"
)

// 不同领域的取消键前缀，防止 key 冲突。
const (
	CancelPrefixChat    = "chat:"
	CancelPrefixStyle   = "style:"
	CancelPrefixPattern = "pattern:"
)

// CancelManager 管理可取消的操作，通过 key 注册和取消。
type CancelManager struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

// NewCancelManager 创建一个新的 CancelManager。
func NewCancelManager() *CancelManager {
	return &CancelManager{
		cancels: make(map[string]context.CancelFunc),
	}
}

// Register 注册一个可取消的操作。
func (m *CancelManager) Register(key string, cancel context.CancelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancels[key] = cancel
}

// Unregister 清理已注册的操作，不调用 cancel。
func (m *CancelManager) Unregister(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cancels, key)
}

// IsRegistered 检查指定 key 是否已注册。
func (m *CancelManager) IsRegistered(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.cancels[key]
	return ok
}

// Cancel 取消并清理指定 key 的操作。如果 key 不存在则无操作。
func (m *CancelManager) Cancel(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.cancels[key]; ok {
		c()
		delete(m.cancels, key)
	}
}
