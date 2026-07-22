package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
)

// apiServer 独立 HTTP 服务器，提供 REST API 和 WebSocket
type apiServer struct {
	port    int
	app     any
	logger  *slog.Logger
	server  *http.Server
	mux     *http.ServeMux
	wsHub   *WSHub
	mu      sync.Mutex
	running bool
}

// WSGub 简化的 WebSocket Hub
type WSHub struct {
	clients map[*WSClient]bool
	mu      sync.RWMutex
}

type WSClient struct {
	conn   net.Conn
	send   chan []byte
	hub    *WSHub
}

// NewServer 创建 API 服务器
func NewServer(port int, app any, logger *slog.Logger) *apiServer {
	s := &apiServer{
		port:   port,
		app:    app,
		logger: logger,
		wsHub:  &WSHub{clients: make(map[*WSClient]bool)},
	}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/api/server", s.handleServerInfo)
	s.mux.HandleFunc("/ws", s.handleWS)
	s.mux.Handle("/", withCORS(s.mux))
	return s
}

// Start 启动服务器
func (s *apiServer) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	addr := fmt.Sprintf(":%d", s.port)
	s.server = &http.Server{
		Addr:    addr,
		Handler: s.mux,
	}

	s.logger.Info("API 服务器启动", "port", s.port, "url", fmt.Sprintf("http://0.0.0.0:%d", s.port))
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		s.logger.Error("API 服务器错误", "err", err)
	}
}

// Stop 停止服务器
func (s *apiServer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.running = false
	if s.server != nil {
		s.server.Shutdown(context.Background())
	}
}

func (s *apiServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"status": "ok", "service": "goink"})
}

func (s *apiServer) handleServerInfo(w http.ResponseWriter, r *http.Request) {
	ip := getLocalIP()
	writeJSON(w, map[string]any{
		"ip":   ip,
		"port": s.port,
		"url":  fmt.Sprintf("http://%s:%d", ip, s.port),
	})
}

func (s *apiServer) handleWS(w http.ResponseWriter, r *http.Request) {
	// WebSocket 升级（简化版，后续完善）
	writeJSON(w, map[string]any{"message": "WebSocket endpoint"})
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func parseIntQuery(r *http.Request, key string) int64 {
	v := r.URL.Query().Get(key)
	if v == "" {
		return 0
	}
	id, _ := strconv.ParseInt(v, 10, 64)
	return id
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			ip := ipnet.IP.String()
			if len(ip) > 4 && ip[:4] != "169." {
				return ip
			}
		}
	}
	return "127.0.0.1"
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
