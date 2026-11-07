package subgen

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"vps-auto-deploy/internal/model"
)

// SubResult 订阅生成结果
type SubResult struct {
	ContentType string
	Filename    string
	Content     string
}

// GetMemberByToken 根据 sub_token 获取有效成员
func GetMemberByToken(token string) (*model.Member, error) {
	var member model.Member
	err := model.DB.QueryRow("SELECT id, name, uuid, sub_token, enabled FROM members WHERE sub_token = ?", token).
		Scan(&member.ID, &member.Name, &member.UUID, &member.SubToken, &member.Enabled)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("invalid subscription token")
	}
	if err != nil {
		return nil, err
	}

	if !member.Enabled {
		return nil, fmt.Errorf("member subscription is disabled")
	}

	return &member, nil
}

// GenerateSubscription 根据 token 与 client 类型渲染对应的订阅数据
func GenerateSubscription(token, target, userAgent string) (*SubResult, error) {
	member, err := GetMemberByToken(token)
	if err != nil {
		return nil, err
	}

	ua := strings.ToLower(userAgent)
	clientType := strings.ToLower(target)

	// 自动判断客户端类型
	if clientType == "" || clientType == "auto" {
		if strings.Contains(ua, "clash") || strings.Contains(ua, "mihomo") || strings.Contains(ua, "stash") {
			clientType = "clash"
		} else if strings.Contains(ua, "sing-box") || strings.Contains(ua, "sfi") || strings.Contains(ua, "sfa") {
			clientType = "singbox"
		} else {
			clientType = "clash" // 默认返回 Clash Meta 格式
		}
	}

	switch clientType {
	case "clash", "meta", "mihomo":
		yamlContent, err := GenerateClashMetaConfig(member)
		if err != nil {
			return nil, err
		}
		return &SubResult{
			ContentType: "application/yaml; charset=utf-8",
			Filename:    fmt.Sprintf("clash-%s.yaml", member.Name),
			Content:     yamlContent,
		}, nil

	case "singbox", "sing-box":
		jsonContent, err := GenerateSingboxClientConfig(member)
		if err != nil {
			return nil, err
		}
		return &SubResult{
			ContentType: "application/json; charset=utf-8",
			Filename:    fmt.Sprintf("singbox-%s.json", member.Name),
			Content:     jsonContent,
		}, nil

	case "base64", "uri", "v2ray":
		b64Content, err := GenerateBase64Sub(member)
		if err != nil {
			return nil, err
		}
		return &SubResult{
			ContentType: "text/plain; charset=utf-8",
			Filename:    fmt.Sprintf("sub-%s.txt", member.Name),
			Content:     b64Content,
		}, nil

	default:
		return nil, fmt.Errorf("unknown target format: %s", target)
	}
}

