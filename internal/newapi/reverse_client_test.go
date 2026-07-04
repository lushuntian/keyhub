package newapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReverseClientCheckAdminChannelList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			if r.Method != http.MethodPost {
				t.Fatalf("login method = %s, want POST", r.Method)
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode login payload: %v", err)
			}
			if payload["username"] != "admin" || payload["password"] != "secret" {
				t.Fatalf("login payload = %#v", payload)
			}
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "ok", Path: "/"})
			writeReverseClientTestJSON(t, w, map[string]any{
				"success": true,
				"message": "",
				"data": map[string]any{
					"id":   12,
					"role": 10,
				},
			})
		case "/api/channel/":
			if r.Header.Get("New-Api-User") != "12" {
				t.Fatalf("New-Api-User = %q, want 12", r.Header.Get("New-Api-User"))
			}
			cookie, err := r.Cookie("session")
			if err != nil || cookie.Value != "ok" {
				t.Fatalf("session cookie = %v, %v", cookie, err)
			}
			writeReverseClientTestJSON(t, w, map[string]any{
				"success": true,
				"message": "",
				"data": map[string]any{
					"items": []any{},
					"total": 7,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewReverseClient(server.URL, "admin", "secret")
	if err != nil {
		t.Fatalf("NewReverseClient() error = %v", err)
	}
	result, err := client.CheckAdminChannelList(context.Background())
	if err != nil {
		t.Fatalf("CheckAdminChannelList() error = %v", err)
	}
	if result.UserID != 12 || result.Role != 10 || result.ChannelTotal != 7 {
		t.Fatalf("result = %+v", result)
	}
}

func TestReverseClientRejects2FALogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeReverseClientTestJSON(t, w, map[string]any{
			"success": true,
			"message": "需要二次验证",
			"data": map[string]any{
				"require_2fa": true,
			},
		})
	}))
	defer server.Close()

	client, err := NewReverseClient(server.URL, "admin", "secret")
	if err != nil {
		t.Fatalf("NewReverseClient() error = %v", err)
	}
	if _, err := client.CheckAdminChannelList(context.Background()); err == nil {
		t.Fatal("CheckAdminChannelList() error = nil, want 2FA error")
	}
}

