package component

import (
	"fmt"
	"sync"
)

// ServiceStatus 组件服务状态
type ServiceStatus struct {
	Name             string `json:"name"`
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installed_version"`
	LatestVersion    string `json:"latest_version"`
	Active           bool   `json:"active"`
	RunningStatus    string `json:"running_status"` // "running", "failed", "stopped", "not_installed"
}

// ComponentDriver 受管组件生命周期抽象
type ComponentDriver interface {
	Name() string
	GetInstalledVersion() (string, error)
	GetLatestRelease() (string, error)
	InstallOrUpgrade(version string) error
	Status() (ServiceStatus, error)
	Start() error
	Stop() error
	Restart() error
}

var (
	driversLock sync.RWMutex
	drivers     = make(map[string]ComponentDriver)
)

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
func ListAll() ([]ServiceStatus, error) {
	driversLock.RLock()
	defer driversLock.RUnlock()

	var list []ServiceStatus
	for _, d := range drivers {
		status, err := d.Status()
		if err != nil {
			status = ServiceStatus{
				Name:          d.Name(),
				RunningStatus: "error",
			}
		}
		list = append(list, status)
	}
	return list, nil
}

