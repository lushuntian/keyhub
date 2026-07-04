package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"keyhub/internal/config"
	"keyhub/internal/database"
	"keyhub/internal/keys"
	"keyhub/internal/newapi"
	"keyhub/internal/security"
)

type Server struct {
	config config.Config
	db     *sql.DB
}

const (
	aggregationTargetConnectionModeAPI           = "api"
	aggregationTargetConnectionModeNewAPIReverse = "new_api_reverse"
)

func New(cfg config.Config, db *sql.DB) *Server {
	return &Server{config: cfg, db: db}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/auth/register", s.handleAuthRegister)
	mux.HandleFunc("/api/auth/login", s.handleAuthLogin)
	mux.HandleFunc("/api/auth/logout", s.handleAuthLogout)
	mux.HandleFunc("/api/auth/me", s.handleAuthMe)
	mux.HandleFunc("/api/dashboard/summary", s.protect(s.handleDashboardSummary))
	mux.HandleFunc("/api/categories", s.protect(s.handleCategories))
	mux.HandleFunc("/api/aggregation-targets", s.protect(s.handleAggregationTargets))
	mux.HandleFunc("/api/admin/aggregation-targets", s.protectAdmin(s.handleAdminAggregationTargets))
	mux.HandleFunc("/api/admin/aggregation-targets/check-contract", s.protectAdmin(s.handleAdminAggregationTargetContractCheck))
	mux.HandleFunc("/api/admin/aggregation-targets/check-reverse-admin", s.protectAdmin(s.handleAdminAggregationTargetReverseAdminCheck))
	mux.HandleFunc("/api/admin/aggregation-targets/", s.protectAdmin(s.handleAdminAggregationTargetAction))
	mux.HandleFunc("/api/keys", s.protect(s.handleAPIKeys))
	mux.HandleFunc("/api/keys/", s.protectAdmin(s.handleAPIKeyAction))
	mux.HandleFunc("/api/keys/import", s.protect(s.handleKeyImport))
	mux.HandleFunc("/api/sync/events", s.protect(s.handleSyncEvents))
	mux.HandleFunc("/api/channels", s.protect(s.handleChannelGroups))
	mux.HandleFunc("/api/pool-rules/refill", s.protect(s.handlePoolFeatureGone))
	mux.HandleFunc("/api/pool-rules", s.protect(s.handlePoolFeatureGone))
	mux.HandleFunc("/api/pool-rules/", s.protect(s.handlePoolFeatureGone))
	mux.HandleFunc("/api/health-checks/run", s.protectAdmin(s.handleHealthCheckRun))
	mux.HandleFunc("/api/health-checks", s.protect(s.handleHealthChecks))
	mux.HandleFunc("/api/usage/sync", s.protectAdmin(s.handleUsageSync))
	mux.HandleFunc("/api/usage/summary", s.protect(s.handleUsageSummary))
	mux.HandleFunc("/api/workers/runs", s.protectAdmin(s.handleWorkerRuns))
	mux.HandleFunc("/api/audit/logs", s.protectAdmin(s.handleAuditLogs))
	mux.HandleFunc("/api/ops/status", s.protectAdmin(s.handleOpsStatus))
	mux.HandleFunc("/api/keys/export", s.protectAdmin(s.handleKeyExport))
	mux.HandleFunc("/api/meta/new-api-channel-types", s.protect(s.handleNewAPIChannelTypes))
	mux.Handle("/", spaHandler(s.config.StaticDir))
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, s.buildSystemHealth(r.Context(), 2*time.Second))
}

