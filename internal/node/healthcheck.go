package node

import (
	"log"
	"sync"
	"time"

	"vps-auto-deploy/internal/model"
)

// StartHealthCheckWorker 启动后台心跳保活协程，定期检测所有受管节点健康状态与往返延迟
func StartHealthCheckWorker(interval time.Duration) {
	go func() {
		// 启动后延时 3 秒执行首轮检查，避开系统刚启动的高峰期
		time.Sleep(3 * time.Second)
		checkAllNodes()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			checkAllNodes()
		}
	}()
}

func checkAllNodes() {
	nodes, err := model.ListNodes()
	if err != nil || len(nodes) == 0 {
		return
	}

	var wg sync.WaitGroup
	for _, n := range nodes {
		if n.IsMaster {
			continue
		}

		nodeCopy := n
		wg.Add(1)
		go func(target model.Node) {
			defer wg.Done()
			_, pingErr := PingNode(&target)
			if pingErr != nil {
				log.Printf("[HealthCheck] Node %s (%s) ping failed: %v", target.Name, target.Host, pingErr)
			}
		}(nodeCopy)
	}
	wg.Wait()
}

