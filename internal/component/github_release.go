package component

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type GitHubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []GitHubAsset `json:"assets"`
}

type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// FetchLatestRelease 查询 GitHub 仓库最新 Release
func FetchLatestRelease(owner, repo string) (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "vps-auto-deploy")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api status: %d", resp.StatusCode)
	}

	var rel GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode release json error: %w", err)
	}

	return &rel, nil
}

// DownloadFile 下载文件到指定目标路径
func DownloadFile(downloadURL, destPath string) error {
	client := &http.Client{Timeout: 300 * time.Second}

	// 优先直接下载，若失败可支持 ghproxy 镜像代理
	urlsToTry := []string{downloadURL}
	if !strings.HasPrefix(downloadURL, "https://ghproxy.") {
		urlsToTry = append(urlsToTry, "https://ghproxy.net/"+downloadURL)
	}

	var lastErr error
	for _, u := range urlsToTry {
		resp, err := client.Get(u)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("download status: %d from %s", resp.StatusCode, u)
			continue
		}

		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			resp.Body.Close()
			return fmt.Errorf("create dest file error: %w", err)
		}

		_, err = io.Copy(out, resp.Body)
		out.Close()
		resp.Body.Close()
		if err == nil {
			return nil
		}
		lastErr = err
	}

	return fmt.Errorf("download failed: %w", lastErr)
}

