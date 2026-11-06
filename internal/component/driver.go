package component

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ServiceStatus 组件服务状态与元信息
type ServiceStatus struct {
	Name             string `json:"name"`
	DisplayName      string `json:"display_name"`
	Description      string `json:"description"`
	Category         string `json:"category"` // "proxy" | "tunnel"
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installed_version"`
	LatestVersion    string `json:"latest_version"`
	HasUpdate        bool   `json:"has_update"`
	Active           bool   `json:"active"`
	RunningStatus    string `json:"running_status"` // "running", "failed", "stopped", "not_installed"
}

// ComponentDriver 受管组件生命周期驱动接口
type ComponentDriver interface {
	Name() string
	DisplayName() string
	Description() string
	Category() string
	GetInstalledVersion() (string, error)
	GetLatestRelease() (string, error)
	InstallOrUpgrade(version string, logFn func(string)) error
	Uninstall() error
	Status() (ServiceStatus, error)
	Start() error
	Stop() error
	Restart() error
}

var (
	driversLock sync.RWMutex
	drivers     = make(map[string]ComponentDriver)

	// 版本缓存，避免频繁触发 GitHub API 限流
	versionCacheLock sync.RWMutex
	versionCache     = make(map[string]cacheItem)

	// 安装任务管理器
	tasksLock    sync.RWMutex
	installTasks = make(map[string]*InstallTask)
)

type cacheItem struct {
	Version   string
	FetchedAt time.Time
}

// Register 注册组件驱动
func Register(d ComponentDriver) {
	driversLock.Lock()
	defer driversLock.Unlock()
	drivers[d.Name()] = d
}

// Get 获取已注册的组件驱动
func Get(name string) (ComponentDriver, error) {
	driversLock.RLock()
	defer driversLock.RUnlock()
	d, ok := drivers[name]
	if !ok {
		return nil, fmt.Errorf("driver %s not found", name)
	}
	return d, nil
}

// ListAll 列出所有组件状态
func ListAll(fetchLatest bool) ([]ServiceStatus, error) {
	driversLock.RLock()
	defer driversLock.RUnlock()

	var list []ServiceStatus
	for _, d := range drivers {
		status, err := d.Status()
		if err != nil {
			status = ServiceStatus{
				Name:          d.Name(),
				DisplayName:   d.DisplayName(),
				Description:   d.Description(),
				Category:      d.Category(),
				RunningStatus: "error",
			}
		}

		// 填充最新版本（优先读缓存）
		latest := GetCachedLatestVersion(d.Name())
		if latest == "" && fetchLatest {
			if rel, err := d.GetLatestRelease(); err == nil {
				SetCachedLatestVersion(d.Name(), rel)
				latest = rel
			}
		}
		status.LatestVersion = latest

		// 判断是否有新版本
		if status.Installed && status.InstalledVersion != "" && latest != "" {
			cleanInstalled := strings.TrimPrefix(status.InstalledVersion, "v")
			cleanLatest := strings.TrimPrefix(latest, "v")
			if cleanInstalled != cleanLatest {
				status.HasUpdate = true
			}
		}

		list = append(list, status)
	}
	return list, nil
}

// GetCachedLatestVersion 获取缓存的最新版本
func GetCachedLatestVersion(name string) string {
	versionCacheLock.RLock()
	defer versionCacheLock.RUnlock()
	item, ok := versionCache[name]
	if ok && time.Since(item.FetchedAt) < 15*time.Minute {
		return item.Version
	}
	return ""
}

// SetCachedLatestVersion 设置最新版本缓存
func SetCachedLatestVersion(name, ver string) {
	versionCacheLock.Lock()
	defer versionCacheLock.Unlock()
	versionCache[name] = cacheItem{
		Version:   ver,
		FetchedAt: time.Now(),
	}
}

// InstallTask 安装任务对象
type InstallTask struct {
	ComponentName string    `json:"component_name"`
	Status        string    `json:"status"` // "running" | "success" | "failed"
	Logs          []string  `json:"logs"`
	Error         string    `json:"error,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	listeners     []chan string
	mu            sync.Mutex
}

func (t *InstallTask) AddLog(msg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	line := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)
	t.Logs = append(t.Logs, line)
	for _, ch := range t.listeners {
		select {
		case ch <- line:
		default:
		}
	}
}

func (t *InstallTask) Subscribe() chan string {
	t.mu.Lock()
	defer t.mu.Unlock()
	ch := make(chan string, 100)
	// 发送历史日志
	for _, line := range t.Logs {
		ch <- line
	}
	t.listeners = append(t.listeners, ch)
	return ch
}

func (t *InstallTask) Unsubscribe(ch chan string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, l := range t.listeners {
		if l == ch {
			t.listeners = append(t.listeners[:i], t.listeners[i+1:]...)
			close(ch)
			break
		}
	}
}

// StartInstallTask 启动一个组件的安装或升级任务
func StartInstallTask(name, version string) (*InstallTask, error) {
	drv, err := Get(name)
	if err != nil {
		return nil, err
	}

	tasksLock.Lock()
	currentTask, exists := installTasks[name]
	if exists && currentTask.Status == "running" {
		tasksLock.Unlock()
		return currentTask, nil
	}

	task := &InstallTask{
		ComponentName: name,
		Status:        "running",
		Logs:          []string{},
		StartedAt:     time.Now(),
	}
	installTasks[name] = task
	tasksLock.Unlock()

	go func() {
		task.AddLog(fmt.Sprintf("开始执行 %s (%s) 安装/升级流程...", drv.DisplayName(), name))
		err := drv.InstallOrUpgrade(version, func(logMsg string) {
			task.AddLog(logMsg)
		})

		task.mu.Lock()
		defer task.mu.Unlock()
		if err != nil {
			task.Status = "failed"
			task.Error = err.Error()
			failMsg := fmt.Sprintf("[%s] [ERROR] 安装失败: %v", time.Now().Format("15:04:05"), err)
			task.Logs = append(task.Logs, failMsg)
			for _, ch := range task.listeners {
				select {
				case ch <- failMsg:
				default:
				}
			}
		} else {
			task.Status = "success"
			successMsg := fmt.Sprintf("[%s] [SUCCESS] %s 安装/升级完成并已成功启动！", time.Now().Format("15:04:05"), drv.DisplayName())
			task.Logs = append(task.Logs, successMsg)
			for _, ch := range task.listeners {
				select {
				case ch <- successMsg:
				default:
				}
			}
		}
	}()

	return task, nil
}

// GetInstallTask 获取当前组件的安装任务
func GetInstallTask(name string) *InstallTask {
	tasksLock.RLock()
	defer tasksLock.RUnlock()
	return installTasks[name]
}
