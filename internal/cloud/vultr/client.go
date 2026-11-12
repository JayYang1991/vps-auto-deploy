package vultr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const DefaultBaseURL = "https://api.vultr.com/v2"

// Client Vultr API v2 轻量客户端（纯 Go 标准库，零第三方依赖）
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewClient 创建 Vultr 客户端
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		baseURL:    DefaultBaseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Account Vultr 账户信息
type Account struct {
	Balance        float64  `json:"balance"`
	PendingCharges float64  `json:"pending_charges"`
	Now            string   `json:"now"`
	Name           string   `json:"name"`
	Email          string   `json:"email"`
	ACLs           []string `json:"acls"`
}

// Region Vultr 数据中心区域
type Region struct {
	ID        string   `json:"id"`
	City      string   `json:"city"`
	Country   string   `json:"country"`
	Continent string   `json:"continent"`
	Options   []string `json:"options"`
}

// Plan Vultr 计算实例规格
type Plan struct {
	ID          string   `json:"id"`
	VCPUCount   int      `json:"vcpu_count"`
	RAM         int      `json:"ram"`
	Disk        int      `json:"disk"`
	DiskCount   int      `json:"disk_count"`
	Bandwidth   int      `json:"bandwidth"`
	MonthlyCost float64  `json:"monthly_cost"`
	Type        string   `json:"type"` // "vc2" | "vhf" | "vhp" | "vdc"
	Locations   []string `json:"locations"`
}

// OS Vultr 操作系统镜像
type OS struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Family string `json:"family"`
	Arch   string `json:"arch"`
}

// Instance Vultr 虚拟机实例
type Instance struct {
	ID              string   `json:"id"`
	Os              string   `json:"os"`
	RAM             int      `json:"ram"`
	Disk            int      `json:"disk"`
	MainIP          string   `json:"main_ip"`
	V6MainIP        string   `json:"v6_main_ip"`
	VCPUCount       int      `json:"vcpu_count"`
	Region          string   `json:"region"`
	Plan            string   `json:"plan"`
	DateCreated     string   `json:"date_created"`
	Status          string   `json:"status"`        // "pending" | "active"
	PowerStatus     string   `json:"power_status"`  // "running" | "stopped"
	ServerStatus    string   `json:"server_status"` // "installingbooting" | "ok"
	DefaultPassword string   `json:"default_password"`
	Label           string   `json:"label"`
	Tags            []string `json:"tags"`
}

// SSHKey Vultr 托管的 SSH 公钥
type SSHKey struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	SSHKey string `json:"ssh_key"`
}

// 缓存结构体
var (
	catalogCacheLock sync.RWMutex
	cachedRegions    []Region
	cachedPlans      []Plan
	cachedOSList     []OS
	cacheExpireAt    time.Time
)

