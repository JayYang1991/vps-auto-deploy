package model

import (
	"fmt"
	"strings"
	"time"
)

// CFPreferredIP 存储 Cloudflare 优选节点/IP 信息
type CFPreferredIP struct {
	ID        int64     `json:"id"`
	IP        string    `json:"ip"`
	Port      int       `json:"port"`
	Tag       string    `json:"tag"`
	Name      string    `json:"name"`
	Colo      string    `json:"colo"`
	Latency   string    `json:"latency"`
	Speed     string    `json:"speed"`
	RawLink   string    `json:"raw_link"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListCFPreferredIPs 检索本地已持久化的 Cloudflare 优选 IP 列表
func ListCFPreferredIPs(keyword string) ([]CFPreferredIP, error) {
	query := "SELECT id, ip, port, tag, name, colo, latency, speed, raw_link, created_at, updated_at FROM cf_preferred_ips"
	var args []interface{}

	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		query += " WHERE ip LIKE ? OR tag LIKE ? OR name LIKE ? OR colo LIKE ?"
		likeArg := "%" + keyword + "%"
		args = append(args, likeArg, likeArg, likeArg, likeArg)
	}

	query += " ORDER BY id ASC"

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query cf_preferred_ips: %w", err)
	}
	defer rows.Close()

	var list []CFPreferredIP
	for rows.Next() {
		var item CFPreferredIP
		if err := rows.Scan(
			&item.ID,
			&item.IP,
			&item.Port,
			&item.Tag,
			&item.Name,
			&item.Colo,
			&item.Latency,
			&item.Speed,
			&item.RawLink,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, item)
	}

	if list == nil {
		list = []CFPreferredIP{}
	}
	return list, nil
}

// SaveCFPreferredIPs 在单事务中原子替换或保存最新抓取到的优选 IP 列表
func SaveCFPreferredIPs(ips []CFPreferredIP) error {
	tx, err := DB.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 清空历史数据
	if _, err := tx.Exec("DELETE FROM cf_preferred_ips"); err != nil {
		return fmt.Errorf("clear old cf_preferred_ips: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO cf_preferred_ips (ip, port, tag, name, colo, latency, speed, raw_link, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now()
	for _, item := range ips {
		port := item.Port
		if port <= 0 {
			port = 443
		}
		_, err := stmt.Exec(
			item.IP,
			port,
			item.Tag,
			item.Name,
			item.Colo,
			item.Latency,
			item.Speed,
			item.RawLink,
			now,
			now,
		)
		if err != nil {
			return fmt.Errorf("insert cf_preferred_ip (%s): %w", item.IP, err)
		}
	}

	return tx.Commit()
}

// CountCFPreferredIPs 统计已持久化的优选 IP 数量
func CountCFPreferredIPs() (int, error) {
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM cf_preferred_ips").Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// ClearCFPreferredIPs 清空本地优选 IP 列表
func ClearCFPreferredIPs() error {
	_, err := DB.Exec("DELETE FROM cf_preferred_ips")
	return err
}

// DeleteCFPreferredIP 删除单条优选 IP
func DeleteCFPreferredIP(id int64) error {
	_, err := DB.Exec("DELETE FROM cf_preferred_ips WHERE id = ?", id)
	return err
}