func TestReverseClientCreateChannelSearchesIDWhenCreateReturnsNoData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "ok", Path: "/"})
			writeReverseClientTestJSON(t, w, map[string]any{
				"success": true,
				"message": "",
				"data": map[string]any{
					"id":   12,
					"role": 10,
				},
			})
		case "/api/channel/":
			if r.Method != http.MethodPost {
				t.Fatalf("create channel method = %s, want POST", r.Method)
			}
			assertReverseClientAuth(t, r)
			var payload AddChannelRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode create payload: %v", err)
			}
			if payload.Mode != "single" || payload.Channel == nil || payload.Channel.Name != "KeyHub openai sk-test" {
				t.Fatalf("create payload = %+v", payload)
			}
			writeReverseClientTestJSON(t, w, map[string]any{
				"success": true,
				"message": "",
			})
		case "/api/channel/search":
			assertReverseClientAuth(t, r)
			if r.URL.Query().Get("keyword") != "KeyHub openai sk-test" {
				t.Fatalf("keyword = %q, want channel name", r.URL.Query().Get("keyword"))
			}
			writeReverseClientTestJSON(t, w, map[string]any{
				"success": true,
				"message": "",
				"data": map[string]any{
					"items": []map[string]any{
						{"id": 44, "name": "KeyHub openai sk-test"},
					},
					"total": 1,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewReverseClient(server.URL, "admin", "secret")
	if err != nil {
		t.Fatalf("NewReverseClient() error = %v", err)
	}
	response, err := client.CreateChannel(context.Background(), ChannelPayload{Name: "KeyHub openai sk-test", Key: "sk-test"})
	if err != nil {
		t.Fatalf("CreateChannel() error = %v", err)
	}
	if response.ChannelID != 44 || response.Action != "created" {
		t.Fatalf("response = %+v", response)
	}
}

func TestReverseClientCreateChannelUsesReturnedID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "ok", Path: "/"})
			writeReverseClientTestJSON(t, w, map[string]any{
				"success": true,
				"message": "",
				"data": map[string]any{
					"id":   12,
					"role": 10,
				},
			})
		case "/api/channel/":
			assertReverseClientAuth(t, r)
			writeReverseClientTestJSON(t, w, map[string]any{
				"success": true,
				"message": "",
				"data": map[string]any{
					"channel": map[string]any{
						"id": 45,
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewReverseClient(server.URL, "admin", "secret")
	if err != nil {
		t.Fatalf("NewReverseClient() error = %v", err)
	}
	response, err := client.CreateChannel(context.Background(), ChannelPayload{Name: "direct id", Key: "sk-test"})
	if err != nil {
		t.Fatalf("CreateChannel() error = %v", err)
	}
	if response.ChannelID != 45 {
		t.Fatalf("ChannelID = %d, want 45", response.ChannelID)
	}
}

func TestReverseClientListChannelUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "ok", Path: "/"})
			writeReverseClientTestJSON(t, w, map[string]any{
				"success": true,
				"message": "",
				"data": map[string]any{
					"id":   12,
					"role": 10,
				},
			})
		case "/api/channel/":
			if r.Method != http.MethodGet {
				t.Fatalf("list channel method = %s, want GET", r.Method)
			}
			assertReverseClientAuth(t, r)
			if r.URL.Query().Get("p") != "1" || r.URL.Query().Get("page_size") != "500" {
				t.Fatalf("query = %s, want first usage page", r.URL.RawQuery)
			}
			writeReverseClientTestJSON(t, w, map[string]any{
				"success": true,
				"message": "",
				"data": map[string]any{
					"items": []map[string]any{
						{"id": 44, "name": "KeyHub openai sk-test", "used_quota": 12345},
						{"id": 45, "name": "KeyHub openai sk-next", "used_quota": 67890},
					},
					"total": 2,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewReverseClient(server.URL, "admin", "secret")
	if err != nil {
		t.Fatalf("NewReverseClient() error = %v", err)
	}
	items, err := client.ListChannelUsage(context.Background())
	if err != nil {
		t.Fatalf("ListChannelUsage() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].ChannelID != 44 || items[0].UsedQuota != 12345 || items[1].ChannelID != 45 || items[1].UsedQuota != 67890 {
		t.Fatalf("items = %+v", items)
	}
}

func TestReverseClientDisableChannel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "ok", Path: "/"})
			writeReverseClientTestJSON(t, w, map[string]any{
				"success": true,
				"message": "",
				"data": map[string]any{
					"id":   12,
					"role": 10,
				},
			})
		case "/api/channel/":
			if r.Method != http.MethodPut {
				t.Fatalf("disable channel method = %s, want PUT", r.Method)
			}
			assertReverseClientAuth(t, r)
			var payload ChannelPayload
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode disable payload: %v", err)
			}
			if payload.ID != 44 || payload.Status != ChannelStatusManuallyDisabled {
				t.Fatalf("disable payload = %+v", payload)
			}
			writeReverseClientTestJSON(t, w, map[string]any{
				"success": true,
				"message": "",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewReverseClient(server.URL, "admin", "secret")
	if err != nil {
		t.Fatalf("NewReverseClient() error = %v", err)
	}
	if err := client.DisableChannel(context.Background(), 44); err != nil {
		t.Fatalf("DisableChannel() error = %v", err)
	}
}

func assertReverseClientAuth(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Header.Get("New-Api-User") != "12" {
		t.Fatalf("New-Api-User = %q, want 12", r.Header.Get("New-Api-User"))
	}
	cookie, err := r.Cookie("session")
	if err != nil || cookie.Value != "ok" {
		t.Fatalf("session cookie = %v, %v", cookie, err)
	}
}

func writeReverseClientTestJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
