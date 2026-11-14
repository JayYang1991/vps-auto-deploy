package component

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// BrowserUserAgent 模拟现代主流 Chrome 浏览器指纹 (Windows 11 x64, Chrome 133)
	BrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"
	// BrowserAcceptLanguage 真实浏览器多语言标头
	BrowserAcceptLanguage = "en-US,en;q=0.9,zh-CN;q=0.8,zh;q=0.7"
	// BrowserSecChUa 现代 Client Hints 特征标头
	BrowserSecChUa = `"Not(A:Brand";v="99", "Google Chrome";v="133", "Chromium";v="133"`
)

type GitHubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []GitHubAsset `json:"assets"`
}

type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type releaseCacheEntry struct {
	release   *GitHubRelease
	fetchedAt time.Time
}

var (
	releaseCache   = make(map[string]*releaseCacheEntry)
	releaseCacheMu sync.RWMutex
	// releaseTTL 版本查询在内存中缓存 10 分钟，严防频繁刷新耗尽 GitHub 60次/小时的未认证 IP 配额
	releaseTTL = 10 * time.Minute
)

// InjectBrowserHeaders 为 HTTP 请求注入拟真现代浏览器指纹标头
func InjectBrowserHeaders(req *http.Request, isAPI bool) {
	req.Header.Set("User-Agent", BrowserUserAgent)
	req.Header.Set("Accept-Language", BrowserAcceptLanguage)
	req.Header.Set("sec-ch-ua", BrowserSecChUa)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	if isAPI {
		req.Header.Set("Accept", "application/vnd.github.v3+json,application/json;q=0.9,*/*;q=0.8")
	} else {
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	}
}

// createBrowserHTTPClient 创建全程保持拟真浏览器指纹的 HTTP 客户端 (支持 302 重定向重写保持)
func createBrowserHTTPClient(timeout time.Duration, isAPI bool) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			// 重定向至 AWS S3 (objects.githubusercontent.com) 或 CDN 镜像时，持续保持浏览器伪装
			InjectBrowserHeaders(req, isAPI)
			return nil
		},
	}
}

// FetchLatestRelease 查询 GitHub 仓库最新 Release (带拟真浏览器指纹与防风控短时内存缓存)
func FetchLatestRelease(owner, repo string) (*GitHubRelease, error) {
	cacheKey := fmt.Sprintf("%s/%s", strings.ToLower(owner), strings.ToLower(repo))

	// 1. 优先命中有效缓存 (TTL 10m)
	releaseCacheMu.RLock()
	cached, found := releaseCache[cacheKey]
	releaseCacheMu.RUnlock()

	if found && cached != nil && time.Since(cached.fetchedAt) < releaseTTL {
		return cached.release, nil
	}

	// 2. 缓存过期或首次拉取，构造完整拟真请求
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	client := createBrowserHTTPClient(15*time.Second, true)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	InjectBrowserHeaders(req, true)

	resp, err := client.Do(req)
	if err != nil {
		// 网络故障时，若有旧缓存则优雅降级返回旧缓存
		if found && cached != nil {
			return cached.release, nil
		}
		return nil, fmt.Errorf("github api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 遭遇 403 / 429 风控限流时，优先返回陈旧缓存保底
		if found && cached != nil && (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests) {
			return cached.release, nil
		}
		return nil, fmt.Errorf("github api status: %d", resp.StatusCode)
	}

	var rel GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode release json error: %w", err)
	}

	// 3. 更新写入内存缓存
	releaseCacheMu.Lock()
	releaseCache[cacheKey] = &releaseCacheEntry{
		release:   &rel,
		fetchedAt: time.Now(),
	}
	releaseCacheMu.Unlock()

	return &rel, nil
}

