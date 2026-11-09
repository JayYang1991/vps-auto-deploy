package handler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/model"
)

func TestSystemOverviewWithVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dbPath := filepath.Join(t.TempDir(), "test_sys_overview.db")
	_, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("init db failed: %v", err)
	}

	r := gin.New()
	r.GET("/api/system/overview", GetSystemOverview)

	req, _ := http.NewRequest(http.MethodGet, "/api/system/overview", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var res SystemOverview
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal overview failed: %v", err)
	}

	if res.Version == "" {
		t.Errorf("expected non-empty version in overview")
	}
}

func TestUploadAndListUpgradePackage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/system/upgrade/package", UploadUpgradePackage)
	r.GET("/api/system/upgrade/packages", ListUpgradePackages)

	exe, err := os.Executable()
	if err != nil {
		t.Skip("unable to get executable")
	}
	exeData, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("package", "vps-panel")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(exeData); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()

	req, _ := http.NewRequest(http.MethodPost, "/api/system/upgrade/package", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on upload, got %d: %s", w.Code, w.Body.String())
	}

	var uploadResp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &uploadResp); err != nil {
		t.Fatal(err)
	}
	if uploadResp["package_id"] == "" || uploadResp["sha256"] == "" {
		t.Errorf("missing package_id or sha256 in response: %v", uploadResp)
	}

	// 验证列表
	reqList, _ := http.NewRequest(http.MethodGet, "/api/system/upgrade/packages", nil)
	wList := httptest.NewRecorder()
	r.ServeHTTP(wList, reqList)

	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200 on list, got %d", wList.Code)
	}
}

func TestStartUpgradeNodeAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dbPath := filepath.Join(t.TempDir(), "test_upgrade_api.db")
	_, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("init db failed: %v", err)
	}

	nodeID, err := model.CreateNode(&model.Node{
		Name:            "Upgrade-Test-Node",
		Host:            "127.0.0.1",
		SSHPort:         22,
		PanelPort:       20888,
		SecretPath:      "testsec",
		APIToken:        "dummy-token",
		CertFingerprint: "AA:BB:CC",
		Status:          "online",
		OSInfo:          "Linux (amd64)",
	})
	if err != nil {
		t.Fatalf("create node failed: %v", err)
	}

	r := gin.New()
	r.POST("/api/nodes/upgrade", StartUpgradeNode)

	// API 模式升级
	payload := map[string]interface{}{
		"node_id": nodeID,
		"mode":    "api",
		"force":   false,
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, "/api/nodes/upgrade", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var res map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res["task_id"] == "" {
		t.Errorf("expected task_id in response: %v", res)
	}
}
