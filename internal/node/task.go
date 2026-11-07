package node

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// DeployTask 记录远程 SSH 部署任务状态及实时日志
type DeployTask struct {
	ID        string    `json:"id"`
	NodeName  string    `json:"node_name"`
	Host      string    `json:"host"`
	Status    string    `json:"status"` // "running" | "success" | "failed"
	Logs      []string  `json:"logs"`
	Error     string    `json:"error,omitempty"`
	NodeID    int64     `json:"node_id,omitempty"`
	StartedAt time.Time `json:"started_at"`

	listeners []chan string
	mu        sync.Mutex
}

var (
	tasksLock   sync.RWMutex
	deployTasks = make(map[string]*DeployTask)
)

// CreateDeployTask 创建并登记一个部署任务
func CreateDeployTask(nodeName, host string) *DeployTask {
	tasksLock.Lock()
	defer tasksLock.Unlock()

	id := "deploy-" + uuid.New().String()[:8]
	task := &DeployTask{
		ID:        id,
		NodeName:  nodeName,
		Host:      host,
		Status:    "running",
		Logs:      make([]string, 0, 50),
		StartedAt: time.Now(),
	}
	deployTasks[id] = task
	return task
}

// GetDeployTask 获取指定部署任务
func GetDeployTask(id string) (*DeployTask, error) {
	tasksLock.RLock()
	defer tasksLock.RUnlock()

	task, exists := deployTasks[id]
	if !exists {
		return nil, fmt.Errorf("deploy task %s not found", id)
	}
	return task, nil
}

// AddLog 向任务追加一行日志并广播给 SSE 监听者
func (t *DeployTask) AddLog(msg string) {
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

// Subscribe 订阅日志流
func (t *DeployTask) Subscribe() chan string {
	t.mu.Lock()
	defer t.mu.Unlock()

	ch := make(chan string, 100)
	for _, line := range t.Logs {
		ch <- line
	}
	t.listeners = append(t.listeners, ch)
	return ch
}

// Unsubscribe 取消订阅
func (t *DeployTask) Unsubscribe(ch chan string) {
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

