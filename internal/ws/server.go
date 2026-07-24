package ws

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	wspulse "github.com/wspulse/hub"
)

// Hub 基于 wspulse 的 WebSocket 管理器，支持房间路由和会话恢复。
type Hub struct {
	srv              wspulse.Hub
	logger           *slog.Logger
	mu               sync.RWMutex
	StreamSessionID  string // 当前流式会话的 session_id
	StreamThinking   string // 当前流式思考内容
	StreamContent    string // 当前流式正文内容
	StreamActive     bool   // 是否有活跃的流式输出
}

// NewHub 创建基于 wspulse 的 Hub。
func NewHub(logger *slog.Logger) *Hub {
	h := &Hub{logger: logger}

	h.srv = wspulse.NewHub(
		// 连接函数：所有客户端加入同一个 global 房间
		func(r *http.Request) (roomID, connectionID string, err error) {
			roomID = "global" // 统一使用 global 房间，确保全局广播
			connectionID = r.URL.Query().Get("user_id")
			if connectionID == "" {
				connectionID = "anonymous"
			}
			return roomID, connectionID, nil
		},
		wspulse.WithOnMessage(func(conn wspulse.Connection, msg wspulse.Message) {
			// 客户端发来的消息：广播到同一房间
			h.srv.Broadcast(conn.RoomID(), msg)
		}),
		wspulse.WithOnConnect(func(conn wspulse.Connection) {
			logger.Info("WebSocket 客户端连接", "id", conn.ID(), "room", conn.RoomID())
		}),
		wspulse.WithOnDisconnect(func(conn wspulse.Connection, err error) {
			logger.Info("WebSocket 客户端断开", "id", conn.ID(), "room", conn.RoomID())
		}),
		wspulse.WithPingInterval(30*time.Second),
		wspulse.WithResumeWindow(30*time.Second),
	)

	return h
}

// Run 启动 Hub（wspulse 内部管理，留空兼容旧接口）。
func (h *Hub) Run() {}

// Broadcast 向所有客户端广播消息（所有客户端在同一 global 房间）。
func (h *Hub) Broadcast(msg []byte) {
	h.srv.Broadcast("global", wspulse.Message{Event: "broadcast", Payload: msg})
}

// GetSyncState 返回当前流式状态的副本。
func (h *Hub) GetSyncState() (active bool, sessionID, thinking, content string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.StreamActive, h.StreamSessionID, h.StreamThinking, h.StreamContent
}

// UpdateStreamState 更新流式状态（由外部调用）。
func (h *Hub) UpdateStreamState(sessionID, thinking, content string, active bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.StreamSessionID = sessionID
	h.StreamThinking = thinking
	h.StreamContent = content
	h.StreamActive = active
}

// AppendThinking 追加思考内容。
func (h *Hub) AppendThinking(text string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.StreamThinking += text
}

// AppendContent 追加正文内容。
func (h *Hub) AppendContent(text string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.StreamContent += text
}

// HandleWS 返回 WebSocket 处理函数（挂载到 HTTP 路由）。
func (h *Hub) HandleWS(logger *slog.Logger) http.HandlerFunc {
	return h.srv.ServeHTTP
}
