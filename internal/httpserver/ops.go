package httpserver

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"keyhub/internal/database"
)

type componentHealth struct {
	Status    string `json:"status"`
	Message   string `json:"message"`
	LatencyMS int64  `json:"latencyMs"`
}

type systemHealth struct {
	Status      string                     `json:"status"`
	Components  map[string]componentHealth `json:"components"`
	NewAPIBase  string                     `json:"newApiBase"`
	NewAPIUser  int                        `json:"newApiUser"`
	ServerTime  string                     `json:"serverTime"`
	StaticDir   string                     `json:"staticDir"`
	AutoMigrate bool                       `json:"autoMigrate"`
	Worker      bool                       `json:"worker"`
}

type opsStatus struct {
	Health       systemHealth           `json:"health"`
	TableStats   []database.TableStat   `json:"tableStats"`
	WorkerStats  []database.WorkerStat  `json:"workerStats"`
	RecentErrors []database.RecentError `json:"recentErrors"`
	GeneratedAt  time.Time              `json:"generatedAt"`
}

func (s *Server) handleOpsStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	tableStats, err := database.LoadTableStats(r.Context(), s.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	workerStats, err := database.LoadWorkerStats(r.Context(), s.db, 7)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	recentErrors, err := database.LoadRecentErrors(r.Context(), s.db, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, opsStatus{
		Health:       s.buildSystemHealth(r.Context(), 3*time.Second),
		TableStats:   tableStats,
		WorkerStats:  workerStats,
		RecentErrors: recentErrors,
		GeneratedAt:  time.Now(),
	})
}

func (s *Server) handleKeyExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := database.ExportKeyInventory(r.Context(), s.db, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = database.InsertAuditLog(r.Context(), s.db, actorFromRequest(r), "keys.export", "api_keys", nil, map[string]any{"count": len(items)})
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) buildSystemHealth(parent context.Context, timeout time.Duration) systemHealth {
	components := map[string]componentHealth{}
	components["database"] = s.checkDatabase(parent, timeout)
	components["static"] = s.checkStaticAssets()
	components["newApi"] = s.checkNewAPI(parent, timeout)

	status := "ok"
	for _, component := range components {
		if component.Status == "failed" {
			status = "degraded"
			break
		}
		if component.Status == "skipped" && status == "ok" {
			status = "ok"
		}
	}

	return systemHealth{
		Status:      status,
		Components:  components,
		NewAPIBase:  s.config.NewAPIBaseURL,
		NewAPIUser:  s.config.NewAPIAdminUserID,
		ServerTime:  time.Now().Format(time.RFC3339),
		StaticDir:   s.config.StaticDir,
		AutoMigrate: s.config.AutoMigrate,
		Worker:      s.config.WorkerEnabled,
	}
}

func (s *Server) checkDatabase(parent context.Context, timeout time.Duration) componentHealth {
	started := time.Now()
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	if err := s.db.PingContext(ctx); err != nil {
		return componentHealth{Status: "failed", Message: err.Error(), LatencyMS: time.Since(started).Milliseconds()}
	}
	return componentHealth{Status: "ok", Message: "connected", LatencyMS: time.Since(started).Milliseconds()}
}

func (s *Server) checkStaticAssets() componentHealth {
	started := time.Now()
	indexPath := filepath.Join(s.config.StaticDir, "index.html")
	info, err := os.Stat(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return componentHealth{Status: "failed", Message: "frontend assets not built", LatencyMS: time.Since(started).Milliseconds()}
		}
		return componentHealth{Status: "failed", Message: err.Error(), LatencyMS: time.Since(started).Milliseconds()}
	}
	if info.IsDir() {
		return componentHealth{Status: "failed", Message: "index.html is a directory", LatencyMS: time.Since(started).Milliseconds()}
	}
	return componentHealth{Status: "ok", Message: "index.html found", LatencyMS: time.Since(started).Milliseconds()}
}

func (s *Server) checkNewAPI(parent context.Context, timeout time.Duration) componentHealth {
	started := time.Now()
	if strings.TrimSpace(s.config.NewAPIAdminToken) == "" || s.config.NewAPIAdminUserID <= 0 {
		return componentHealth{Status: "skipped", Message: "new-api admin credentials are not configured", LatencyMS: time.Since(started).Milliseconds()}
	}
	client, err := s.newAPIClient()
	if err != nil {
		return componentHealth{Status: "failed", Message: err.Error(), LatencyMS: time.Since(started).Milliseconds()}
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	if err := client.CheckAdmin(ctx); err != nil {
		return componentHealth{Status: "failed", Message: err.Error(), LatencyMS: time.Since(started).Milliseconds()}
	}
	return componentHealth{Status: "ok", Message: "admin api reachable", LatencyMS: time.Since(started).Milliseconds()}
}
