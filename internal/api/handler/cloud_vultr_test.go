package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/model"
)

func TestCloudVultrHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestDB(t)

	r := gin.New()
	r.GET("/api/nodes/cloud/vultr/config", GetVultrConfig)
	r.POST("/api/nodes/cloud/vultr/config", SaveVultrConfig)
	r.GET("/api/nodes/cloud/vultr/catalog", GetVultrCatalog)
	r.GET("/api/nodes/cloud/vultr/instances", ListVultrInstances)
	r.POST("/api/nodes/cloud/vultr/provision", ProvisionVultrNode)

	// 1. GetVultrConfig (Initially unconfigured)
	{
		req, _ := http.NewRequest(http.MethodGet, "/api/nodes/cloud/vultr/config", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d", w.Code)
		}
		var res struct {
			Configured bool   `json:"configured"`
			MaskedKey  string `json:"masked_key"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &res)
		if res.Configured {
			t.Fatalf("Expected configured=false initially")
		}
	}

	// 2. SaveVultrConfig with invalid token (should return 400 because remote validation fails)
	{
		body, _ := json.Marshal(map[string]string{
			"api_key": "invalid-dummy-token-12345",
		})
		req, _ := http.NewRequest(http.MethodPost, "/api/nodes/cloud/vultr/config", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("Expected 400 Bad Request with invalid token, got %d: %s", w.Code, w.Body.String())
		}
	}

	// 3. Manually save encrypted token in DB to test GetVultrConfig masked output
	{
		_ = model.SetVultrAPIKey("VULTR_MOCK_SECRET_KEY_9876543210")
		req, _ := http.NewRequest(http.MethodGet, "/api/nodes/cloud/vultr/config", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d", w.Code)
		}
		var res struct {
			Configured bool   `json:"configured"`
			MaskedKey  string `json:"masked_key"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &res)
		if !res.Configured || res.MaskedKey != "VULTR••••••••3210" {
			t.Fatalf("Unexpected config response: %+v", res)
		}
	}

	// 4. ProvisionVultrNode with missing fields -> 400 Bad Request
	{
		body, _ := json.Marshal(map[string]any{
			"region": "",
			"plan":   "",
		})
		req, _ := http.NewRequest(http.MethodPost, "/api/nodes/cloud/vultr/provision", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("Expected 400 Bad Request for empty region, got %d", w.Code)
		}
	}

	// 5. ProvisionVultrNode with instance_id -> 200 OK (accepts existing instance onboarding)
	{
		body, _ := json.Marshal(map[string]any{
			"instance_id": "inst-test-existing-12345",
			"name":        "Test Existing",
		})
		req, _ := http.NewRequest(http.MethodPost, "/api/nodes/cloud/vultr/provision", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK for instance_id provision, got %d: %s", w.Code, w.Body.String())
		}
		var res struct {
			TaskID string `json:"task_id"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &res)
		if res.TaskID == "" {
			t.Fatalf("Expected non-empty task_id")
		}
	}
}

