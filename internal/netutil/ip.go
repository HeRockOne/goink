package netutil

import (
	"net"
	"strings"
)

// isPrivateLAN 检查 IP 是否属于常见局域网段（192.168.x / 10.x / 172.16-31.x）。
func isPrivateLAN(ip string) bool {
	if strings.HasPrefix(ip, "192.168.") {
		return true
	}
	if strings.HasPrefix(ip, "10.") {
		return true
	}
	if strings.HasPrefix(ip, "172.") {
		// 检查 172.16.0.0/12 范围
		parts := strings.SplitN(ip, ".", 3)
		if len(parts) >= 2 {
			// 简化：只检查常见段
			second := parts[1]
			if second == "16" || second == "17" || second == "18" || second == "19" ||
				second == "20" || second == "21" || second == "22" || second == "23" ||
				second == "24" || second == "25" || second == "26" || second == "27" ||
				second == "28" || second == "29" || second == "30" || second == "31" {
				return true
			}
		}
	}
	return false
}

// isVPNAddress 检查 IP 是否属于已知 VPN/虚拟网卡地址段。
func isVPNAddress(ip string) bool {
	// WireGuard 默认分配的 10.x.x.x 但在 VPN 上下文中
	// 100.64.0.0/10 (NAT64/DNS64, WireGuard, 运营商级 NAT)
	if strings.HasPrefix(ip, "100.") {
		// 解析第二段数字
		parts := strings.SplitN(ip, ".", 3)
		if len(parts) >= 2 {
			second := 0
			for _, c := range parts[1] {
				second = second*10 + int(c-'0')
			}
			if second >= 64 && second <= 127 {
				return true
			}
		}
	}
	// 198.18.0.0/15 (VMware/VirtualBox 虚拟网卡)
	if strings.HasPrefix(ip, "198.18.") || strings.HasPrefix(ip, "198.19.") {
		return true
	}
	// 169.254.x.x (APIPA)
	if strings.HasPrefix(ip, "169.254.") {
		return true
	}
	// 172.16.0.0/12 在 VPN 上下文（但保留 LAN 优先的 172.16-31.x）
	// 这里不标记为 VPN，因为在局域网中 172.16.x.x 也可能是真实 LAN
	return false
}

// isVPNInterface 根据网卡名称判断是否为 VPN/虚拟网卡。
func isVPNInterface(name string) bool {
	lower := strings.ToLower(name)
	// Windows VPN 适配器常见命名
	vpnPatterns := []string{
		"tun", "tap", "wg", "wireguard", "openvpn",
		"clash", "v2ray", "trojan", "sing-box", "mihomo",
		"hamachi", "tailscale", "zerotier",
		"cisco", "forticlient", "globalprotect",
		"sangfor", "easyconnect",
	}
	for _, p := range vpnPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	// Windows: 虚拟网卡通常以 "Virtual", "VPN", "Tunnel" 开头
	if strings.HasPrefix(lower, "virtual") || strings.HasPrefix(lower, "vpn") || strings.HasPrefix(lower, "tunnel") {
		return true
	}
	return false
}

// NetworkInterface 表示一个可用的网络接口及其 IP。
type NetworkInterface struct {
	Name    string // 网卡名称（如 "WLAN", "以太网"）
	IP      string // IPv4 地址
	IsLAN   bool   // 是否局域网地址
	IsVPN   bool   // 是否 VPN/虚拟网卡
}

// GetLocalInterfaces 返回所有可用的 IPv4 网络接口，供前端选择。
func GetLocalInterfaces() []NetworkInterface {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}

	// 获取接口名称映射
	ifaceMap := make(map[string]string)
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			ifaceAddrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range ifaceAddrs {
				if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
					ifaceMap[ipnet.IP.String()] = iface.Name
				}
			}
		}
	}

	var result []NetworkInterface
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() || ipnet.IP.To4() == nil {
			continue
		}
		ip := ipnet.IP.String()
		name := ifaceMap[ip]
		if name == "" {
			name = "unknown"
		}
		result = append(result, NetworkInterface{
			Name:  name,
			IP:    ip,
			IsLAN: isPrivateLAN(ip),
			IsVPN: isVPNAddress(ip) || isVPNInterface(name),
		})
	}
	return result
}

// GetLocalIP 智能选择局域网 IP：
// 1. 优先返回 192.168.x.x / 10.x.x.x / 172.16-31.x.x 中非 VPN 的地址
// 2. 次选 VPN 排除后的其他非 loopback IPv4 地址
// 3. 兜底 127.0.0.1
func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}

	// 获取接口名称映射（用于 VPN 接口检测）
	ifaceMap := make(map[string]string)
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			ifaceAddrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range ifaceAddrs {
				if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
					ifaceMap[ipnet.IP.String()] = iface.Name
				}
			}
		}
	}

	// 三轮选择：LAN 非 VPN > 非 VPN > 任意
	var fallbackVPN, fallbackAny string
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() || ipnet.IP.To4() == nil {
			continue
		}
		ip := ipnet.IP.String()
		ifaceName := ifaceMap[ip]
		vpn := isVPNAddress(ip) || isVPNInterface(ifaceName)

		if isPrivateLAN(ip) && !vpn {
			return ip // 最优：局域网 + 非 VPN
		}
		if !vpn && fallbackAny == "" {
			fallbackAny = ip
		}
		if vpn && fallbackVPN == "" {
			fallbackVPN = ip
		}
	}

	if fallbackAny != "" {
		return fallbackAny
	}
	if fallbackVPN != "" {
		return fallbackVPN
	}
	return "127.0.0.1"
}
