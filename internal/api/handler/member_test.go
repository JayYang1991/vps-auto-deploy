package handler

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/node"
)

func TestListMembersHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestDB(t)

	r := gin.New()
	r.GET("/api/members", ListMembers)

	req := httptest.NewRequest(http.MethodGet, "/api/members", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	t.Logf("Status: %d, Body: %s", w.Code, w.Body.String())
}

func TestSyncMembersHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestDB(t)

	now := time.Now()
	r := gin.New()
	r.POST("/api/members/sync", SyncMembers)

	// 2. 构造来自主控端的新成员列表
	syncList := []model.Member{
		{
			ID:        10,
			Name:      "Master-User-A",
			UUID:      "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			SubToken:  "token-aaa",
			Enabled:   true,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        11,
			Name:      "Master-User-B",
			UUID:      "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
			SubToken:  "token-bbb",
			Enabled:   false,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	body, _ := json.Marshal(syncList)
	req := httptest.NewRequest(http.MethodPost, "/api/members/sync", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", w.Code, w.Body.String())
	}

	// 3. 校验本地成员表：本地原有的用户得到保留，且主控的新成员已成功合并
	members, err := model.ListMembers()
	if err != nil {
		t.Fatalf("ListMembers failed: %v", err)
	}

	// 初始默认包含 1 个 owner，同步 2 个新成员后应为 3 个成员
	if len(members) != 3 {
		t.Fatalf("expected 3 members after sync (1 local preserved + 2 from master), got %d", len(members))
	}

	// 查找并校验本地原有成员与同步的新成员
	var foundOwner, foundA, foundB bool
	for _, m := range members {
		if m.Name == "Default Owner" {
			foundOwner = true
		}
		if m.Name == "Master-User-A" && m.UUID == "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" && m.Enabled {
			foundA = true
		}
		if m.Name == "Master-User-B" && m.UUID == "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb" && !m.Enabled {
			foundB = true
		}
	}

	if !foundOwner {
		t.Fatalf("local owner member was unexpectedly removed")
	}
	if !foundA || !foundB {
		t.Fatalf("synced master members missing or mismatched: %+v", members)
	}
}

func TestSyncMembersHandlerReplaceMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestDB(t)

	now := time.Now()
	r := gin.New()
	r.POST("/api/members/sync", SyncMembers)

	// 初始默认包含 Default Owner
	// 构造 replace 列表，不包含 Default Owner
	syncList := []model.Member{
		{
			ID:        100,
			Name:      "Clean-Master-User",
			UUID:      "cccccccc-cccc-cccc-cccc-cccccccccccc",
			SubToken:  "token-clean",
			Enabled:   true,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	body, _ := json.Marshal(syncList)
	req := httptest.NewRequest(http.MethodPost, "/api/members/sync?mode=replace", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", w.Code, w.Body.String())
	}

	members, err := model.ListMembers()
	if err != nil {
		t.Fatalf("ListMembers failed: %v", err)
	}

	// 在 replace 模式下，本地残留的 Default Owner 必须被彻底清除，仅保留主控推过来的 1 个成员
	if len(members) != 1 {
		t.Fatalf("expected exactly 1 member after replace sync, got %d: %+v", len(members), members)
	}
	if members[0].Name != "Clean-Master-User" {
		t.Fatalf("expected Clean-Master-User, got %s", members[0].Name)
	}
}

func TestBroadcastSyncMembersHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestDB(t)

	r := gin.New()
	r.POST("/api/members/broadcast-sync", BroadcastSyncMembers)

	req := httptest.NewRequest(http.MethodPost, "/api/members/broadcast-sync", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", w.Code, w.Body.String())
	}

	var res map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res["message"] != "全网凭据广播同步完成" {
		t.Fatalf("unexpected message: %v", res["message"])
	}
}

func TestSyncMembersToNodeMock(t *testing.T) {
	// 创建 mock 远端受管节点接收端
	var receivedMembers []model.Member
	var receivedAuthHeader string

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/testsecret/api/members/sync" {
			http.NotFound(w, r)
			return
		}
		receivedAuthHeader = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&receivedMembers); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"synced","count":2}`))
	}))
	defer mockServer.Close()

	// 从 mockServer.URL 解析 host 与 port
	host, portStr, err := net.SplitHostPort(mockServer.Listener.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort failed: %v", err)
	}
	port, _ := strconv.Atoi(portStr)

	dummyNode := &model.Node{
		ID:         2,
		Name:       "Mock-Worker-01",
		Host:       host,
		PanelPort:  port,
		SecretPath: "testsecret",
		APIToken:   "test-token-jwt-999",
		IsMaster:   false,
		UseTLS:     false,
	}

	testMembers := []model.Member{
		{
			ID:       1,
			Name:     "Test-Alice",
			UUID:     "12345678-1234-1234-1234-123456789012",
			SubToken: "tok-alice",
			Enabled:  true,
		},
	}

	err = node.SyncMembersToNode(dummyNode, testMembers)
	if err != nil {
		t.Fatalf("SyncMembersToNode failed: %v", err)
	}

	if receivedAuthHeader != "Bearer test-token-jwt-999" {
		t.Fatalf("expected Auth header 'Bearer test-token-jwt-999', got %q", receivedAuthHeader)
	}

	if len(receivedMembers) != 1 || receivedMembers[0].Name != "Test-Alice" {
		t.Fatalf("received members mismatch: %+v", receivedMembers)
	}
}
