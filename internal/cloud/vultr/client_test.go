package vultr

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVultrClientMock(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-api-key" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"Invalid API Key"}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/account":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"account": {
					"balance": -25.50,
					"pending_charges": 4.10,
					"email": "test@example.com",
					"name": "Dev User"
				}
			}`))
		case "/regions":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"regions": [
					{"id": "nrt", "city": "Tokyo", "country": "JP", "continent": "Asia"},
					{"id": "lax", "city": "Los Angeles", "country": "US", "continent": "North America"}
				]
			}`))
		case "/plans":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"plans": [
					{"id": "vc2-1c-1gb", "vcpu_count": 1, "ram": 1024, "disk": 25, "monthly_cost": 5, "locations": ["nrt", "lax"]}
				]
			}`))
		case "/os":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"os": [
					{"id": 2136, "name": "Debian 12 x64", "family": "debian", "arch": "x64"},
					{"id": 124, "name": "Windows 2022", "family": "windows", "arch": "x64"},
					{"id": 400, "name": "FreeBSD 14", "family": "freebsd", "arch": "x64"}
				]
			}`))
		case "/ssh-keys":
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"ssh_keys": []}`))
			} else if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"ssh_key": {"id": "key-uuid-123", "name": "test-key"}}`))
			}
		case "/instances":
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{
					"instances": [
						{
							"id": "inst-uuid-456",
							"main_ip": "149.28.10.20",
							"status": "active",
							"power_status": "running",
							"label": "my-test-vps",
							"region": "nrt",
							"plan": "vc2-1c-1gb"
						}
					]
				}`))
			} else if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte(`{
					"instance": {
						"id": "inst-uuid-456",
						"main_ip": "0.0.0.0",
						"status": "pending",
						"region": "nrt",
						"plan": "vc2-1c-1gb"
					}
				}`))
			}
		case "/instances/inst-uuid-456":
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{
					"instance": {
						"id": "inst-uuid-456",
						"main_ip": "149.28.10.20",
						"status": "active",
						"power_status": "running",
						"default_password": "InitialRootPassword@888"
					}
				}`))
			} else if r.Method == http.MethodDelete {
				w.WriteHeader(http.StatusNoContent)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	client := NewClient("test-api-key")
	client.baseURL = mockServer.URL

	// 1. GetAccount
	acc, err := client.GetAccount()
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if acc.Email != "test@example.com" || acc.Balance != -25.50 {
		t.Fatalf("Unexpected account: %+v", acc)
	}

	// 2. ListRegions
	regions, err := client.ListRegions(true)
	if err != nil || len(regions) != 2 {
		t.Fatalf("ListRegions failed: %v, count: %d", err, len(regions))
	}

	// 3. ListPlans
	plans, err := client.ListPlans(true)
	if err != nil || len(plans) != 1 {
		t.Fatalf("ListPlans failed: %v", err)
	}

	// 4. ListOSList (verifying Windows & FreeBSD are filtered out)
	osList, err := client.ListOSList(true)
	if err != nil {
		t.Fatalf("ListOSList failed: %v", err)
	}
	if len(osList) != 1 || osList[0].Family != "debian" {
		t.Fatalf("Expected only Debian in filtered OS list, got: %+v", osList)
	}

	// 5. EnsureSSHKey
	keyID, err := client.EnsureSSHKey("master-key", "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIMockKey123")
	if err != nil || keyID != "key-uuid-123" {
		t.Fatalf("EnsureSSHKey failed: %v, keyID: %s", err, keyID)
	}

	// 6. CreateInstance
	inst, err := client.CreateInstance(CreateInstanceRequest{
		Region:     "nrt",
		Plan:       "vc2-1c-1gb",
		OSID:       2136,
		Label:      "test-instance",
		SSHKeyIDs:  []string{keyID},
		EnableIPv6: true,
	})
	if err != nil || inst.ID != "inst-uuid-456" {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	// 7. GetInstance
	polled, err := client.GetInstance(inst.ID)
	if err != nil || polled.MainIP != "149.28.10.20" || polled.Status != "active" {
		t.Fatalf("GetInstance failed: %v, polled: %+v", err, polled)
	}

	// 8. ListInstances
	allInsts, err := client.ListInstances()
	if err != nil || len(allInsts) != 1 || allInsts[0].ID != "inst-uuid-456" {
		t.Fatalf("ListInstances failed: %v, allInsts: %+v", err, allInsts)
	}

	// 9. DeleteInstance
	err = client.DeleteInstance(inst.ID)
	if err != nil {
		t.Fatalf("DeleteInstance failed: %v", err)
	}
}
