package newapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
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

func TestReverseClientCheckAdminChannelListWithBearerLogin(t *testing.T) {
	var logoutCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			http.SetCookie(w, &http.Cookie{Name: "refresh_token", Value: "modern-refresh-token", Path: "/"})
			writeReverseClientTestJSON(t, w, map[string]any{
				"success": true,
				"message": "",
				"data": map[string]any{
					"access_token": "modern-access-token",
					"token_type":   "Bearer",
					"user": map[string]any{
						"id":   21,
						"role": 10,
					},
					"session": map[string]any{
						"sid": "modern-session-id",
					},
				},
			})
		case "/api/channel/":
			if got := r.Header.Get("New-Api-User"); got != "21" {
				t.Fatalf("New-Api-User = %q, want 21", got)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer modern-access-token" {
				t.Fatalf("Authorization = %q, want bearer token", got)
			}
			writeReverseClientTestJSON(t, w, map[string]any{
				"success": true,
				"message": "",
				"data": map[string]any{
					"items": []any{},
					"total": 3,
				},
			})
		case "/api/user/auth/logout":
			if r.Method != http.MethodPost {
				t.Fatalf("logout method = %s, want POST", r.Method)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer modern-access-token" {
				t.Fatalf("logout Authorization = %q, want bearer token", got)
			}
			if got := r.Header.Get("X-Auth-Session"); got != "modern-session-id" {
				t.Fatalf("X-Auth-Session = %q, want modern-session-id", got)
			}
			if cookie, err := r.Cookie("refresh_token"); err != nil || cookie.Value != "modern-refresh-token" {
				t.Fatalf("logout refresh cookie = %v, %v", cookie, err)
			}
			logoutCalls.Add(1)
			writeReverseClientTestJSON(t, w, map[string]any{
				"success": true,
				"message": "",
				"data": map[string]any{
					"revoked_sid":    "modern-session-id",
					"cookie_cleared": true,
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
	if result.UserID != 21 || result.Role != 10 || result.ChannelTotal != 3 {
		t.Fatalf("result = %+v", result)
	}
	if err := client.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := logoutCalls.Load(); got != 1 {
		t.Fatalf("logout calls = %d, want 1", got)
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
						{"id": 41, "name": "KeyHub openai sk-test"},
						{"id": 44, "name": "KeyHub openai sk-test"},
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
						{"id": 44, "name": "KeyHub openai sk-test", "used_quota": 12345, "status": 1},
						{"id": 45, "name": "KeyHub openai sk-next", "used_quota": 67890, "status": 3},
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
	if items[0].ChannelID != 44 ||
		items[0].UsedQuota != 12345 ||
		items[0].Status != ChannelStatusEnabled ||
		items[1].ChannelID != 45 ||
		items[1].UsedQuota != 67890 ||
		items[1].Status != ChannelStatusAutoDisabled {
		t.Fatalf("items = %+v", items)
	}
}

func TestReverseClientReusesLoginAcrossUsageRequests(t *testing.T) {
	var loginCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			loginCount.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "cached", Path: "/"})
			writeReverseClientTestJSON(t, w, map[string]any{
				"success": true,
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
			if err != nil || cookie.Value != "cached" {
				t.Fatalf("session cookie = %v, %v", cookie, err)
			}
			writeReverseClientTestJSON(t, w, map[string]any{
				"success": true,
				"data": map[string]any{
					"items": []map[string]any{{"id": 44, "used_quota": 12345}},
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
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := client.ListChannelUsage(context.Background()); err != nil {
			t.Fatalf("ListChannelUsage() attempt %d error = %v", attempt+1, err)
		}
	}
	if got := loginCount.Load(); got != 1 {
		t.Fatalf("login count = %d, want 1", got)
	}
}

func TestReverseClientReloginsAndRetriesAfterSessionExpires(t *testing.T) {
	var loginCount atomic.Int32
	var channelRequestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			loginNumber := loginCount.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "session", Value: strconv.Itoa(int(loginNumber)), Path: "/"})
			writeReverseClientTestJSON(t, w, map[string]any{
				"success": true,
				"data": map[string]any{
					"id":   12,
					"role": 10,
				},
			})
		case "/api/channel/":
			requestNumber := channelRequestCount.Add(1)
			cookie, err := r.Cookie("session")
			if err != nil {
				t.Fatalf("session cookie error = %v", err)
			}
			if requestNumber == 2 && cookie.Value == "1" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if requestNumber == 3 && cookie.Value != "2" {
				t.Fatalf("retried session cookie = %q, want 2", cookie.Value)
			}
			writeReverseClientTestJSON(t, w, map[string]any{
				"success": true,
				"data": map[string]any{
					"items": []map[string]any{{"id": 44, "used_quota": 12345}},
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
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := client.ListChannelUsage(context.Background()); err != nil {
			t.Fatalf("ListChannelUsage() attempt %d error = %v", attempt+1, err)
		}
	}
	if got := loginCount.Load(); got != 2 {
		t.Fatalf("login count = %d, want 2", got)
	}
	if got := channelRequestCount.Load(); got != 3 {
		t.Fatalf("channel request count = %d, want 3", got)
	}
}

func TestReverseClientSerializesConcurrentInitialLogin(t *testing.T) {
	var loginCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			loginCount.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "cached", Path: "/"})
			writeReverseClientTestJSON(t, w, map[string]any{
				"success": true,
				"data": map[string]any{
					"id":   12,
					"role": 10,
				},
			})
		case "/api/channel/":
			writeReverseClientTestJSON(t, w, map[string]any{
				"success": true,
				"data": map[string]any{
					"items": []map[string]any{{"id": 44, "used_quota": 12345}},
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
	var waitGroup sync.WaitGroup
	errorsByRequest := make(chan error, 8)
	for request := 0; request < 8; request++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			_, err := client.ListChannelUsage(context.Background())
			errorsByRequest <- err
		}()
	}
	waitGroup.Wait()
	close(errorsByRequest)
	for err := range errorsByRequest {
		if err != nil {
			t.Fatalf("ListChannelUsage() error = %v", err)
		}
	}
	if got := loginCount.Load(); got != 1 {
		t.Fatalf("login count = %d, want 1", got)
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

func TestReverseClientUsageLogsAndStats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "ok", Path: "/"})
			writeReverseClientTestJSON(t, w, map[string]any{
				"success": true,
				"message": "",
				"data":    map[string]any{"id": 12, "role": 10},
			})
		case "/api/log/":
			assertReverseClientAuth(t, r)
			query := r.URL.Query()
			if query.Get("p") != "2" ||
				query.Get("page_size") != "25" ||
				query.Get("type") != "5" ||
				query.Get("channel") != "44" ||
				query.Get("model_name") != "claude-opus-4-8" ||
				query.Get("request_id") != "req-1" {
				t.Fatalf("usage log query = %s", r.URL.RawQuery)
			}
			writeReverseClientTestJSON(t, w, map[string]any{
				"success": true,
				"message": "",
				"data": map[string]any{
					"items": []map[string]any{{
						"id":                9,
						"created_at":        123,
						"type":              5,
						"model_name":        "claude-opus-4-8",
						"channel":           44,
						"prompt_tokens":     12,
						"completion_tokens": 3,
					}},
					"total":     31,
					"page":      2,
					"page_size": 25,
				},
			})
		case "/api/log/stat":
			assertReverseClientAuth(t, r)
			if r.URL.Query().Get("channel") != "44" || r.URL.Query().Get("p") != "" {
				t.Fatalf("usage log stats query = %s", r.URL.RawQuery)
			}
			writeReverseClientTestJSON(t, w, map[string]any{
				"success": true,
				"message": "",
				"data":    map[string]any{"quota": 500000, "rpm": 7, "tpm": 321},
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
	query := UsageLogQuery{
		Page:      2,
		PageSize:  25,
		Type:      5,
		ModelName: "claude-opus-4-8",
		ChannelID: 44,
		RequestID: "req-1",
	}
	page, err := client.ListUsageLogs(context.Background(), query)
	if err != nil {
		t.Fatalf("ListUsageLogs() error = %v", err)
	}
	if page.Total != 31 || len(page.Items) != 1 || page.Items[0].ChannelID != 44 {
		t.Fatalf("ListUsageLogs() = %+v", page)
	}
	stats, err := client.GetUsageLogStats(context.Background(), query)
	if err != nil {
		t.Fatalf("GetUsageLogStats() error = %v", err)
	}
	if stats.Quota != 500000 || stats.RPM != 7 || stats.TPM != 321 {
		t.Fatalf("GetUsageLogStats() = %+v", stats)
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
