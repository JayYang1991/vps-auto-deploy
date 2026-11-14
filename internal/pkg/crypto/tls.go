package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vps-auto-deploy/internal/pkg/netutil"
)

// FingerprintSHA256 计算证书 DER 二进制数据的 SHA-256 指纹（小写十六进制，每两位以冒号隔开）
func FingerprintSHA256(der []byte) string {
	sum := sha256.Sum256(der)
	hexStr := hex.EncodeToString(sum[:])
	var parts []string
	for i := 0; i < len(hexStr); i += 2 {
		parts = append(parts, hexStr[i:i+2])
	}
	return strings.Join(parts, ":")
}

// GenerateSelfSignedCert 生成 ECDSA P-256 自签名 TLS 证书与私钥，有效期 10 年
func GenerateSelfSignedCert(certPath, keyPath string, hosts []string) (string, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", fmt.Errorf("生成 ECDSA 私钥失败: %w", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return "", fmt.Errorf("生成序列号失败: %w", err)
	}

	notBefore := time.Now().Add(-1 * time.Hour)
	notAfter := notBefore.Add(10 * 365 * 24 * time.Hour) // 10 年有效期

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"VPS Auto Deploy Cluster Node"},
			CommonName:   "vps-panel-node",
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// 绑定主机名与 IP SANs
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}
	// 默认添加 localhost 与 127.0.0.1
	template.IPAddresses = append(template.IPAddresses, net.ParseIP("127.0.0.1"))
	template.DNSNames = append(template.DNSNames, "localhost")

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return "", fmt.Errorf("创建 X509 证书失败: %w", err)
	}

	fingerprint := FingerprintSHA256(derBytes)

	// 创建证书与私钥目录 (严格 0700 权限，防本地低权限读取私钥)
	if err := os.MkdirAll(filepath.Dir(certPath), 0700); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return "", err
	}

	// 写入证书 cert.pem
	/* #nosec G304 - internal TLS cert path */
	certOut, err := os.OpenFile(certPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return "", fmt.Errorf("写入证书文件失败: %w", err)
	}
	defer certOut.Close()
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return "", err
	}

	// 写入私钥 key.pem
	/* #nosec G304 - internal TLS key path */
	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return "", fmt.Errorf("写入私钥文件失败: %w", err)
	}
	defer keyOut.Close()

	privBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return "", err
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes}); err != nil {
		return "", err
	}

	return fingerprint, nil
}

// GetCertFingerprintFromFile 从现有 cert.pem 中读取并计算 SHA-256 指纹
func GetCertFingerprintFromFile(certPath string) (string, error) {
	/* #nosec G304 - internal TLS cert path */
	data, err := os.ReadFile(certPath)
	if err != nil {
		return "", err
	}

	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", errors.New("无效的 PEM 证书文件")
	}

	return FingerprintSHA256(block.Bytes), nil
}

// LoadOrGenerateCert 加载现有证书或自动生成新证书，并返回指纹
func LoadOrGenerateCert(certPath, keyPath string, hosts []string) (string, error) {
	if _, err := os.Stat(certPath); err == nil {
		if _, errKey := os.Stat(keyPath); errKey == nil {
			fp, err := GetCertFingerprintFromFile(certPath)
			if err == nil && fp != "" {
				return fp, nil
			}
		}
	}
	return GenerateSelfSignedCert(certPath, keyPath, hosts)
}

// CreatePinnedTLSConfig 创建针对指定证书指纹进行硬件绑定的 TLS 配置
// 当设置了 pinnedFingerprint 时，即使对方证书自签或未经公共 CA 机构背书，
// 只要指纹完全一致即安全放行；一旦指纹不一致，立即中止连接并报错（防御 MITM 劫持）
func CreatePinnedTLSConfig(pinnedFingerprint string) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, /* #nosec G402 - pinned certificate verified via VerifyConnection */
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("远端未提供任何 TLS 证书")
			}
			peerCert := cs.PeerCertificates[0]
			currentFP := FingerprintSHA256(peerCert.Raw)

			cleanExpected := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(pinnedFingerprint), ":", ""))
			if cleanExpected == "" {
				return errors.New("【安全拦截】TLS 证书指纹未配置或为空，拒绝未经验证的自签连接 (防中间人劫持)")
			}

			cleanCurrent := strings.ToLower(strings.ReplaceAll(currentFP, ":", ""))
			if subtle.ConstantTimeCompare([]byte(cleanExpected), []byte(cleanCurrent)) != 1 {
				return fmt.Errorf("【安全拦截】检测到中间人攻击风险！远端证书指纹不匹配 (预期: %s, 实际: %s)", pinnedFingerprint, currentFP)
			}
			return nil
		},
	}
}

// CreatePinnedTransport 创建带指纹强校验且免疫 SSRF / DNS 重绑定的安全 HTTP Transport
func CreatePinnedTransport(pinnedFingerprint string) *http.Transport {
	t := netutil.CreateSafeHTTPTransport(60 * time.Second)
	t.TLSClientConfig = CreatePinnedTLSConfig(pinnedFingerprint)
	t.ForceAttemptHTTP2 = true
	t.MaxIdleConnsPerHost = 10
	return t
}

// CreatePinnedHTTPClient 创建配置有指纹锁定的 HTTP 客户端
func CreatePinnedHTTPClient(pinnedFingerprint string, timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: CreatePinnedTransport(pinnedFingerprint),
		Timeout:   timeout,
	}
}

