package llm

import (
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// sharedTransport 包级共享连接池。旧实现每次请求新建 Transport，零连接复用：
// 每次 LLM 调用都付完整 DNS+TCP+TLS 握手（同网络同模型下其他共享 keep-alive
// 的平台握手快一个量级），一章几十个工具回合就是几十次全握手。
// Clone 自 DefaultTransport：自带 HTTP/2 与合理默认值，仅覆写代理与池参数。
var sharedTransport = func() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = cachedSystemProxy
	t.MaxIdleConns = 100
	t.MaxIdleConnsPerHost = 4
	t.IdleConnTimeout = 90 * time.Second
	t.TLSHandshakeTimeout = 15 * time.Second
	return t
}()

// newHTTPClient 返回共享连接池上的客户端。timeout 只约束非流式调用；
// 流式调用传 0，由请求 ctx 控制生命周期（见 stream.go）。
func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: sharedTransport,
	}
}

// cachedSystemProxy 缓存系统代理解析结果。Proxy 钩子在每次新建连接时都会被调，
// Windows 注册表读取不应发生在拨号热路径上；缓存 5 分钟兼顾配置变更感知。
var (
	proxyMu     sync.Mutex
	proxyCached *url.URL
	proxyExpire time.Time
)

func cachedSystemProxy(req *http.Request) (*url.URL, error) {
	proxyMu.Lock()
	defer proxyMu.Unlock()
	if time.Now().Before(proxyExpire) {
		return proxyCached, nil
	}
	u, err := systemProxy(req)
	if err != nil {
		return nil, err
	}
	proxyCached = u
	proxyExpire = time.Now().Add(5 * time.Minute)
	return u, nil
}

// systemProxy 解析系统代理设置。
// 优先读环境变量（Linux/macOS/手动设置）；Windows 读注册表 IE 代理设置。
func systemProxy(req *http.Request) (*url.URL, error) {
	// 优先读环境变量（Linux/macOS/手动设置）
	if p := os.Getenv("HTTPS_PROXY"); p != "" {
		return url.Parse(p)
	}
	if p := os.Getenv("HTTP_PROXY"); p != "" {
		return url.Parse(p)
	}
	if p := os.Getenv("https_proxy"); p != "" {
		return url.Parse(p)
	}
	if p := os.Getenv("http_proxy"); p != "" {
		return url.Parse(p)
	}
	// Windows: 读注册表 IE 代理
	if runtime.GOOS == "windows" {
		if p := windowsSystemProxy(); p != "" {
			return url.Parse(p)
		}
	}
	return nil, nil
}

func windowsSystemProxy() string {
	// 读 HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings
	// ProxyEnable=1 时 ProxyServer 值即代理地址
	key, err := openInternetSettingsKey()
	if err != nil {
		return ""
	}
	defer key.Close()

	enabled, err := readDWORD(key, "ProxyEnable")
	if err != nil || enabled == 0 {
		return ""
	}

	server, err := readString(key, "ProxyServer")
	if err != nil || server == "" {
		return ""
	}

	// ProxyServer 可能是 "http=host:port;https=host:port" 或直接 "host:port"
	if strings.Contains(server, "=") {
		// 解析分号分隔的协议映射
		for _, part := range strings.Split(server, ";") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "https=") {
				return "http://" + strings.TrimPrefix(part, "https=")
			}
			if strings.HasPrefix(part, "http=") {
				return "http://" + strings.TrimPrefix(part, "http=")
			}
		}
		// 没找到 https= 条目，取第一个
		if idx := strings.IndexByte(server, ';'); idx > 0 {
			server = server[:idx]
		}
		return "http://" + strings.TrimSpace(server)
	}

	// 直接是 host:port 格式
	return "http://" + server
}