func (c *Client) doRequest(method, path string, bodyData any) ([]byte, int, error) {
	fullURL := c.baseURL + path
	var bodyReader io.Reader
	if bodyData != nil {
		b, err := json.Marshal(bodyData)
		if err != nil {
			return nil, 0, err
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, fullURL, bodyReader)
	if err != nil {
		return nil, 0, err
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	// 限制读取最大 5MB
	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return nil, resp.StatusCode, err
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(respBytes, &errResp)
		msg := errResp.Error
		if msg == "" {
			msg = string(respBytes)
		}
		return respBytes, resp.StatusCode, fmt.Errorf("vultr api 错误 (%d): %s", resp.StatusCode, msg)
	}

	return respBytes, resp.StatusCode, nil
}

// GetAccount 获取账户信息（余额、已发生账单、邮箱）
func (c *Client) GetAccount() (*Account, error) {
	bytes, _, err := c.doRequest(http.MethodGet, "/account", nil)
	if err != nil {
		return nil, err
	}
	var res struct {
		Account Account `json:"account"`
	}
	if err := json.Unmarshal(bytes, &res); err != nil {
		return nil, err
	}
	return &res.Account, nil
}

// ListRegions 列出所有数据中心区域（带 15 分钟内存缓存）
func (c *Client) ListRegions(forceRefresh bool) ([]Region, error) {
	catalogCacheLock.RLock()
	if !forceRefresh && len(cachedRegions) > 0 && time.Now().Before(cacheExpireAt) {
		defer catalogCacheLock.RUnlock()
		return cachedRegions, nil
	}
	catalogCacheLock.RUnlock()

	bytes, _, err := c.doRequest(http.MethodGet, "/regions?per_page=100", nil)
	if err != nil {
		return nil, err
	}
	var res struct {
		Regions []Region `json:"regions"`
	}
	if err := json.Unmarshal(bytes, &res); err != nil {
		return nil, err
	}

	catalogCacheLock.Lock()
	cachedRegions = res.Regions
	cacheExpireAt = time.Now().Add(15 * time.Minute)
	catalogCacheLock.Unlock()

	return res.Regions, nil
}

// ListPlans 列出所有云主机规格套餐（带 15 分钟内存缓存）
func (c *Client) ListPlans(forceRefresh bool) ([]Plan, error) {
	catalogCacheLock.RLock()
	if !forceRefresh && len(cachedPlans) > 0 && time.Now().Before(cacheExpireAt) {
		defer catalogCacheLock.RUnlock()
		return cachedPlans, nil
	}
	catalogCacheLock.RUnlock()

	bytes, _, err := c.doRequest(http.MethodGet, "/plans?per_page=500", nil)
	if err != nil {
		return nil, err
	}
	var res struct {
		Plans []Plan `json:"plans"`
	}
	if err := json.Unmarshal(bytes, &res); err != nil {
		return nil, err
	}

	catalogCacheLock.Lock()
	cachedPlans = res.Plans
	cacheExpireAt = time.Now().Add(15 * time.Minute)
	catalogCacheLock.Unlock()

	return res.Plans, nil
}

// ListOSList 列出兼容的 64 位 Linux 操作系统镜像
func (c *Client) ListOSList(forceRefresh bool) ([]OS, error) {
	catalogCacheLock.RLock()
	if !forceRefresh && len(cachedOSList) > 0 && time.Now().Before(cacheExpireAt) {
		defer catalogCacheLock.RUnlock()
		return cachedOSList, nil
	}
	catalogCacheLock.RUnlock()

	bytes, _, err := c.doRequest(http.MethodGet, "/os?per_page=200", nil)
	if err != nil {
		return nil, err
	}
	var res struct {
		OSList []OS `json:"os"`
	}
	if err := json.Unmarshal(bytes, &res); err != nil {
		return nil, err
	}

	// 智能过滤：仅保留原生搭载 Systemd 的主流 Linux 64位镜像，剔除 Windows/BSD/Alpine
	var filtered []OS
	for _, o := range res.OSList {
		if o.Arch != "x64" && o.Arch != "x86_64" && o.Arch != "arm64" {
			continue
		}
		fam := strings.ToLower(o.Family)
		if fam == "debian" || fam == "ubuntu" || fam == "rocky linux" || fam == "almalinux" || fam == "centos" || fam == "fedora" || fam == "arch linux" {
			filtered = append(filtered, o)
		}
	}

	catalogCacheLock.Lock()
	cachedOSList = filtered
	cacheExpireAt = time.Now().Add(15 * time.Minute)
	catalogCacheLock.Unlock()

	return filtered, nil
}

// EnsureSSHKey 确保主控公钥已注册至 Vultr，若已存在同内容公钥则直接复用
func (c *Client) EnsureSSHKey(keyName, publicKey string) (string, error) {
	cleanPub := strings.TrimSpace(publicKey)
	// 1. 查询现有 SSH 密钥
	respBytes, _, err := c.doRequest(http.MethodGet, "/ssh-keys?per_page=100", nil)
	if err == nil {
		var listRes struct {
			SSHKeys []SSHKey `json:"ssh_keys"`
		}
		if err := json.Unmarshal(respBytes, &listRes); err == nil {
			for _, k := range listRes.SSHKeys {
				if strings.TrimSpace(k.SSHKey) == cleanPub {
					return k.ID, nil
				}
			}
		}
	}

	// 2. 若不存在，创建新 SSH 密钥
	payload := map[string]string{
		"name":    keyName,
		"ssh_key": cleanPub,
	}
	respBytes, _, err = c.doRequest(http.MethodPost, "/ssh-keys", payload)
	if err != nil {
		return "", err
	}

	var createRes struct {
		SSHKey SSHKey `json:"ssh_key"`
	}
	if err := json.Unmarshal(respBytes, &createRes); err != nil {
		return "", err
	}
	return createRes.SSHKey.ID, nil
}

// CreateInstanceRequest 创建云实例参数
type CreateInstanceRequest struct {
	Region     string   `json:"region"`
	Plan       string   `json:"plan"`
	OSID       int      `json:"os_id"`
	Label      string   `json:"label"`
	SSHKeyIDs  []string `json:"sshkey_id,omitempty"`
	EnableIPv6 bool     `json:"enable_ipv6"`
}

// CreateInstance 创建实例
func (c *Client) CreateInstance(req CreateInstanceRequest) (*Instance, error) {
	payload := map[string]any{
		"region":      req.Region,
		"plan":        req.Plan,
		"os_id":       req.OSID,
		"label":       req.Label,
		"backups":     "disabled",
		"enable_ipv6": req.EnableIPv6,
	}
	if len(req.SSHKeyIDs) > 0 {
		payload["sshkey_id"] = req.SSHKeyIDs
	}

	respBytes, _, err := c.doRequest(http.MethodPost, "/instances", payload)
	if err != nil {
		return nil, err
	}

	var res struct {
		Instance Instance `json:"instance"`
	}
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return nil, err
	}
	return &res.Instance, nil
}

// ListInstances 查询当前账户下的所有虚拟机实例
func (c *Client) ListInstances() ([]Instance, error) {
	respBytes, _, err := c.doRequest(http.MethodGet, "/instances?per_page=500", nil)
	if err != nil {
		return nil, err
	}

	var res struct {
		Instances []Instance `json:"instances"`
	}
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return nil, err
	}
	return res.Instances, nil
}

// GetInstance 查询单个实例详情
func (c *Client) GetInstance(id string) (*Instance, error) {
	respBytes, _, err := c.doRequest(http.MethodGet, "/instances/"+id, nil)
	if err != nil {
		return nil, err
	}

	var res struct {
		Instance Instance `json:"instance"`
	}
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return nil, err
	}
	return &res.Instance, nil
}

// DeleteInstance 彻底销毁实例并停止计费
func (c *Client) DeleteInstance(id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("instance id 不能为空")
	}
	_, _, err := c.doRequest(http.MethodDelete, "/instances/"+id, nil)
	return err
}

