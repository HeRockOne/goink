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
	done    map[string]chan struct{} // 关闭信号：UnregisterCancel 时 close+delete
}

// NewCancelManager 创建一个新的 CancelManager。
func NewCancelManager() *CancelManager {
	return &CancelManager{
		cancels: make(map[string]context.CancelFunc),
		done:    make(map[string]chan struct{}),
	}
}

// Register 注册一个可取消的操作。
func (m *CancelManager) Register(key string, cancel context.CancelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancels[key] = cancel
}

// RegisterShutdown 注册会话结束信号通道，供 WaitForStop 阻塞等待。
// 必须在 Register 之后调用。取消或完成后，Unregister 会关闭并删除该通道。
func (m *CancelManager) RegisterShutdown(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.done[key] = make(chan struct{})
}

// Unregister 清理已注册的操作，不调用 cancel。
// 如果 done 通道存在则关闭并删除（标记 run loop 已退出）。
func (m *CancelManager) Unregister(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cancels, key)
	if ch, ok := m.done[key]; ok {
		close(ch)
		delete(m.done, key)
	}
}

// WaitForStop 阻塞直到会话 run loop 退出（Unregister 被调用）或超时。
// 返回 true 表示已停止，false 表示超时仍在运行。
// Cancel() 会立即删除 cancels 条目但保留 done 通道，loop 退出时才关闭并删除 done。
func (m *CancelManager) WaitForStop(key string, timeout context.Context) bool {
	m.mu.Lock()
	ch, ok := m.done[key]
	m.mu.Unlock()
	if !ok {
		return true // 通道不存在 = 已经结束或从未注册过
	}
	select {
	case <-ch:
		return true
	case <-timeout.Done():
		return false
	}
}

// IsRegistered 检查指定 key 是否已注册。
func (m *CancelManager) IsRegistered(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.cancels[key]
	return ok
}

// Cancel 取消并清理指定 key 的操作。如果 key 不存在则无操作。
// 取消后 done 通道保留——直到 Unregister 关闭它，表示 run loop 实际退出。
func (m *CancelManager) Cancel(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.cancels[key]; ok {
		c()
		delete(m.cancels, key)
		// done 通道保留：供 WaitForStop 等待 run loop 退出
	}
}