// ComputeFileSHA256 计算文件的 SHA-256 十六进制小写哈希
func ComputeFileSHA256(filePath string) (string, error) {
	/* #nosec G304 - reading validated local file */
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifyFileSHA256 校验文件实际 SHA-256 与预期哈希是否匹配 (防供应链劫持与镜像投毒)
func VerifyFileSHA256(filePath, expectedHash string) error {
	cleanExpected := strings.ToLower(strings.TrimSpace(expectedHash))
	if cleanExpected == "" {
		return nil
	}

	actual, err := ComputeFileSHA256(filePath)
	if err != nil {
		return fmt.Errorf("计算文件哈希失败: %w", err)
	}

	if actual != cleanExpected {
		return fmt.Errorf("【供应链完整性拦截】文件 SHA-256 校验失败！预期: %s, 实际: %s (防镜像篡改/投毒)", cleanExpected, actual)
	}
	return nil
}

// FetchReleaseChecksums 解析 GitHub Release Assets 中的 checksum 文件 (如 *checksums*.txt, *sha256* 等)
func FetchReleaseChecksums(rel *GitHubRelease) map[string]string {
	result := make(map[string]string)
	if rel == nil || len(rel.Assets) == 0 {
		return result
	}

	client := createBrowserHTTPClient(15*time.Second, false)

	for _, asset := range rel.Assets {
		lowerName := strings.ToLower(asset.Name)
		// 识别常见的 Release 校验清单文件名
		if strings.Contains(lowerName, "checksum") || strings.HasSuffix(lowerName, ".sha256") || strings.Contains(lowerName, "sha256sum") {
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, asset.BrowserDownloadURL, nil)
			if err != nil {
				continue
			}
			InjectBrowserHeaders(req, false)
			resp, err := client.Do(req)
			if err != nil || resp.StatusCode != http.StatusOK {
				if resp != nil {
					resp.Body.Close()
				}
				continue
			}

			// 限制校验文件扫描最多读取 2MB，防范超大文件 DoS
			scanner := bufio.NewScanner(io.LimitReader(resp.Body, 2*1024*1024))
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					hash := strings.ToLower(strings.TrimSpace(fields[0]))
					filename := strings.TrimPrefix(strings.TrimSpace(fields[1]), "*")
					filename = filepath.Base(filename)
					if len(hash) == 64 { // SHA-256 为 64 个十六进制字符
						result[filename] = hash
					}
				} else if len(fields) == 1 && strings.HasSuffix(lowerName, ".sha256") {
					// 类似 <package>.sha256 仅包含单个哈希的情况
					hash := strings.ToLower(strings.TrimSpace(fields[0]))
					if len(hash) == 64 {
						targetPkg := strings.TrimSuffix(asset.Name, ".sha256")
						result[targetPkg] = hash
					}
				}
			}
			resp.Body.Close()
		}
	}

	return result
}

// DownloadFileWithChecksum 下载文件并严格校验 SHA-256 (若提供了 expectedHash，校验不匹配立即销毁文件并拦截)
func DownloadFileWithChecksum(downloadURL, destPath, expectedHash string) error {
	client := createBrowserHTTPClient(300*time.Second, false)

	// 优先直接下载，若失败可支持 ghproxy 镜像代理
	urlsToTry := []string{downloadURL}
	if !strings.HasPrefix(downloadURL, "https://ghproxy.") {
		urlsToTry = append(urlsToTry, "https://ghproxy.net/"+downloadURL)
	}

	var lastErr error
	for _, u := range urlsToTry {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u, nil)
		if err != nil {
			lastErr = err
			continue
		}
		InjectBrowserHeaders(req, false)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("download status: %d from %s", resp.StatusCode, u)
			continue
		}

		/* #nosec G302, G304 - downloaded binary file requires executable permission */
		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			resp.Body.Close()
			return fmt.Errorf("create dest file error: %w", err)
		}

		// 单文件下载上限限制 500MB，防范无限数据流与磁盘耗尽
		_, err = io.Copy(out, io.LimitReader(resp.Body, 500*1024*1024))
		out.Close()
		resp.Body.Close()
		if err != nil {
			_ = os.Remove(destPath)
			lastErr = err
			continue
		}

		// 下载完成后立即进行 SHA-256 强校验 (若提供了预期哈希)
		if expectedHash != "" {
			if verifyErr := VerifyFileSHA256(destPath, expectedHash); verifyErr != nil {
				_ = os.Remove(destPath)
				return verifyErr
			}
		}

		return nil
	}

	return fmt.Errorf("download failed: %w", lastErr)
}

// DownloadFile 下载文件到指定目标路径 (全链路注入拟真浏览器指纹与多源镜像回退)
func DownloadFile(downloadURL, destPath string) error {
	return DownloadFileWithChecksum(downloadURL, destPath, "")
}
