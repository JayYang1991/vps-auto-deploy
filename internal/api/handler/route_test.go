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
