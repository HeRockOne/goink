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

	"novel/internal/netutil"
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
	return netutil.GetLocalIP()
}