func (s *Server) handleDashboardSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	summary, err := database.LoadDashboardSummary(r.Context(), s.db, s.keyOwnerFilter(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handleChannelGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	groups, err := database.LoadChannelGroups(r.Context(), s.db, s.keyOwnerFilter(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": groups})
}

func (s *Server) handlePoolFeatureGone(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusGone, "库存自动补给功能已停用")
}

func (s *Server) handleCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	categories, err := database.LoadCategories(r.Context(), s.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": categories})
}

type aggregationTargetResponse struct {
	Code           string `json:"code"`
	Name           string `json:"name"`
	BaseURL        string `json:"baseUrl"`
	ConnectionMode string `json:"connectionMode,omitempty"`
	Default        bool   `json:"default"`
}

type adminAggregationTargetResponse struct {
	Code               string    `json:"code"`
	Name               string    `json:"name"`
	BaseURL            string    `json:"baseUrl"`
	ConnectionMode     string    `json:"connectionMode"`
	Enabled            bool      `json:"enabled"`
	Default            bool      `json:"default"`
	Source             string    `json:"source"`
	HasToken           bool      `json:"hasToken"`
	ReverseUsername    string    `json:"reverseUsername,omitempty"`
	HasReversePassword bool      `json:"hasReversePassword"`
	CreatedAt          time.Time `json:"createdAt,omitempty"`
	UpdatedAt          time.Time `json:"updatedAt,omitempty"`
}

type adminAggregationTargetRequest struct {
	Code            string `json:"code"`
	Name            string `json:"name"`
	BaseURL         string `json:"baseUrl"`
	ConnectionMode  string `json:"connectionMode"`
	Token           string `json:"token"`
	ReverseUsername string `json:"reverseUsername"`
	ReversePassword string `json:"reversePassword"`
	Enabled         *bool  `json:"enabled"`
	Default         bool   `json:"default"`
}

type aggregationTargetContractCheckRequest struct {
	Code    string `json:"code"`
	BaseURL string `json:"baseUrl"`
	Token   string `json:"token"`
}

type aggregationTargetReverseAdminCheckRequest struct {
	Code            string `json:"code"`
	BaseURL         string `json:"baseUrl"`
	ReverseUsername string `json:"reverseUsername"`
	ReversePassword string `json:"reversePassword"`
}

func (s *Server) handleAggregationTargets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	targets, err := s.listEnabledAggregationTargetOptions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]aggregationTargetResponse, 0, len(targets))
	for index, target := range targets {
		items = append(items, aggregationTargetResponse{
			Code:           target.Code,
			Name:           target.Name,
			BaseURL:        target.BaseURL,
			ConnectionMode: normalizeAggregationTargetConnectionModeOrAPI(target.ConnectionMode),
			Default:        index == 0,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleAdminAggregationTargetContractCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var request aggregationTargetContractCheckRequest
	if err := readJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	code := strings.TrimSpace(request.Code)
	if code == "" {
		code = "draft"
	}
	baseURL := strings.TrimRight(strings.TrimSpace(request.BaseURL), "/")
	if err := validateAggregationTargetBaseURL(baseURL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	token, err := s.resolveAggregationTargetToken(r.Context(), code, strings.TrimSpace(request.Token))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	usageCount, err := s.checkAggregationTargetCapabilities(r.Context(), baseURL, token)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("接收平台 %s 必须实现 GET /api/keyhub/channels/usage，否则不允许接入: %v", code, err))
		return
	}
	_ = database.InsertAuditLog(r.Context(), s.db, actorFromRequest(r), "aggregation_target.check_contract", "aggregation_targets", nil, map[string]any{
		"code":       code,
		"baseUrl":    baseURL,
		"usageCount": usageCount,
	})
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "usageCount": usageCount})
}

