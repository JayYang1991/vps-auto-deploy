package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/model"
)

func setupRouteTestDB(t *testing.T) string {
	tmpDB, err := os.CreateTemp("", "route_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	dbPath := tmpDB.Name()
	tmpDB.Close()

	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("failed to init test db: %v", err)
	}
	model.DB = db
	return dbPath
}

func TestRouteCRUDAndReorder(t *testing.T) {
	os.Setenv("SKIP_SERVICE_RELOAD", "1")
	defer os.Unsetenv("SKIP_SERVICE_RELOAD")

	gin.SetMode(gin.TestMode)
	dbPath := setupRouteTestDB(t)
	defer os.Remove(dbPath)

	r := gin.New()
	r.GET("/routes", ListRoutes)
	r.POST("/routes", CreateRoute)
	r.GET("/routes/:id", GetRoute)
	r.PUT("/routes/:id", UpdateRoute)
	r.POST("/routes/:id/toggle", ToggleRoute)
	r.DELETE("/routes/:id", DeleteRoute)
	r.POST("/routes/reorder", ReorderRoutes)

	// 1. Initial list should contain default route rules
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/routes", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var list []model.RouteRule
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) < 2 {
		t.Fatalf("expected at least 2 default routes, got %d", len(list))
	}

	// 2. Create new route rule
	newRule := RouteRequest{
		Name:         "国内直连测试",
		Action:       "route",
		OutboundTag:  "direct",
		RuleSet:      "geosite-cn,geoip-cn",
		Priority:     10,
		Enabled:      true,
		DomainSuffix: "baidu.com,qq.com",
	}
	body, _ := json.Marshal(newRule)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/routes", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create route failed: %d, body: %s", w.Code, w.Body.String())
	}
	var created model.RouteRule
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if created.ID == 0 || created.Name != "国内直连测试" {
		t.Fatalf("unexpected created rule: %+v", created)
	}

	// 3. Toggle route
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, fmt.Sprintf("/routes/%d/toggle", created.ID), nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("toggle route failed: %d, body: %s", w.Code, w.Body.String())
	}

	// 4. Update route
	created.Name = "国内直连更新版"
	created.RuleSet = "geosite-cn"
	updateBody, _ := json.Marshal(created)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPut, fmt.Sprintf("/routes/%d", created.ID), bytes.NewBuffer(updateBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update route failed: %d, body: %s", w.Code, w.Body.String())
	}

	// 5. Reorder
	reorderReq := ReorderRequest{
		IDs: []int64{created.ID, list[0].ID},
	}
	reorderBody, _ := json.Marshal(reorderReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/routes/reorder", bytes.NewBuffer(reorderBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("reorder route failed: %d, body: %s", w.Code, w.Body.String())
	}

	// 6. Delete route
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodDelete, fmt.Sprintf("/routes/%d", created.ID), nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete route failed: %d, body: %s", w.Code, w.Body.String())
	}
}

func TestRouteRuleSetURLsValidation(t *testing.T) {
	os.Setenv("SKIP_SERVICE_RELOAD", "1")
	defer os.Unsetenv("SKIP_SERVICE_RELOAD")

	gin.SetMode(gin.TestMode)
	dbPath := setupRouteTestDB(t)
	defer os.Remove(dbPath)

	r := gin.New()
	r.POST("/routes", CreateRoute)

	// Case 1: SSRF loopback URL should be rejected with 400
	ssrfRule := RouteRequest{
		Name:        "SSRF测试规则",
		OutboundTag: "direct",
		RuleSetURLs: "malicious=http://127.0.0.1:8080/srs",
	}
	body, _ := json.Marshal(ssrfRule)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/routes", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for loopback SSRF URL, got %d", w.Code)
	}

	// Case 2: Cloud metadata SSRF URL should be rejected with 400
	metaRule := RouteRequest{
		Name:        "Metadata测试规则",
		OutboundTag: "direct",
		RuleSetURLs: "meta=http://169.254.169.254/latest/meta-data",
	}
	body, _ = json.Marshal(metaRule)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/routes", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for cloud metadata SSRF URL, got %d", w.Code)
	}

	// Case 3: Empty name should be rejected with 400
	emptyNameRule := RouteRequest{
		Name:        "   ",
		OutboundTag: "direct",
	}
	body, _ = json.Marshal(emptyNameRule)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/routes", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty name, got %d", w.Code)
	}

	// Case 4: Valid public URL should pass
	validRule := RouteRequest{
		Name:        "合法规则集",
		OutboundTag: "direct",
		RuleSetURLs: "my-rules=https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-cn.srs",
	}
	body, _ = json.Marshal(validRule)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/routes", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid public URL, got %d: %s", w.Code, w.Body.String())
	}
}
