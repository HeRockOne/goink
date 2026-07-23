package webdav

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
	"golang.org/x/net/webdav"
)

// Server WebDAV 服务器 + mDNS 广播
type Server struct {
	logger   *slog.Logger
	rootDir  string
	port     int
	username string
	password string

	httpServer *http.Server
	mdnsServer *zeroconf.Server
	running    bool
	mu         sync.Mutex
}

func New(rootDir string, port int, username, password string, logger *slog.Logger) *Server {
	return &Server{logger: logger, rootDir: rootDir, port: port, username: username, password: password}
}

func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return fmt.Errorf("已在运行")
	}
	os.MkdirAll(s.rootDir, 0755)

	// WebDAV handler（标准 WebDAV 协议，支持 PROPFIND/PROPPATCH/MKCOL 等）
	davHandler := &webdav.Handler{
		Prefix:     "/",
		FileSystem: webdav.Dir(s.rootDir),
		LockSystem: webdav.NewMemLS(),
		Logger: func(r *http.Request, err error) {
			if err != nil {
				s.logger.Warn("WebDAV error", "method", r.Method, "path", r.URL.Path, "err", err)
			}
		},
	}

	mux := http.NewServeMux()
	// WebDAV 处理所有方法（GET/PUT/DELETE/PROPFIND/MKCOL 等）
	mux.Handle("/", s.withAuth(davHandler))

	s.httpServer = &http.Server{Addr: fmt.Sprintf(":%d", s.port), Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	s.running = true

	go func() {
		s.httpServer.ListenAndServe()
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	// mDNS 广播
	go s.startMdns()

	return nil
}

func (s *Server) startMdns() {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "goink"
	}

	server, err := zeroconf.Register(
		hostname,
		"_webdav._tcp",
		".local.",
		s.port,
		[]string{"path=/"},
		nil,
	)
	if err != nil {
		s.logger.Warn("mDNS 广播启动失败", "err", err)
		return
	}
	s.mdnsServer = server
	s.logger.Info("mDNS 广播已启动", "name", hostname)
}

func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return nil
	}
	if s.httpServer != nil {
		s.httpServer.Shutdown(context.Background())
	}
	if s.mdnsServer != nil {
		s.mdnsServer.Shutdown()
	}
	s.running = false
	return nil
}

func (s *Server) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *Server) GetURL() string {
	ip := getLocalIP()
	hostname := getHostname()
	return fmt.Sprintf("浏览器阅读: http://%s:%d/\n文件管理器: http://%s:%d/\n用户名: %s  密码: %s", ip, s.port, hostname, s.port, s.username, s.password)
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != s.username || pass != s.password {
			w.Header().Set("WWW-Authenticate", `Basic realm="Goink"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	// 优先取局域网 IP（192.168.x.x / 10.x.x.x / 172.16-31.x.x）
	var fallback string
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			ip := ipnet.IP.String()
			// 跳过 APIPA 地址（169.254.x.x）
			if strings.HasPrefix(ip, "169.254.") {
				continue
			}
			// 优先局域网地址
			if strings.HasPrefix(ip, "192.168.") || strings.HasPrefix(ip, "10.") ||
				(strings.HasPrefix(ip, "172.") && len(ip) > 4) {
				return ip
			}
			if fallback == "" {
				fallback = ip
			}
		}
	}
	if fallback != "" {
		return fallback
	}
	return "127.0.0.1"
}

func getHostname() string {
	h, _ := os.Hostname()
	if h == "" {
		h = "localhost"
	}
	return strings.ToLower(h)
}