func (s *Server) handleAdminAggregationTargetReverseAdminCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var request aggregationTargetReverseAdminCheckRequest
	if err := readJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	code := strings.TrimSpace(request.Code)
	if code == "" {
		code = "draft"
	}
	baseURL := strings.TrimRight(strings.TrimSpace(request.BaseURL), "/")
	if err := validateAggregationTargetBaseURL(baseURL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	username, password, err := s.resolveAggregationTargetReverseCredentials(
		r.Context(),
		code,
		strings.TrimSpace(request.ReverseUsername),
		strings.TrimSpace(request.ReversePassword),
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	client, err := newapi.NewReverseClient(baseURL, username, password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	checkCtx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	result, err := client.CheckAdminChannelList(checkCtx)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	_ = database.InsertAuditLog(r.Context(), s.db, actorFromRequest(r), "aggregation_target.check_reverse_admin", "aggregation_targets", nil, map[string]any{
		"code":         code,
		"baseUrl":      baseURL,
		"username":     username,
		"userId":       result.UserID,
		"role":         result.Role,
		"channelTotal": result.ChannelTotal,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"success":      true,
		"userId":       result.UserID,
		"role":         result.Role,
		"channelTotal": result.ChannelTotal,
	})
}

func (s *Server) handleAdminAggregationTargets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := s.listAdminAggregationTargets(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		s.saveAdminAggregationTarget(w, r, "")
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleAdminAggregationTargetAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "api" || parts[1] != "admin" || parts[2] != "aggregation-targets" {
		writeError(w, http.StatusNotFound, "api route not found")
		return
	}
	code, err := url.PathUnescape(parts[3])
	if err != nil || strings.TrimSpace(code) == "" {
		writeError(w, http.StatusBadRequest, "invalid aggregation target code")
		return
	}
	switch r.Method {
	case http.MethodPut:
		s.saveAdminAggregationTarget(w, r, code)
	case http.MethodDelete:
		if err := database.DeleteAggregationTarget(r.Context(), s.db, strings.TrimSpace(code)); err != nil {
			if errors.Is(err, database.ErrAggregationTargetNotFound) {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = database.InsertAuditLog(r.Context(), s.db, actorFromRequest(r), "aggregation_target.delete", "aggregation_targets", nil, map[string]any{
			"code": strings.TrimSpace(code),
		})
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) saveAdminAggregationTarget(w http.ResponseWriter, r *http.Request, pathCode string) {
	var request adminAggregationTargetRequest
	if err := readJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	code := strings.TrimSpace(request.Code)
	if strings.TrimSpace(pathCode) != "" {
		code = strings.TrimSpace(pathCode)
	}
	name := strings.TrimSpace(request.Name)
	baseURL := strings.TrimRight(strings.TrimSpace(request.BaseURL), "/")
	connectionMode, err := normalizeAggregationTargetConnectionMode(request.ConnectionMode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	if err := validateAggregationTargetInput(code, name, baseURL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var existing database.AggregationTargetRecord
	existing, err = database.GetAggregationTarget(r.Context(), s.db, code)
	if err != nil && !errors.Is(err, database.ErrAggregationTargetNotFound) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	isCreate := errors.Is(err, database.ErrAggregationTargetNotFound)
	if strings.TrimSpace(pathCode) != "" && isCreate {
		writeError(w, http.StatusNotFound, database.ErrAggregationTargetNotFound.Error())
		return
	}

	tokenCiphertext := existing.TokenCiphertext
	reverseUsername := strings.TrimSpace(request.ReverseUsername)
	reversePasswordCiphertext := existing.ReversePasswordCiphertext
	if connectionMode == aggregationTargetConnectionModeAPI {
		reverseUsername = ""
		reversePasswordCiphertext = ""

		token := strings.TrimSpace(request.Token)
		tokenPlaintext := token
		if tokenPlaintext == "" && tokenCiphertext != "" {
			encryptor, err := security.NewEncryptor(s.config.EncryptionKey)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			tokenPlaintext, err = encryptor.Decrypt(tokenCiphertext)
			if err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("decrypt aggregation target token: %v", err))
				return
			}
		}
		if tokenPlaintext == "" {
			writeError(w, http.StatusBadRequest, "token is required")
			return
		}
		if _, err := s.checkAggregationTargetCapabilities(r.Context(), baseURL, tokenPlaintext); err != nil {
			err = fmt.Errorf("接收平台 %s 必须实现 GET /api/keyhub/channels/usage，否则不允许接入: %w", code, err)
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if token != "" {
			encryptor, err := security.NewEncryptor(s.config.EncryptionKey)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			tokenCiphertext, err = encryptor.Encrypt(token)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
	} else {
		tokenCiphertext = ""
		if reverseUsername == "" || len([]rune(reverseUsername)) > 255 {
			writeError(w, http.StatusBadRequest, "new-api 逆向账号必须为 1-255 个字符")
			return
		}
		reversePassword := strings.TrimSpace(request.ReversePassword)
		if reversePassword != "" {
			encryptor, err := security.NewEncryptor(s.config.EncryptionKey)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			reversePasswordCiphertext, err = encryptor.Encrypt(reversePassword)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		if reversePasswordCiphertext == "" {
			writeError(w, http.StatusBadRequest, "new-api 逆向密码不能为空")
			return
		}
	}
	if isCreate {
		count, err := database.CountAggregationTargets(r.Context(), s.db)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if count == 0 && enabled {
			request.Default = true
		}
	}

	if err := database.SaveAggregationTarget(r.Context(), s.db, database.AggregationTargetUpsert{
		Code:                      code,
		Name:                      name,
		BaseURL:                   baseURL,
		ConnectionMode:            connectionMode,
		TokenCiphertext:           tokenCiphertext,
		ReverseUsername:           reverseUsername,
		ReversePasswordCiphertext: reversePasswordCiphertext,
		Enabled:                   enabled,
		Default:                   request.Default && enabled,
		SortOrder:                 100,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	action := "aggregation_target.update"
	if isCreate {
		action = "aggregation_target.create"
	}
	_ = database.InsertAuditLog(r.Context(), s.db, actorFromRequest(r), action, "aggregation_targets", nil, map[string]any{
		"code":           code,
		"baseUrl":        baseURL,
		"connectionMode": connectionMode,
		"enabled":        enabled,
		"default":        request.Default && enabled,
	})
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) listAdminAggregationTargets(ctx context.Context) ([]adminAggregationTargetResponse, error) {
	records, err := database.ListAggregationTargets(ctx, s.db, false)
	if err != nil {
		return nil, err
	}
	if len(records) > 0 {
		items := make([]adminAggregationTargetResponse, 0, len(records))
		for index, record := range records {
			items = append(items, adminAggregationTargetResponse{
				Code:               record.Code,
				Name:               record.Name,
				BaseURL:            record.BaseURL,
				ConnectionMode:     normalizeAggregationTargetConnectionModeOrAPI(record.ConnectionMode),
				Enabled:            record.Enabled,
				Default:            record.Default || index == 0 && !hasDefaultAggregationTarget(records),
				Source:             record.Source,
				HasToken:           record.HasToken,
				ReverseUsername:    record.ReverseUsername,
				HasReversePassword: record.HasReversePassword,
				CreatedAt:          record.CreatedAt,
				UpdatedAt:          record.UpdatedAt,
			})
		}
		return items, nil
	}

	items := make([]adminAggregationTargetResponse, 0, len(s.config.AggregationTargets))
	for index, target := range s.config.AggregationTargets {
		items = append(items, adminAggregationTargetResponse{
			Code:               target.Code,
			Name:               target.Name,
			BaseURL:            target.BaseURL,
			ConnectionMode:     normalizeAggregationTargetConnectionModeOrAPI(target.ConnectionMode),
			Enabled:            true,
			Default:            index == 0,
			Source:             "env",
			HasToken:           target.Token != "",
			HasReversePassword: false,
		})
	}
	return items, nil
}

func (s *Server) validateAggregationTargetCapabilities(ctx context.Context, code string, baseURL string, token string) error {
	if _, err := s.checkAggregationTargetCapabilities(ctx, baseURL, token); err != nil {
		return fmt.Errorf("接收平台 %s 必须实现 GET /api/keyhub/channels/usage，否则不允许接入: %w", code, err)
	}
	return nil
}

func (s *Server) checkAggregationTargetCapabilities(ctx context.Context, baseURL string, token string) (int, error) {
	client, err := newapi.NewPushClient(baseURL, token)
	if err != nil {
		return 0, err
	}
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	items, err := client.ListChannelUsage(checkCtx)
	if err != nil {
		return 0, err
	}
	return len(items), nil
}

func (s *Server) resolveAggregationTargetToken(ctx context.Context, code string, token string) (string, error) {
	if strings.TrimSpace(token) != "" {
		return strings.TrimSpace(token), nil
	}
	record, err := database.GetAggregationTarget(ctx, s.db, code)
	if errors.Is(err, database.ErrAggregationTargetNotFound) {
		return "", fmt.Errorf("token is required")
	}
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(record.TokenCiphertext) == "" {
		return "", fmt.Errorf("token is required")
	}
	encryptor, err := security.NewEncryptor(s.config.EncryptionKey)
	if err != nil {
		return "", err
	}
	return encryptor.Decrypt(record.TokenCiphertext)
}

func (s *Server) resolveAggregationTargetReverseCredentials(ctx context.Context, code string, username string, password string) (string, string, error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" {
		record, err := database.GetAggregationTarget(ctx, s.db, strings.TrimSpace(code))
		if err == nil {
			username = strings.TrimSpace(record.ReverseUsername)
		} else if !errors.Is(err, database.ErrAggregationTargetNotFound) {
			return "", "", err
		}
	}
	if username == "" {
		return "", "", fmt.Errorf("new-api 逆向账号不能为空")
	}
	if password != "" {
		return username, password, nil
	}
	record, err := database.GetAggregationTarget(ctx, s.db, strings.TrimSpace(code))
	if errors.Is(err, database.ErrAggregationTargetNotFound) {
		return "", "", fmt.Errorf("new-api 逆向密码不能为空")
	}
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(record.ReversePasswordCiphertext) == "" {
		return "", "", fmt.Errorf("new-api 逆向密码不能为空")
	}
	encryptor, err := security.NewEncryptor(s.config.EncryptionKey)
	if err != nil {
		return "", "", err
	}
	password, err = encryptor.Decrypt(record.ReversePasswordCiphertext)
	if err != nil {
		return "", "", fmt.Errorf("decrypt aggregation target reverse password: %w", err)
	}
	return username, password, nil
}

func validateAggregationTargetInput(code string, name string, baseURL string) error {
	if !validAggregationTargetCode(code) {
		return fmt.Errorf("code must be 1-64 characters and only contain letters, numbers, dot, underscore, or hyphen")
	}
	if name == "" || len([]rune(name)) > 128 {
		return fmt.Errorf("name must be 1-128 characters")
	}
	return validateAggregationTargetBaseURL(baseURL)
}

func normalizeAggregationTargetConnectionMode(mode string) (string, error) {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return aggregationTargetConnectionModeAPI, nil
	}
	switch mode {
	case aggregationTargetConnectionModeAPI, aggregationTargetConnectionModeNewAPIReverse:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported aggregation target connectionMode: %s", mode)
	}
}

func normalizeAggregationTargetConnectionModeOrAPI(mode string) string {
	normalized, err := normalizeAggregationTargetConnectionMode(mode)
	if err != nil {
		return aggregationTargetConnectionModeAPI
	}
	return normalized
}

func validateAggregationTargetBaseURL(baseURL string) error {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("baseUrl must be a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("baseUrl must use http or https")
	}
	if len(baseURL) > 512 {
		return fmt.Errorf("baseUrl is too long")
	}
	return nil
}

func validAggregationTargetCode(code string) bool {
	if len(code) == 0 || len(code) > 64 {
		return false
	}
	for _, ch := range code {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '.' || ch == '_' || ch == '-' {
			continue
		}
		return false
	}
	return true
}

func hasDefaultAggregationTarget(records []database.AggregationTargetRecord) bool {
	for _, record := range records {
		if record.Default {
			return true
		}
	}
	return false
}

func (s *Server) handleAPIKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	result, err := database.ListAPIKeys(r.Context(), s.db, limit, offset, s.keyOwnerFilter(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleAPIKeyAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if (len(parts) != 3 && len(parts) != 4) || parts[0] != "api" || parts[1] != "keys" {
		writeError(w, http.StatusNotFound, "api route not found")
		return
	}
	keyID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || keyID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid key id")
		return
	}

	if len(parts) == 3 {
		if r.Method != http.MethodDelete {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleKeyDelete(w, r, keyID)
		return
	}

	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	switch parts[3] {
	case "activate":
		s.handleKeyActivate(w, r, keyID)
	case "disable":
		s.handleKeyDisable(w, r, keyID)
	default:
		writeError(w, http.StatusNotFound, "api route not found")
	}
}

type keyImportRequest struct {
	CategoryCode string   `json:"categoryCode"`
	EndpointURL  string   `json:"endpointUrl"`
	RawText      string   `json:"rawText"`
	Tag          string   `json:"tag"`
	GroupName    string   `json:"groupName"`
	Models       []string `json:"models"`
	Note         string   `json:"note"`
	ExpectedTPM  int64    `json:"expectedTpm"`
}

type keyActivateRequest struct {
	TargetCode string `json:"targetCode"`
}

func (s *Server) handleKeyImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var request keyImportRequest
	if err := readJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if request.ExpectedTPM < 0 {
		writeError(w, http.StatusBadRequest, "expectedTpm must be greater than or equal to 0")
		return
	}
	category, err := database.LoadCategory(r.Context(), s.db, request.CategoryCode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	endpointURL := strings.TrimSpace(request.EndpointURL)
	encryptor, err := security.NewEncryptor(s.config.EncryptionKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	groupName := strings.TrimSpace(request.GroupName)
	if groupName == "" {
		groupName = "default"
	}
	status := "inventory"

	parsedItems, err := s.parseKeys(r.Context(), request.RawText, category, request.Models)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	rows := make([]database.ImportAPIKey, 0, len(parsedItems))
	duplicates := 0
	failed := 0
	for _, item := range parsedItems {
		if item.Error != "" {
			if item.Duplicate {
				duplicates++
			} else {
				failed++
			}
			continue
		}

		ciphertext, encryptErr := encryptor.Encrypt(item.Normalized)
		if encryptErr != nil {
			writeError(w, http.StatusInternalServerError, encryptErr.Error())
			return
		}
		baseURL := strings.TrimSpace(item.BaseURL)
		if baseURL == "" {
			baseURL = endpointURLForParsedKey(endpointURL, item)
		}
		rows = append(rows, database.ImportAPIKey{
			CategoryCode: category.Code,
			Ciphertext:   ciphertext,
			Fingerprint:  item.Fingerprint,
			KeyHint:      item.KeyHint,
			Region:       item.Region,
			BaseURL:      baseURL,
			Models:       item.Models,
			Tag:          strings.TrimSpace(request.Tag),
			GroupName:    groupName,
			Note:         strings.TrimSpace(request.Note),
			ExpectedTPM:  request.ExpectedTPM,
			Status:       status,
		})
	}

	result, err := database.ImportKeys(
		r.Context(),
		s.db,
		category.Code,
		strings.TrimSpace(request.Tag),
		groupName,
		strings.TrimSpace(request.Note),
		actorFromRequest(r),
		len(parsedItems),
		rows,
		duplicates,
		failed,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleSyncEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	events, err := database.ListSyncEvents(r.Context(), s.db, limit, s.keyOwnerFilter(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": events})
}

func (s *Server) handleKeyActivate(w http.ResponseWriter, r *http.Request, keyID int64) {
	request := keyActivateRequest{}
	if r.Body != nil {
		if err := readJSON(w, r, &request); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	newAPIChannelID, targetCode, err := s.activateKey(r.Context(), keyID, request.TargetCode)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"newApiChannelId": newAPIChannelID, "targetCode": targetCode})
}

func (s *Server) handleKeyDisable(w http.ResponseWriter, r *http.Request, keyID int64) {
	err := s.disableKey(r.Context(), keyID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"disabled": true})
}

func (s *Server) handleKeyDelete(w http.ResponseWriter, r *http.Request, keyID int64) {
	record, err := database.DeleteAPIKey(r.Context(), s.db, keyID)
	if err != nil {
		if errors.Is(err, database.ErrAPIKeyNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, database.ErrActiveAPIKeyDeleteBlock) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = database.InsertAuditLog(r.Context(), s.db, actorFromRequest(r), "key.delete", "api_keys", &keyID, map[string]any{
		"categoryCode": record.CategoryCode,
		"keyHint":      record.KeyHint,
		"status":       record.Status,
	})
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) handleNewAPIChannelTypes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": []map[string]any{
			{"code": "aws_bedrock", "label": "AWS Bedrock", "newApiType": 33, "keyFormat": "AccessKey|SecretKey|Region"},
			{"code": "anthropic", "label": "Anthropic 官方", "newApiType": 14, "keyFormat": "sk-ant-..."},
			{"code": "openai", "label": "OpenAI", "newApiType": 1, "keyFormat": "sk-..."},
			{"code": "azure_openai", "label": "Azure OpenAI", "newApiType": 3, "keyFormat": "Endpoint|ApiKey|ApiVersion"},
			{"code": "google_ai_studio", "label": "Google AI Studio", "newApiType": 24, "keyFormat": "AIza..."},
		},
	})
}

func (s *Server) newAPIClient() (*newapi.Client, error) {
	return newapi.NewClient(s.config.NewAPIBaseURL, s.config.NewAPIAdminToken, s.config.NewAPIAdminUserID)
}

func (s *Server) listEnabledAggregationTargetOptions(ctx context.Context) ([]config.AggregationTarget, error) {
	records, err := database.ListAggregationTargets(ctx, s.db, true)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		targets := make([]config.AggregationTarget, 0, len(s.config.AggregationTargets))
		for _, target := range s.config.AggregationTargets {
			target.ConnectionMode = normalizeAggregationTargetConnectionModeOrAPI(target.ConnectionMode)
			targets = append(targets, target)
		}
		return targets, nil
	}

	targets := make([]config.AggregationTarget, 0, len(records))
	for _, record := range records {
		targets = append(targets, config.AggregationTarget{
			Code:           record.Code,
			Name:           record.Name,
			BaseURL:        record.BaseURL,
			ConnectionMode: normalizeAggregationTargetConnectionModeOrAPI(record.ConnectionMode),
		})
	}
	return targets, nil
}

func (s *Server) loadEnabledAggregationTargets(ctx context.Context) ([]config.AggregationTarget, error) {
	records, err := database.ListAggregationTargetSecrets(ctx, s.db, false)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		targets := make([]config.AggregationTarget, 0, len(s.config.AggregationTargets))
		for _, target := range s.config.AggregationTargets {
			target.ConnectionMode = normalizeAggregationTargetConnectionModeOrAPI(target.ConnectionMode)
			targets = append(targets, target)
		}
		return targets, nil
	}

	encryptor, err := security.NewEncryptor(s.config.EncryptionKey)
	if err != nil {
		return nil, err
	}
	targets := make([]config.AggregationTarget, 0, len(records))
	for _, record := range records {
		if !record.Enabled {
			continue
		}
		connectionMode := normalizeAggregationTargetConnectionModeOrAPI(record.ConnectionMode)
		target := config.AggregationTarget{
			Code:           record.Code,
			Name:           record.Name,
			BaseURL:        record.BaseURL,
			ConnectionMode: connectionMode,
		}
		switch connectionMode {
		case aggregationTargetConnectionModeAPI:
			token, err := encryptor.Decrypt(record.TokenCiphertext)
			if err != nil {
				return nil, fmt.Errorf("decrypt aggregation target %s token: %w", record.Code, err)
			}
			target.Token = token
		case aggregationTargetConnectionModeNewAPIReverse:
			password, err := encryptor.Decrypt(record.ReversePasswordCiphertext)
			if err != nil {
				return nil, fmt.Errorf("decrypt aggregation target %s reverse password: %w", record.Code, err)
			}
			target.ReverseUsername = record.ReverseUsername
			target.ReversePassword = password
		}
		targets = append(targets, target)
	}
	return targets, nil
}

func (s *Server) aggregationTarget(ctx context.Context, code string) (config.AggregationTarget, error) {
	targets, err := s.loadEnabledAggregationTargets(ctx)
	if err != nil {
		return config.AggregationTarget{}, err
	}
	code = strings.TrimSpace(code)
	if code == "" && len(targets) > 0 {
		return targets[0], nil
	}
	for _, target := range targets {
		if target.Code == code {
			return target, nil
		}
	}
	return config.AggregationTarget{}, fmt.Errorf("unknown aggregation target: %s", code)
}

func buildNewAPIChannelPayload(keyRecord database.APIKeyForSync, plaintextKey string) newapi.ChannelPayload {
	autoBan := 1
	priority := int64(0)
	weight := uint(0)
	channelKey := strings.TrimSpace(plaintextKey)
	baseURL := strings.TrimSpace(keyRecord.BaseURL)
	other := ""
	if keyRecord.CategoryCode == "azure_openai" {
		channelKey, baseURL, other = normalizeAzureNewAPIChannelFields(plaintextKey, baseURL)
	}
	group := strings.TrimSpace(keyRecord.GroupName)
	if group == "" {
		group = "default"
	}
	payload := newapi.ChannelPayload{
		Type:          keyRecord.NewAPIType,
		Key:           channelKey,
		Status:        newapi.ChannelStatusManuallyDisabled,
		Name:          buildChannelName(keyRecord),
		Models:        strings.Join(keyRecord.Models, ","),
		Group:         group,
		Other:         other,
		OtherSettings: buildOtherSettings(keyRecord.CategoryCode, plaintextKey),
		AutoBan:       &autoBan,
		Priority:      &priority,
		Weight:        &weight,
	}
	if baseURL != "" {
		payload.BaseURL = &baseURL
	}
	if strings.TrimSpace(keyRecord.Tag) != "" {
		tag := strings.TrimSpace(keyRecord.Tag)
		payload.Tag = &tag
	}
	if strings.TrimSpace(keyRecord.Note) != "" {
		remark := strings.TrimSpace(keyRecord.Note)
		payload.Remark = &remark
	}
	return payload
}

func normalizeAzureNewAPIChannelFields(plaintextKey string, fallbackBaseURL string) (string, string, string) {
	fallbackBaseURL = strings.TrimRight(strings.TrimSpace(fallbackBaseURL), "/")
	parts := strings.Split(plaintextKey, "|")
	if len(parts) != 2 && len(parts) != 3 {
		return strings.TrimSpace(plaintextKey), fallbackBaseURL, ""
	}
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
		if parts[index] == "" {
			return strings.TrimSpace(plaintextKey), fallbackBaseURL, ""
		}
	}
	apiVersion := ""
	if len(parts) == 3 {
		apiVersion = parts[2]
	}
	return parts[1], strings.TrimRight(parts[0], "/"), apiVersion
}

func buildChannelName(keyRecord database.APIKeyForSync) string {
	hint := strings.NewReplacer("|", "-", " ", "-", "...", "-").Replace(keyRecord.KeyHint)
	hint = strings.Trim(hint, "-")
	if hint == "" {
		hint = "key"
	}
	return fmt.Sprintf("keyhub_%s_%d_%s", keyRecord.CategoryCode, keyRecord.ID, hint)
}

func buildOtherSettings(categoryCode string, plaintextKey string) string {
	if categoryCode != "aws_bedrock" && categoryCode != "claude_on_aws" {
		return "{}"
	}
	keyType := "ak_sk"
	if len(strings.Split(plaintextKey, "|")) == 2 {
		keyType = "api_key"
	}
	payload, _ := json.Marshal(map[string]string{"aws_key_type": keyType})
	return string(payload)
}

func redactedChannelPayload(keyRecord database.APIKeyForSync, payload newapi.ChannelPayload) map[string]any {
	return map[string]any{
		"apiKeyId":     keyRecord.ID,
		"categoryCode": keyRecord.CategoryCode,
		"keyHint":      keyRecord.KeyHint,
		"name":         payload.Name,
		"type":         payload.Type,
		"models":       payload.Models,
		"group":        payload.Group,
		"tag":          keyRecord.Tag,
		"baseUrl":      keyRecord.BaseURL,
	}
}

func endpointURLForParsedKey(endpointURL string, item keys.ParsedKey) string {
	endpointURL = strings.TrimRight(strings.TrimSpace(endpointURL), "/")
	if endpointURL == "" {
		return ""
	}
	if strings.TrimSpace(item.Region) == "" {
		return endpointURL
	}
	return strings.NewReplacer(
		"{region}", item.Region,
		"${region}", item.Region,
		"<region>", item.Region,
	).Replace(endpointURL)
}

func (s *Server) parseKeys(ctx context.Context, rawText string, category keys.Category, models []string) ([]keys.ParsedKey, error) {
	items := keys.ParseLines(rawText, keys.ParseOptions{Category: category, Models: models})
	fingerprints := make([]string, 0, len(items))
	for _, item := range items {
		if item.Error == "" {
			fingerprints = append(fingerprints, item.Fingerprint)
		}
	}

	existing, err := database.ExistingFingerprints(ctx, s.db, fingerprints)
	if err != nil {
		return nil, err
	}
	for index := range items {
		if items[index].Error == "" && existing[items[index].Fingerprint] {
			items[index].Duplicate = true
			items[index].Error = "数据库中已存在"
		}
	}
	return items, nil
}

func actorFromRequest(r *http.Request) string {
	if user := currentAdminUser(r); user != nil && strings.TrimSpace(user.Username) != "" {
		return user.Username
	}
	actor := strings.TrimSpace(r.Header.Get("X-KeyHub-Actor"))
	if actor == "" {
		return "local"
	}
	return actor
}

func readJSON(w http.ResponseWriter, r *http.Request, value any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"success": false,
		"message": message,
	})
}

func spaHandler(staticDir string) http.HandlerFunc {
	fileServer := http.FileServer(http.Dir(staticDir))
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusNotFound, "api route not found")
			return
		}

		cleanPath := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if cleanPath == "." {
			cleanPath = "index.html"
		}
		fullPath := filepath.Join(staticDir, cleanPath)

		info, err := os.Stat(fullPath)
		if err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		indexPath := filepath.Join(staticDir, "index.html")
		if _, err := os.Stat(indexPath); err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("<!doctype html><title>KeyHub</title><body><h1>KeyHub backend is running</h1><p>Frontend assets have not been built yet.</p></body>"))
			return
		}
		http.ServeFile(w, r, indexPath)
	}
}
