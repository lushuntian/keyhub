package httpserver

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"keyhub/internal/config"
	"keyhub/internal/database"
	"keyhub/internal/newapi"
	"keyhub/internal/security"
)

// Opt-in tests create and drop ONLY a newly generated test database.
func publicAPITestServer(t *testing.T) *Server {
	t.Helper()
	dsn := os.Getenv("KEYHUB_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set KEYHUB_TEST_MYSQL_DSN to an isolated MySQL with CREATE DATABASE permission")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DBName = ""
	cfg.ParseTime = true
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	name := "keyhub_api_test_" + hex.EncodeToString(raw)
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	cfg.DBName = name
	db, err := database.Open(context.Background(), cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); _, _ = admin.Exec("DROP DATABASE " + name); admin.Close() })
	if err := database.ApplyMigrations(context.Background(), db, "../../migrations", 500000); err != nil {
		t.Fatal(err)
	}
	return New(config.Config{AuthEnabled: true, WorkerEnabled: true, EncryptionKey: "api-integration-test-encryption-key", SessionTTL: time.Hour, StaticDir: "../../web/public"}, db)
}

func apiTestUser(t *testing.T, s *Server, name string) (*database.AdminUser, string) {
	t.Helper()
	user, err := database.CreateAdminUser(context.Background(), s.db, name, "test-password-only", name)
	if err != nil {
		t.Fatal(err)
	}
	session := "session-" + name
	if err := database.CreateAdminSession(context.Background(), s.db, security.SessionTokenHash(session), user.ID, time.Now().Add(time.Hour), "test", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	return user, session
}

func apiTestCall(s *Server, method, path, token, cookie, idempotency, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if cookie != "" {
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	}
	if idempotency != "" {
		request.Header.Set("Idempotency-Key", idempotency)
	}
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	return response
}

func apiTestToken(t *testing.T, s *Server, cookie string) (string, int64) {
	t.Helper()
	response := apiTestCall(s, "POST", "/api/account/api-tokens", "", cookie, "", `{"name":"test-script","expiresInDays":30}`)
	if response.Code != 201 {
		t.Fatalf("create token: %d %s", response.Code, response.Body.String())
	}
	var result struct {
		Token string
		ID    int64
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.Token, result.ID
}

func apiTestTarget(t *testing.T, s *Server, userID int64, code, mode, baseURL string) {
	t.Helper()
	encryptor, _ := security.NewEncryptor(s.config.EncryptionKey)
	encrypted, _ := encryptor.Encrypt("receiver-test-secret")
	err := database.SaveAggregationTarget(context.Background(), s.db, database.AggregationTargetUpsert{Code: code, Name: "private-" + code, BaseURL: baseURL, ConnectionMode: mode, ChannelType: "anthropic", Models: []string{"test-model"}, TokenCiphertext: encrypted, ReverseUsername: "receiver-user", ReversePasswordCiphertext: encrypted, ReverseGroupName: "default", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO admin_user_aggregation_targets(user_id, target_code, created_by) VALUES (?, ?, 'test')`, userID, code); err != nil {
		t.Fatal(err)
	}
}

func apiTestImport(t *testing.T, s *Server, token, idempotency, body string) database.APIImportJob {
	t.Helper()
	response := apiTestCall(s, "POST", "/api/v1/key-imports", token, "", idempotency, body)
	if response.Code != 202 && response.Code != 200 {
		t.Fatalf("import: %d %s", response.Code, response.Body.String())
	}
	var job database.APIImportJob
	if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	return job
}

func apiTestDrain(t *testing.T, s *Server) {
	t.Helper()
	for i := 0; i < 30; i++ {
		worked, err := s.runNextAPIImportTask(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !worked {
			return
		}
	}
	t.Fatal("queue did not drain")
}

func apiTestReceiver(t *testing.T, fail bool) (*httptest.Server, *atomic.Int32) {
	calls := &atomic.Int32{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/keyhub/channels/usage":
			writeJSON(w, 200, map[string]any{"success": true, "data": []any{}})
		case "/api/keyhub/channels":
			id := calls.Add(1)
			if fail {
				http.Error(w, "private receiver failure", 502)
				return
			}
			writeJSON(w, 200, map[string]any{"success": true, "data": map[string]any{"channel_id": id, "action": "created"}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)
	return upstream, calls
}

func TestPublicAPITokenLifecycleIntegration(t *testing.T) {
	s := publicAPITestServer(t)
	user, cookie := apiTestUser(t, s, "alice")
	_, bobCookie := apiTestUser(t, s, "bob")
	token, id := apiTestToken(t, s, cookie)
	if !validAPIToken(token) {
		t.Fatal("invalid generated token")
	}
	var stored string
	if err := s.db.QueryRow(`SELECT token_hash FROM user_api_tokens WHERE id = ?`, id).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == token || stored != security.SessionTokenHash(token) {
		t.Fatal("token storage is not hashed")
	}
	response := apiTestCall(s, "GET", "/api/account/api-tokens", "", cookie, "", "")
	if response.Code != 200 || strings.Contains(response.Body.String(), token) {
		t.Fatal("token list leaks plaintext or failed")
	}
	if got := apiTestCall(s, "GET", "/api/v1/key-options", token, "", "", ""); got.Code != 200 {
		t.Fatalf("options: %s", got.Body.String())
	}
	if got := apiTestCall(s, "GET", "/api/admin/users", token, "", "", ""); got.Code != 401 {
		t.Fatal("API token grants session/admin access")
	}
	path := fmt.Sprintf("/api/account/api-tokens/%d", id)
	if got := apiTestCall(s, "DELETE", path, "", bobCookie, "", ""); got.Code != 404 {
		t.Fatal("cross-user revocation accepted")
	}
	_, _ = s.db.Exec(`UPDATE admin_users SET disabled = TRUE WHERE id = ?`, user.ID)
	if got := apiTestCall(s, "GET", "/api/v1/key-options", token, "", "", ""); got.Code != 401 {
		t.Fatal("disabled user token accepted")
	}
	_, _ = s.db.Exec(`UPDATE admin_users SET disabled = FALSE WHERE id = ?`, user.ID)
	_, _ = s.db.Exec(`UPDATE user_api_tokens SET expires_at = CURRENT_TIMESTAMP - INTERVAL 1 DAY WHERE id = ?`, id)
	if got := apiTestCall(s, "GET", "/api/v1/key-options", token, "", "", ""); got.Code != 401 {
		t.Fatal("expired token accepted")
	}
	_, _ = s.db.Exec(`UPDATE user_api_tokens SET expires_at = CURRENT_TIMESTAMP + INTERVAL 1 DAY WHERE id = ?`, id)
	if got := apiTestCall(s, "DELETE", path, "", cookie, "", ""); got.Code != 200 {
		t.Fatalf("revoke: %s", got.Body.String())
	}
	if got := apiTestCall(s, "GET", "/api/v1/key-options", token, "", "", ""); got.Code != 401 {
		t.Fatal("revoked token accepted")
	}
}

func TestPublicAPIImportAndIsolationIntegration(t *testing.T) {
	s := publicAPITestServer(t)
	alice, cookie := apiTestUser(t, s, "alice")
	bob, bobCookie := apiTestUser(t, s, "bob")
	token, _ := apiTestToken(t, s, cookie)
	bobToken, _ := apiTestToken(t, s, bobCookie)
	receiver, calls := apiTestReceiver(t, false)
	apiTestTarget(t, s, alice.ID, "target-a", "api", receiver.URL)
	apiTestTarget(t, s, bob.ID, "target-b", "api", receiver.URL)
	body := `{"keys":["opaque-first","opaque-first","opaque-second"],"channelTypes":["anthropic"]}`
	job := apiTestImport(t, s, token, "request-0001", body)
	if job.Imported != 2 || job.Duplicates != 1 || job.PublishTotal != 2 || job.Items[1].KeyID != nil {
		t.Fatalf("job=%+v", job)
	}
	replay := apiTestImport(t, s, token, "request-0001", body)
	if replay.ID != job.ID {
		t.Fatal("idempotency returned another job")
	}
	if got := apiTestCall(s, "POST", "/api/v1/key-imports", token, "", "request-0001", `{"keys":["changed"],"channelTypes":["anthropic"]}`); got.Code != 409 {
		t.Fatal("idempotency conflict accepted")
	}
	otherJob := apiTestImport(t, s, bobToken, "request-0001", body)
	if otherJob.Imported != 0 || otherJob.Duplicates != 3 {
		t.Fatalf("cross-user dedupe: %+v", otherJob)
	}
	for _, item := range otherJob.Items {
		if item.KeyID != nil {
			t.Fatal("duplicate leaks another user's key id")
		}
	}
	if got := apiTestCall(s, "GET", "/api/v1/key-imports/"+job.ID, bobToken, "", "", ""); got.Code != 404 {
		t.Fatal("cross-user job visible")
	}
	apiTestDrain(t, s)
	completed, err := database.LoadAPIImportJob(context.Background(), s.db, alice.ID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "succeeded" || calls.Load() != 2 {
		t.Fatalf("job=%+v, calls=%d", completed, calls.Load())
	}
	got := apiTestCall(s, "GET", "/api/v1/key-imports/"+job.ID, token, "", "", "")
	for _, secret := range []string{"opaque-first", "target-a", receiver.URL, "receiver-test-secret"} {
		if strings.Contains(got.Body.String(), secret) {
			t.Fatalf("response exposes %q", secret)
		}
	}
	var ciphertext string
	if err := s.db.QueryRow(`SELECT key_ciphertext FROM api_keys WHERE id = ?`, *job.Items[0].KeyID).Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ciphertext, "opaque-first") {
		t.Fatal("key stored in plaintext")
	}
	// Existing page import and activation remain compatible.
	legacy := apiTestCall(s, "POST", "/api/keys/import", "", cookie, "", `{"rawText":"page-key","channelTypes":["anthropic"]}`)
	if legacy.Code != 200 {
		t.Fatalf("page import: %s", legacy.Body.String())
	}
	var legacyResult database.ImportResult
	_ = json.Unmarshal(legacy.Body.Bytes(), &legacyResult)
	legacy = apiTestCall(s, "POST", fmt.Sprintf("/api/keys/%d/activate-associated", legacyResult.ImportedKeyIDs[0]), "", cookie, "", `{"channelTypes":["anthropic"]}`)
	if legacy.Code != 200 || calls.Load() != 3 {
		t.Fatalf("page publish: %s", legacy.Body.String())
	}
}

func TestPublicAPIConcurrentIdempotencyIntegration(t *testing.T) {
	s := publicAPITestServer(t)
	user, cookie := apiTestUser(t, s, "alice")
	token, _ := apiTestToken(t, s, cookie)
	upstream, calls := apiTestReceiver(t, false)
	apiTestTarget(t, s, user.ID, "target", "api", upstream.URL)
	var wg sync.WaitGroup
	results := make(chan *httptest.ResponseRecorder, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- apiTestCall(s, "POST", "/api/v1/key-imports", token, "", "concurrent-request", `{"keys":["single-key"],"channelTypes":["anthropic"]}`)
		}()
	}
	wg.Wait()
	close(results)
	ids := map[string]bool{}
	for response := range results {
		if response.Code != 200 && response.Code != 202 {
			t.Fatalf("concurrent request: %d %s", response.Code, response.Body.String())
		}
		var job database.APIImportJob
		_ = json.Unmarshal(response.Body.Bytes(), &job)
		ids[job.ID] = true
	}
	if len(ids) != 1 {
		t.Fatal("concurrent requests created multiple jobs")
	}
	apiTestDrain(t, s)
	if calls.Load() != 1 {
		t.Fatalf("remote called %d times", calls.Load())
	}
	// Two tokens share the same write limit.
	secondToken, _ := apiTestToken(t, s, cookie)
	for i := 0; i < 6; i++ {
		_ = apiTestCall(s, "POST", "/api/v1/key-imports", secondToken, "", "concurrent-request", `{"keys":["single-key"],"channelTypes":["anthropic"]}`)
	}
	if got := apiTestCall(s, "POST", "/api/v1/key-imports", token, "", "concurrent-request", `{}`); got.Code != 429 {
		t.Fatalf("shared limit: %d", got.Code)
	}
}

func TestPublicAPIPermissionRetryAndUnknownOutcomeIntegration(t *testing.T) {
	s := publicAPITestServer(t)
	user, cookie := apiTestUser(t, s, "alice")
	token, _ := apiTestToken(t, s, cookie)
	healthy, goodCalls := apiTestReceiver(t, false)
	broken, badCalls := apiTestReceiver(t, true)
	apiTestTarget(t, s, user.ID, "healthy", "api", healthy.URL)
	apiTestTarget(t, s, user.ID, "broken", "api", broken.URL)
	job := apiTestImport(t, s, token, "partial-request", `{"keys":["partial-key"],"channelTypes":["anthropic"]}`)
	_, _ = s.db.Exec(`UPDATE aggregation_targets SET enabled = FALSE WHERE code = 'healthy'`)
	apiTestDrain(t, s)
	failed, _ := database.LoadAPIImportJob(context.Background(), s.db, user.ID, job.ID)
	if failed.Failed != 1 || failed.NeedsReview != 1 || goodCalls.Load() != 0 || badCalls.Load() != 1 {
		t.Fatalf("job=%+v", failed)
	}
	_, _ = s.db.Exec(`UPDATE aggregation_targets SET enabled = TRUE WHERE code = 'healthy'`)
	if got := apiTestCall(s, "POST", "/api/v1/key-imports/"+job.ID+"/retry", token, "", "", ""); got.Code != 202 {
		t.Fatalf("retry: %s", got.Body.String())
	}
	apiTestDrain(t, s)
	result, _ := database.LoadAPIImportJob(context.Background(), s.db, user.ID, job.ID)
	if result.Succeeded != 1 || result.NeedsReview != 1 || goodCalls.Load() != 1 || badCalls.Load() != 1 {
		t.Fatalf("unsafe retry or lost success: %+v good=%d bad=%d", result, goodCalls.Load(), badCalls.Load())
	}
}

func TestPublicAPIReverseRecoveryIntegration(t *testing.T) {
	s := publicAPITestServer(t)
	user, cookie := apiTestUser(t, s, "alice")
	token, _ := apiTestToken(t, s, cookie)
	var calls atomic.Int32
	var channel newapi.Channel
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/user/login":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "test", Path: "/"})
			writeJSON(w, 200, map[string]any{"success": true, "data": map[string]any{"id": 12, "role": 10}})
		case r.URL.Path == "/api/channel/" && r.Method == "POST":
			var request newapi.AddChannelRequest
			_ = json.NewDecoder(r.Body).Decode(&request)
			calls.Add(1)
			channel = newapi.Channel{ID: 99, Name: request.Channel.Name, Status: 1}
			// Remote side effect succeeded, but the response was lost.
			http.Error(w, "response lost", 502)
		case r.URL.Path == "/api/channel/" && r.Method == "GET":
			items := []newapi.Channel{}
			if channel.ID > 0 {
				items = append(items, channel)
			}
			writeJSON(w, 200, map[string]any{"success": true, "data": map[string]any{"items": items, "total": len(items)}})
		default:
			writeJSON(w, 200, map[string]any{"success": true})
		}
	}))
	defer upstream.Close()
	apiTestTarget(t, s, user.ID, "reverse", "new_api_reverse", upstream.URL)
	job := apiTestImport(t, s, token, "reverse-request", `{"keys":["reverse-key"],"channelTypes":["anthropic"]}`)
	apiTestDrain(t, s)
	result, _ := database.LoadAPIImportJob(context.Background(), s.db, user.ID, job.ID)
	if result.Status != "succeeded" || calls.Load() != 1 {
		t.Fatalf("recovery=%+v calls=%d", result, calls.Load())
	}
	// Simulate process exit after the successful remote operation but before task completion.
	_, _ = s.db.Exec(`UPDATE api_import_tasks SET status = 'running'`)
	apiTestDrain(t, s)
	result, _ = database.LoadAPIImportJob(context.Background(), s.db, user.ID, job.ID)
	if result.Status != "succeeded" || calls.Load() != 1 {
		t.Fatal("restart recovery repeated outbound creation")
	}
}

func TestPublicAPIInheritedPermissionsIntegration(t *testing.T) {
	s := publicAPITestServer(t)
	_, _ = apiTestUser(t, s, "root-user")
	manager, managerCookie := apiTestUser(t, s, "manager")
	if err := database.SetAdminUserRole(context.Background(), s.db, manager.ID, "account_manager"); err != nil {
		t.Fatal(err)
	}
	child, err := database.CreateAdminUserForAccountManager(context.Background(), s.db, "child", "test-password-only", "child", manager.ID)
	if err != nil {
		t.Fatal(err)
	}
	childSession := "session-child"
	if err := database.CreateAdminSession(context.Background(), s.db, security.SessionTokenHash(childSession), child.ID, time.Now().Add(time.Hour), "test", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	childToken, _ := apiTestToken(t, s, childSession)
	managerToken, _ := apiTestToken(t, s, managerCookie)
	upstream, calls := apiTestReceiver(t, false)
	apiTestTarget(t, s, manager.ID, "inherited", "api", upstream.URL)
	job := apiTestImport(t, s, childToken, "inherited-request", `{"keys":["child-key"],"channelTypes":["anthropic"]}`)
	apiTestDrain(t, s)
	var owner string
	if err := s.db.QueryRow(`SELECT created_by FROM api_keys WHERE id = ?`, *job.Items[0].KeyID).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner != "child" || calls.Load() != 1 {
		t.Fatal("inherited import ownership or publish incorrect")
	}
	if response := apiTestCall(s, "GET", "/api/v1/key-imports/"+job.ID, managerToken, "", "", ""); response.Code != 404 {
		t.Fatal("manager API token accesses another user's job")
	}
}

func TestPublicAPIAtomicEnqueueIntegration(t *testing.T) {
	s := publicAPITestServer(t)
	user, cookie := apiTestUser(t, s, "alice")
	_, tokenID := apiTestToken(t, s, cookie)
	err := database.EnqueueAPIImport(context.Background(), s.db, strings.Repeat("a", 32), user, tokenID, "atomic-request", strings.Repeat("b", 64),
		[]database.APIImportRow{{InputIndex: 0, Key: database.ImportAPIKey{Ciphertext: "test-ciphertext", Fingerprint: strings.Repeat("c", 64), KeyHint: "test", Models: []string{}, GroupName: "default", Status: "inventory"}}},
		[]database.APIImportTarget{{Code: strings.Repeat("x", 65), ChannelType: "anthropic", Signature: strings.Repeat("d", 64)}})
	if err == nil {
		t.Fatal("expected queue insertion failure")
	}
	for _, table := range []string{"api_keys", "key_import_batches", "api_import_jobs", "api_import_items", "api_import_tasks"} {
		var count int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("partial import left %d rows in %s", count, table)
		}
	}
}

func TestPublicAPIWorkerSwitchAndLockIntegration(t *testing.T) {
	s := publicAPITestServer(t)
	user, cookie := apiTestUser(t, s, "alice")
	token, _ := apiTestToken(t, s, cookie)
	upstream, calls := apiTestReceiver(t, false)
	apiTestTarget(t, s, user.ID, "target", "api", upstream.URL)
	body := `{"keys":["worker-key"],"channelTypes":["anthropic"]}`
	s.config.WorkerEnabled = false
	if response := apiTestCall(s, "POST", "/api/v1/key-imports", token, "", "worker-request", body); response.Code != 503 {
		t.Fatal("accepted new job with disabled worker")
	}
	s.config.WorkerEnabled = true
	job := apiTestImport(t, s, token, "worker-request", body)
	release, locked, err := database.AcquireAPILock(context.Background(), s.db, "api-import-worker", 0)
	if err != nil || !locked {
		t.Fatalf("lock: %v", err)
	}
	worked, err := s.runNextAPIImportTask(context.Background())
	release()
	if err != nil || worked || calls.Load() != 0 {
		t.Fatal("another worker processed while lock held")
	}
	apiTestDrain(t, s)
	s.config.WorkerEnabled = false
	replay := apiTestImport(t, s, token, "worker-request", body)
	if replay.ID != job.ID || replay.Status != "succeeded" {
		t.Fatal("disabled worker prevents read-only replay")
	}
}
