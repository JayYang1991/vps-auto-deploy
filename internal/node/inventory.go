package node

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/crypto"
)

// FetchRemoteNodeInventory 跨节点抓取受管端的 inbounds 与 tunnels 配置
func FetchRemoteNodeInventory(node *model.Node) ([]model.Inbound, map[int64]string, error) {
	scheme := "http"
	if node.UseTLS {
		scheme = "https"
	}

	var client *http.Client
	if node.UseTLS {
		client = crypto.CreatePinnedHTTPClient(node.CertFingerprint, 3*time.Second)
	} else {
		client = &http.Client{Timeout: 3 * time.Second}
	}

	// 1. 抓取 inbounds
	ibURL := fmt.Sprintf("%s://%s:%d/%s/api/inbounds", scheme, node.Host, node.PanelPort, strings.Trim(node.SecretPath, "/"))
	req, err := http.NewRequest(http.MethodGet, ibURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+node.APIToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var inbounds []model.Inbound
	if err := json.NewDecoder(resp.Body).Decode(&inbounds); err != nil {
		return nil, nil, err
	}

	// 2. 抓取 tunnels
	tunnelsMap := make(map[int64]string)
	tnURL := fmt.Sprintf("%s://%s:%d/%s/api/tunnels", scheme, node.Host, node.PanelPort, strings.Trim(node.SecretPath, "/"))
	reqTn, err := http.NewRequest(http.MethodGet, tnURL, nil)
	if err == nil {
		reqTn.Header.Set("Authorization", "Bearer "+node.APIToken)
		if respTn, err := client.Do(reqTn); err == nil {
			defer respTn.Body.Close()
			if respTn.StatusCode == http.StatusOK {
				var tunnels []model.Tunnel
				if err := json.NewDecoder(respTn.Body).Decode(&tunnels); err == nil {
					for _, t := range tunnels {
						if t.Domain != "" {
							tunnelsMap[t.ID] = t.Domain
						}
					}
				}
			}
		}
	}

	return inbounds, tunnelsMap, nil
}
