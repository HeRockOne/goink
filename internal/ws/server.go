package ws

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Hub 管理所有 WebSocket 连接
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
	logger     *slog.Logger
}

// Client 代表一个 WebSocket 连接
type Client struct {
	hub     *Hub
	conn    *websocket.Conn
	send    chan []byte
	userID  string
	logger  *slog.Logger
}

// Message 是客户端和服务端之间的消息格式
type Message struct {
	Type       string          `json:"type"`
	SessionID  string          `json:"session_id,omitempty"`
	NovelID    int64           `json:"novel_id,omitempty"`
	Message    string          `json:"message,omitempty"`
	Model      string          `json:"model,omitempty"`
	Provider   string          `json:"provider,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
	Error      string          `json:"error,omitempty"`
	TurnID     int             `json:"turn_id,omitempty"`
	PhaseGate  json.RawMessage `json:"phase_gate,omitempty"`
	RetryCount int             `json:"retry_count,omitempty"`
	RetryMax   int             `json:"retry_max,omitempty"`
	RetryWait  int             `json:"retry_wait,omitempty"`
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// NewHub 创建新的 Hub
func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		logger:     logger,
	}
}

// Run 启动 Hub 的主循环
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			h.logger.Info("WebSocket 客户端连接", "user_id", client.userID, "total", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			h.logger.Info("WebSocket 客户端断开", "user_id", client.userID, "total", len(h.clients))

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast 向所有客户端广播消息
func (h *Hub) Broadcast(msg []byte) {
	h.broadcast <- msg
}

// BroadcastTo 向指定用户广播消息
func (h *Hub) BroadcastTo(userID string, msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients {
		if client.userID == userID {
			select {
			case client.send <- msg:
			default:
			}
		}
	}
}

// ClientCount 返回当前连接数
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// HandleWS 处理 WebSocket 升级请求
func HandleWS(hub *Hub, authFunc func(r *http.Request) bool, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 认证
		if authFunc != nil && !authFunc(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.Warn("WebSocket 升级失败", "err", err)
			return
		}

		userID := r.URL.Query().Get("user_id")
		if userID == "" {
			userID = "anonymous"
		}

		client := &Client{
			hub:    hub,
			conn:   conn,
			send:   make(chan []byte, 256),
			userID: userID,
			logger: logger,
		}

		hub.register <- client

		go client.writePump()
		go client.readPump()
	}
}

// readPump 读取客户端消息
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512 * 1024)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				c.logger.Warn("WebSocket 读取错误", "err", err)
			}
			break
		}

		// 解析消息并处理
		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			c.logger.Warn("消息解析失败", "err", err)
			continue
		}

		c.logger.Info("收到客户端消息", "type", msg.Type, "user_id", c.userID)

		// 消息处理由外部注册的 handler 负责
		// 这里只做基础的 echo 或转发
	}
}

// writePump 向客户端写入消息
func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// SendMessage 向客户端发送消息
func (c *Client) SendMessage(msg Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	select {
	case c.send <- data:
		return nil
	default:
		return nil
	}
}
