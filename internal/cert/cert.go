package cert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EnsureCert 检查并生成 HTTPS 证书。
// 返回 (certFile, keyFile, ip, error)。
func EnsureCert(dataDir string) (certFile, keyFile, ip string, err error) {
	ip = getLocalIP()

	certDir := filepath.Join(dataDir, "certs")
	if err := os.MkdirAll(certDir, 0700); err != nil {
		return "", "", "", fmt.Errorf("创建证书目录失败: %w", err)
	}

	certFile = filepath.Join(certDir, ip+".pem")
	keyFile = filepath.Join(certDir, ip+"-key.pem")

	// 已有证书则直接返回
	if _, err := os.Stat(certFile); err == nil {
		if _, err := os.Stat(keyFile); err == nil {
			return certFile, keyFile, ip, nil
		}
	}

	// 生成新证书
	if err := generateCert(certFile, keyFile, ip); err != nil {
		return "", "", "", fmt.Errorf("生成证书失败: %w", err)
	}

	return certFile, keyFile, ip, nil
}

func generateCert(certFile, keyFile, ip string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Goink"},
			CommonName:   "Goink Local",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP(ip), net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return err
	}

	// 写证书
	certOut, err := os.Create(certFile)
	if err != nil {
		return err
	}
	defer certOut.Close()
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	// 写私钥
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	keyOut, err := os.Create(keyFile)
	if err != nil {
		return err
	}
	defer keyOut.Close()
	pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return nil
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	// 优先取局域网 IP（192.168.x.x / 10.x.x.x / 172.16-31.x.x）
	var fallback string
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			ip := ipNet.IP.String()
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
