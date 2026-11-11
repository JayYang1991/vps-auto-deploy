package node

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/crypto"
)

// SyncMembersToNode 将小组成员列表精准推送至单个受管节点
func SyncMembersToNode(node *model.Node, members []model.Member) error {
	if node == nil {
		return fmt.Errorf("node is nil")
	}
	if node.IsMaster {
		return nil
	}

	var err error
	if members == nil {
		members, err = model.ListMembers()
		if err != nil {
			return fmt.Errorf("获取本地主控成员列表失败: %w", err)
		}
	}

	payload, err := json.Marshal(members)
	if err != nil {
		return fmt.Errorf("序列化成员凭据失败: %w", err)
	}

	scheme := "http"
	if node.UseTLS {
		scheme = "https"
	}

	url := fmt.Sprintf("%s://%s:%d/%s/api/members/sync?mode=replace",
		scheme,
		node.Host,
		node.PanelPort,
		strings.Trim(node.SecretPath, "/"),
	)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("创建同步请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+node.APIToken)

	var client *http.Client
	if node.UseTLS {
		client = crypto.CreatePinnedHTTPClient(node.CertFingerprint, 25*time.Second)
	} else {
		client = &http.Client{Timeout: 25 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送凭据同步网络请求失败: %w", err)
	}

	// 若 Token 偶发失效 (401)，尝试自愈解密密码重新登录刷新 Token
	if resp.StatusCode == http.StatusUnauthorized && node.PasswordEncrypted != "" {
		resp.Body.Close()
		rawPass, decErr := crypto.DecryptAESGCM(node.PasswordEncrypted, model.GetEncryptionKey())
		if decErr == nil && rawPass != "" {
			newToken, logErr := LoginNode(node.Host, node.PanelPort, node.SecretPath, node.Username, rawPass, node.UseTLS, node.CertFingerprint)
			if logErr == nil && newToken != "" {
				node.APIToken = newToken
				_ = model.UpdateNodeToken(node.ID, newToken)
				// 重构请求体再次重试
				reqRetry, retryErr := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
				if retryErr == nil {
					reqRetry.Header.Set("Content-Type", "application/json")
					reqRetry.Header.Set("Authorization", "Bearer "+newToken)
					resp, err = client.Do(reqRetry)
					if err != nil {
						return fmt.Errorf("凭据刷新后重试发送失败: %w", err)
					}
				}
			}
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("远端节点版本过低未开启凭据同步接口 (HTTP 404)，请在集群管理中点击该节点的 [🚀 升级节点] 更新软件")
		}
		return fmt.Errorf("远端节点拒绝同步 (HTTP %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// BroadcastMembersSync 将主控的成员身份全量广播同步给集群中所有启用的受管节点
func BroadcastMembersSync() (total int, successCount int, failedCount int, errs []error) {
	nodes, err := model.ListNodes()
	if err != nil {
		return 0, 0, 0, []error{fmt.Errorf("获取集群节点列表失败: %w", err)}
	}

	var targetNodes []model.Node
	for _, n := range nodes {
		if !n.IsMaster && (n.Status == "online" || n.Status == "") {
			targetNodes = append(targetNodes, n)
		}
	}

	if len(targetNodes) == 0 {
		return 0, 0, 0, nil
	}

	members, err := model.ListMembers()
	if err != nil {
		return len(targetNodes), 0, len(targetNodes), []error{fmt.Errorf("获取主控成员失败: %w", err)}
	}

	total = len(targetNodes)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, n := range targetNodes {
		nodeCopy := n
		wg.Add(1)
		go func(target model.Node) {
			defer wg.Done()
			syncErr := SyncMembersToNode(&target, members)
			mu.Lock()
			defer mu.Unlock()
			if syncErr != nil {
				failedCount++
				errs = append(errs, fmt.Errorf("节点 %s (%s): %v", target.Name, target.Host, syncErr))
				log.Printf("[MemberSync] 同步成员至节点 %s (%s) 失败: %v", target.Name, target.Host, syncErr)
			} else {
				successCount++
				log.Printf("[MemberSync] 成功同步成员凭据至受管节点 %s (%s)", target.Name, target.Host)
			}
		}(nodeCopy)
	}

	wg.Wait()
	return total, successCount, failedCount, errs
}
