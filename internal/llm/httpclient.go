package llm

import (
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"
)

// newHTTPClient 创建带系统代理的 HTTP 客户端。
// Windows: 读注册表 IE 代理设置；其他平台: 读 HTTP_PROXY/HTTPS_PROXY 环境变量。
func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: systemProxy,
		},
	}
}

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
