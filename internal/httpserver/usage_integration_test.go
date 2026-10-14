package httpserver

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"keyhub/internal/database"
	"keyhub/internal/newapi"
)

func TestUsageSchemaMigrationIntegration(t *testing.T) {
	for _, units := range []float64{250000, 0} {
		t.Run(fmt.Sprintf("units_%g", units), func(t *testing.T) {
			s := publicAPITestServer(t)
			ctx := context.Background()
			user, cookie := apiTestUser(t, s, "usage-owner")
			token, _ := apiTestToken(t, s, cookie)
			var currentQuota atomic.Int64
			currentQuota.Store(1000000)
			receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/keyhub/channels":
					writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{"channel_id": 42, "action": "created"}})
				case "/api/keyhub/channels/usage":
					writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": []newapi.ChannelUsage{{ChannelID: 42, UsedQuota: currentQuota.Load()}}})
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(receiver.Close)
			apiTestTarget(t, s, user.ID, "usage-target", "api", receiver.URL)
			job := apiTestImport(t, s, token, "usage-schema-request", `{"keys":["usage-test-key"],"channelTypes":["anthropic"]}`)
			apiTestDrain(t, s)
			if len(job.Items) != 1 || job.Items[0].KeyID == nil {
				t.Fatalf("import result: %+v", job)
			}
			keyID := *job.Items[0].KeyID
			var createdAt time.Time
			if err := s.db.QueryRow(`SELECT created_at FROM admin_user_aggregation_targets WHERE user_id = ?`, user.ID).Scan(&createdAt); err != nil {
				t.Fatal(err)
			}
			for _, query := range []string{
				`ALTER TABLE admin_user_aggregation_targets ADD COLUMN extra_value DECIMAL(10,6) NOT NULL DEFAULT 0.4, ADD CONSTRAINT chk_user_target_extra_value CHECK (extra_value >= 0 AND extra_value <= 1)`,
				`DELETE FROM schema_migrations WHERE version = '034_normalize_usage_schema.sql'`,
			} {
				if _, err := s.db.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.db.Exec(`INSERT INTO usage_daily_snapshots(stat_date, api_key_id, target_code, newapi_channel_id, quota_amount, usd_amount) VALUES (UTC_DATE() - INTERVAL 1 DAY, ?, 'usage-target', 42, 1000000, 0.8)`, keyID); err != nil {
				t.Fatal(err)
			}
			if err := database.ApplyMigrations(ctx, s.db, "../../migrations", units); err != nil {
				t.Fatal(err)
			}
			if err := database.ApplyMigrations(ctx, s.db, "../../migrations", units); err != nil {
				t.Fatalf("repeat migration: %v", err)
			}
			var columns string
			if err := s.db.QueryRow(`SELECT GROUP_CONCAT(COLUMN_NAME ORDER BY ORDINAL_POSITION) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'admin_user_aggregation_targets'`).Scan(&columns); err != nil {
				t.Fatal(err)
			}
			if columns != "user_id,target_code,created_by,created_at" {
				t.Fatalf("association columns: %s", columns)
			}
			var targetCode, createdBy string
			var migratedAt time.Time
			if err := s.db.QueryRow(`SELECT target_code, created_by, created_at FROM admin_user_aggregation_targets WHERE user_id = ?`, user.ID).Scan(&targetCode, &createdBy, &migratedAt); err != nil {
				t.Fatal(err)
			}
			if targetCode != "usage-target" || createdBy != "test" || !createdAt.Equal(migratedAt) {
				t.Fatalf("association changed: %s %s %v", targetCode, createdBy, migratedAt)
			}
			var foreignKeys int
			if err := s.db.QueryRow(`SELECT COUNT(*) FROM information_schema.TABLE_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = DATABASE() AND TABLE_NAME = 'admin_user_aggregation_targets' AND CONSTRAINT_TYPE = 'FOREIGN KEY'`).Scan(&foreignKeys); err != nil {
				t.Fatal(err)
			}
			if foreignKeys != 2 {
				t.Fatalf("association foreign keys: %d", foreignKeys)
			}
			effectiveUnits := units
			if effectiveUnits <= 0 {
				effectiveUnits = 500000
			}
			s.config.NewAPIQuotaPerUSD = effectiveUnits
			assertUsageTotals := func(quota int64) {
				t.Helper()
				want := float64(quota) / effectiveUnits
				summary, err := database.LoadUsageSummary(ctx, s.db, 30, user.Username)
				if err != nil {
					t.Fatal(err)
				}
				dashboard, err := database.LoadDashboardSummary(ctx, s.db, user.Username)
				if err != nil {
					t.Fatal(err)
				}
				keys, err := database.ListAPIKeys(ctx, s.db, 20, 0, user.Username, true)
				if err != nil {
					t.Fatal(err)
				}
				users, err := database.ListAdminUsers(ctx, s.db)
				if err != nil || len(users) != 1 {
					t.Fatalf("users: %+v, error: %v", users, err)
				}
				log := sanitizeUsageLog(newapi.UsageLog{Quota: quota}, keyID, effectiveUnits)
				for name, got := range map[string]float64{
					"summary": summary.TotalUSD, "dashboard": dashboard.TotalUsageUSD,
					"keys": keys.TotalUsageUSD, "user": users[0].TotalUsageUSD, "log": log.QuotaUSD,
				} {
					if math.Abs(got-want) > 1e-9 {
						t.Errorf("%s amount = %v, want %v", name, got, want)
					}
				}
				if summary.TotalQuota != quota {
					t.Errorf("summary quota = %d, want %d", summary.TotalQuota, quota)
				}
			}
			assertUsageTotals(1000000)
			for _, quota := range []int64{1000000, 1500000} {
				currentQuota.Store(quota)
				result, err := s.runUsageSync(ctx)
				if err != nil || result.Synced != 1 || result.TotalSyncedQuota != quota || result.TotalSyncedUSD != float64(quota)/effectiveUnits {
					t.Fatalf("usage sync: %+v, error: %v", result, err)
				}
				assertUsageTotals(quota)
			}
		})
	}
}

func TestUserTargetAssociationsIntegration(t *testing.T) {
	s := publicAPITestServer(t)
	user, cookie := apiTestUser(t, s, "root-user")
	apiTestTarget(t, s, user.ID, "target-a", "api", "https://example.invalid")
	path := fmt.Sprintf("/api/admin/users/%d/aggregation-targets", user.ID)
	for _, body := range []string{`{"targets":[{"targetCode":"target-a"}]}`, `{"targetCodes":["target-a"]}`} {
		response := apiTestCall(s, http.MethodPut, path, "", cookie, "", body)
		if response.Code != http.StatusOK {
			t.Fatalf("save targets: %d %s", response.Code, response.Body.String())
		}
		targets, err := database.ListAdminUserAggregationTargets(context.Background(), s.db)
		want := []database.AdminUserAggregationTarget{{TargetCode: "target-a"}}
		if err != nil || !reflect.DeepEqual(targets[user.ID], want) {
			t.Fatalf("saved targets: %+v, error: %v", targets, err)
		}
	}
	response := apiTestCall(s, http.MethodPut, path, "", cookie, "", `{"targets":[]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("clear targets: %d %s", response.Code, response.Body.String())
	}
	allowed, err := database.UserCanUseAggregationTarget(context.Background(), s.db, user.Username, "target-a")
	if err != nil || allowed {
		t.Fatalf("cleared target permission: %v, error: %v", allowed, err)
	}
}
