package tokensupply

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientAddAPIKeySendsProviderUserHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode login payload: %v", err)
			}
			if payload["username"] != "chengguanru" || payload["password"] != "secret" {
				t.Fatalf("login payload = %#v", payload)
			}
			writeTestJSON(t, w, map[string]any{
				"success": true,
				"data": map[string]any{
					"username":          "chengguanru",
					"purchase_provider": "test",
				},
			})
		case "/api/apikey/add":
			if r.Header.Get("x-provider-user") != "chengguanru" {
				t.Fatalf("x-provider-user = %q, want chengguanru", r.Header.Get("x-provider-user"))
			}
			var payload AddAPIKeyRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode add payload: %v", err)
			}
			if payload.APIKey != "sk-test" || payload.ProviderSlug != "anthropic" || payload.HasQuotaLimit != 0 || payload.BizDate == "" {
				t.Fatalf("add payload = %+v", payload)
			}
			writeTestJSON(t, w, map[string]any{
				"success": true,
				"data": map[string]any{
					"id":            "2627PAKrPECMI14613669",
					"provider_slug": "anthropic",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "chengguanru", "secret")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	record, err := client.AddAPIKey(context.Background(), AddAPIKeyRequest{
		APIKey:       "sk-test",
		ProviderSlug: "anthropic",
		BizDate:      "20260705",
	})
	if err != nil {
		t.Fatalf("AddAPIKey() error = %v", err)
	}
	if record.ID != "2627PAKrPECMI14613669" {
		t.Fatalf("record.ID = %q", record.ID)
	}
}

func TestClientListUsageConvertsActualCostToQuota(t *testing.T) {
	remoteID := "2627PAKrPECMI14613669"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			writeTestJSON(t, w, map[string]any{
				"success": true,
				"data": map[string]any{
					"username": "chengguanru",
				},
			})
		case "/api/apikey/list":
			if r.Header.Get("x-provider-user") != "chengguanru" {
				t.Fatalf("x-provider-user = %q, want chengguanru", r.Header.Get("x-provider-user"))
			}
			writeTestJSON(t, w, map[string]any{
				"success": true,
				"data": []map[string]any{
					{"id": remoteID, "provider_slug": "anthropic", "actual_cost": "1.25"},
					{"id": "zero", "provider_slug": "openai", "actual_cost": nil},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "chengguanru", "secret")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	items, err := client.ListUsage(context.Background(), 500000)
	if err != nil {
		t.Fatalf("ListUsage() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].RemoteID != RemoteIDNumber(remoteID) || items[0].UsedQuota != 625000 {
		t.Fatalf("items[0] = %+v", items[0])
	}
	if items[1].UsedQuota != 0 {
		t.Fatalf("items[1].UsedQuota = %d, want 0", items[1].UsedQuota)
	}
}

func TestClientDeleteAPIKeyByRemoteIDFindsStringID(t *testing.T) {
	remoteID := "abc123"
	var deletedID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			writeTestJSON(t, w, map[string]any{
				"success": true,
				"data": map[string]any{
					"username": "chengguanru",
				},
			})
		case "/api/apikey/list":
			writeTestJSON(t, w, map[string]any{
				"success": true,
				"data": []map[string]any{
					{"id": remoteID, "provider_slug": "anthropic"},
				},
			})
		case "/api/apikey/delete":
			if r.Header.Get("x-provider-user") != "chengguanru" {
				t.Fatalf("x-provider-user = %q, want chengguanru", r.Header.Get("x-provider-user"))
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode delete payload: %v", err)
			}
			deletedID = payload["id"]
			writeTestJSON(t, w, map[string]any{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "chengguanru", "secret")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if err := client.DeleteAPIKeyByRemoteID(context.Background(), RemoteIDNumber(remoteID)); err != nil {
		t.Fatalf("DeleteAPIKeyByRemoteID() error = %v", err)
	}
	if deletedID != remoteID {
		t.Fatalf("deletedID = %q, want %q", deletedID, remoteID)
	}
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
