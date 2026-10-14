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
	"keyhub/internal/tokensupply"
)

type Server struct {
	config         config.Config
	db             *sql.DB
	reverseClients reverseClientCache
}

const (
	aggregationTargetConnectionModeAPI           = "api"
	aggregationTargetConnectionModeNewAPIReverse = "new_api_reverse"
	aggregationTargetConnectionModeTokenSupply   = "token_supply"
	aggregationTargetDefaultChannelType          = "anthropic"
	aggregationTargetChannelTypeAWSBedrockLegacy = "aws_bedrock"
	aggregationTargetChannelTypeAWSBedrockNew    = "aws_bedrock_new"
	awsCredentialTypeAKSK                        = "ak_sk"
	awsCredentialTypeAPIKey                      = "api_key"
	awsBedrockNewRegionPrefix                    = "https://bedrock-mantle"
)

var aggregationTargetChannelTypes = map[string]struct{}{
	"anthropic": {},
	aggregationTargetChannelTypeAWSBedrockLegacy: {},
	aggregationTargetChannelTypeAWSBedrockNew:    {},
	"openai":           {},
	"azure_openai":     {},
	"google_ai_studio": {},
}

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
	mux.HandleFunc("/api/account/api-tokens", s.protect(s.handleAPITokens))
	mux.HandleFunc("/api/account/api-tokens/", s.protect(s.handleAPITokenAction))
	mux.HandleFunc("/api/v1/key-options", s.protectPublicAPI(s.handlePublicKeyOptions))
	mux.HandleFunc("/api/v1/key-imports", s.protectPublicAPI(s.handlePublicKeyImports))
	mux.HandleFunc("/api/v1/key-imports/", s.protectPublicAPI(s.handlePublicKeyImportAction))
	mux.HandleFunc("/api/v1/", s.protectPublicAPI(func(w http.ResponseWriter, r *http.Request) { apiError(w, 404, "NOT_FOUND", "接口不存在") }))
	mux.HandleFunc("/api/admin/users", s.protectUserManager(s.handleAdminUsers))
	mux.HandleFunc("/api/admin/users/", s.protectUserManager(s.handleAdminUserAction))
	mux.HandleFunc("/api/dashboard/summary", s.protect(s.handleDashboardSummary))
	mux.HandleFunc("/api/categories", s.protect(s.handleCategories))
	mux.HandleFunc("/api/aggregation-targets", s.protect(s.handleAggregationTargets))
	mux.HandleFunc("/api/admin/aggregation-targets", s.protectAdmin(s.handleAdminAggregationTargets))
	mux.HandleFunc("/api/admin/aggregation-targets/check-contract", s.protectAdmin(s.handleAdminAggregationTargetContractCheck))
	mux.HandleFunc("/api/admin/aggregation-targets/check-reverse-admin", s.protectAdmin(s.handleAdminAggregationTargetReverseAdminCheck))
	mux.HandleFunc("/api/admin/aggregation-targets/check-token-supply", s.protectAdmin(s.handleAdminAggregationTargetTokenSupplyCheck))
	mux.HandleFunc("/api/admin/aggregation-targets/", s.protectAdmin(s.handleAdminAggregationTargetAction))
	mux.HandleFunc("/api/model-gaps", s.protect(s.handleModelGaps))
	mux.HandleFunc("/api/admin/model-gaps", s.protectRoot(s.handleAdminModelGaps))
	mux.HandleFunc("/api/admin/model-gaps/", s.protectRoot(s.handleAdminModelGapAction))
	mux.HandleFunc("/api/keys", s.protect(s.handleAPIKeys))
	mux.HandleFunc("/api/keys/", s.protect(s.handleAPIKeyAction))
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
	mux.HandleFunc("/api/usage-logs/configuration", s.protect(s.handleUsageLogConfiguration))
	mux.HandleFunc("/api/usage-logs", s.protect(s.handleUsageLogs))
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
	Code             string   `json:"code,omitempty"`
	Name             string   `json:"name,omitempty"`
	BaseURL          string   `json:"baseUrl,omitempty"`
	ConnectionMode   string   `json:"connectionMode,omitempty"`
	ChannelType      string   `json:"channelType"`
	Models           []string `json:"models,omitempty"`
	ReverseGroupName string   `json:"reverseGroupName,omitempty"`
	TargetCount      int      `json:"targetCount,omitempty"`
}

type adminAggregationTargetResponse struct {
	Code               string    `json:"code"`
	Name               string    `json:"name"`
	BaseURL            string    `json:"baseUrl"`
	ConnectionMode     string    `json:"connectionMode"`
	ChannelType        string    `json:"channelType"`
	Models             []string  `json:"models"`
	Enabled            bool      `json:"enabled"`
	IsSystemDefault    bool      `json:"isSystemDefault"`
	Source             string    `json:"source"`
	HasToken           bool      `json:"hasToken"`
	ReverseUsername    string    `json:"reverseUsername,omitempty"`
	ReverseGroupName   string    `json:"reverseGroupName"`
	HasReversePassword bool      `json:"hasReversePassword"`
	CreatedAt          time.Time `json:"createdAt,omitempty"`
	UpdatedAt          time.Time `json:"updatedAt,omitempty"`
}

type adminAggregationTargetRequest struct {
	Name             string   `json:"name"`
	BaseURL          string   `json:"baseUrl"`
	ConnectionMode   string   `json:"connectionMode"`
	ChannelType      string   `json:"channelType"`
	Models           []string `json:"models"`
	Token            string   `json:"token"`
	ReverseUsername  string   `json:"reverseUsername"`
	ReverseGroupName string   `json:"reverseGroupName"`
	ReversePassword  string   `json:"reversePassword"`
	Enabled          *bool    `json:"enabled"`
	IsSystemDefault  *bool    `json:"isSystemDefault"`
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

type aggregationTargetTokenSupplyCheckRequest struct {
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
	var records []database.AggregationTargetRecord
	var err error
	if s.config.AuthEnabled {
		user := currentAdminUser(r)
		if user == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		records, err = database.ListUserAggregationTargets(r.Context(), s.db, user.Username)
	} else {
		records, err = database.ListAggregationTargets(r.Context(), s.db, true)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(records) > 0 || s.config.AuthEnabled {
		if s.isRegularUserRequest(r) {
			writeJSON(w, http.StatusOK, map[string]any{"items": regularUserAggregationTargetResponses(records)})
			return
		}

		items := make([]aggregationTargetResponse, 0, len(records))
		for _, record := range records {
			items = append(items, aggregationTargetResponse{
				Code:             record.Code,
				Name:             record.Name,
				BaseURL:          record.BaseURL,
				ConnectionMode:   normalizeAggregationTargetConnectionModeOrAPI(record.ConnectionMode),
				ChannelType:      normalizeAggregationTargetChannelTypeOrDefault(record.ChannelType),
				Models:           append([]string(nil), record.Models...),
				ReverseGroupName: normalizeAggregationTargetReverseGroupName(record.ReverseGroupName),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
		return
	}

	items := make([]aggregationTargetResponse, 0, len(s.config.AggregationTargets))
	for _, target := range s.config.AggregationTargets {
		items = append(items, aggregationTargetResponse{
			Code:             target.Code,
			Name:             target.Name,
			BaseURL:          target.BaseURL,
			ConnectionMode:   normalizeAggregationTargetConnectionModeOrAPI(target.ConnectionMode),
			ChannelType:      normalizeAggregationTargetChannelTypeOrDefault(target.ChannelType),
			Models:           append([]string(nil), target.Models...),
			ReverseGroupName: normalizeAggregationTargetReverseGroupName(target.ReverseGroupName),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func regularUserAggregationTargetResponses(records []database.AggregationTargetRecord) []aggregationTargetResponse {
	countsByChannelType := make(map[string]int)
	modelsByChannelType := make(map[string][]string)
	channelTypes := make([]string, 0)
	for _, record := range records {
		channelType := normalizeAggregationTargetChannelTypeOrDefault(record.ChannelType)
		if countsByChannelType[channelType] == 0 {
			channelTypes = append(channelTypes, channelType)
			modelsByChannelType[channelType] = cleanModelList(record.Models)
		}
		countsByChannelType[channelType]++
	}
	items := make([]aggregationTargetResponse, 0, len(channelTypes))
	for _, channelType := range channelTypes {
		items = append(items, aggregationTargetResponse{
			ChannelType: channelType,
			Models:      append([]string(nil), modelsByChannelType[channelType]...),
			TargetCount: countsByChannelType[channelType],
		})
	}
	return items
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
		writeError(w, http.StatusBadRequest, fmt.Sprintf("接收平台必须实现 GET /api/keyhub/channels/usage，否则不允许接入: %v", err))
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
	defer func() {
		logoutCtx, logoutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer logoutCancel()
		_ = client.Close(logoutCtx)
	}()
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

func (s *Server) handleAdminAggregationTargetTokenSupplyCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var request aggregationTargetTokenSupplyCheckRequest
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
	username, password, err := s.resolveAggregationTargetUsernamePassword(
		r.Context(),
		code,
		strings.TrimSpace(request.ReverseUsername),
		strings.TrimSpace(request.ReversePassword),
		"tokensupply",
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	client, err := tokensupply.NewClient(baseURL, username, password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	checkCtx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	result, err := client.CheckAccount(checkCtx)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	_ = database.InsertAuditLog(r.Context(), s.db, actorFromRequest(r), "aggregation_target.check_token_supply", "aggregation_targets", nil, map[string]any{
		"code":             code,
		"baseUrl":          baseURL,
		"username":         username,
		"purchaseProvider": result.PurchaseProvider,
		"providerCount":    result.ProviderCount,
		"apiKeyCount":      result.APIKeyCount,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"success":          true,
		"username":         result.Username,
		"purchaseProvider": result.PurchaseProvider,
		"providerCount":    result.ProviderCount,
		"apiKeyCount":      result.APIKeyCount,
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

	code := strings.TrimSpace(pathCode)
	isCreate := code == ""
	name := strings.TrimSpace(request.Name)
	baseURL := strings.TrimRight(strings.TrimSpace(request.BaseURL), "/")
	connectionMode, err := normalizeAggregationTargetConnectionMode(request.ConnectionMode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	channelType, err := normalizeAggregationTargetChannelType(request.ChannelType)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	models := cleanModelList(request.Models)
	if len(models) == 0 {
		writeError(w, http.StatusBadRequest, "请至少配置一个模型")
		return
	}
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}

	var existing database.AggregationTargetRecord
	if isCreate {
		code, err = s.generateAggregationTargetCode(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		existing, err = database.GetAggregationTarget(r.Context(), s.db, code)
		if errors.Is(err, database.ErrAggregationTargetNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	isSystemDefault := false
	if request.IsSystemDefault != nil {
		isSystemDefault = *request.IsSystemDefault
	} else if !isCreate {
		isSystemDefault = existing.IsSystemDefault
	}
	if isSystemDefault && connectionMode != aggregationTargetConnectionModeNewAPIReverse {
		writeError(w, http.StatusBadRequest, "系统默认接收平台必须使用 new-api逆向 接入方式")
		return
	}
	if err := validateAggregationTargetInput(code, name, baseURL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	tokenCiphertext := existing.TokenCiphertext
	reverseUsername := strings.TrimSpace(request.ReverseUsername)
	reverseGroupName := normalizeAggregationTargetReverseGroupName(request.ReverseGroupName)
	reversePasswordCiphertext := existing.ReversePasswordCiphertext
	if !isCreate && normalizeAggregationTargetConnectionModeOrAPI(existing.ConnectionMode) != connectionMode {
		tokenCiphertext = ""
		reversePasswordCiphertext = ""
	}
	if connectionMode == aggregationTargetConnectionModeAPI {
		reverseUsername = ""
		reverseGroupName = "default"
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
			err = fmt.Errorf("接收平台必须实现 GET /api/keyhub/channels/usage，否则不允许接入: %w", err)
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
		if len([]rune(reverseGroupName)) > 64 {
			writeError(w, http.StatusBadRequest, "new-api 逆向分组名称最多 64 个字符")
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
	if err := database.SaveAggregationTarget(r.Context(), s.db, database.AggregationTargetUpsert{
		Code:                      code,
		Name:                      name,
		BaseURL:                   baseURL,
		ConnectionMode:            connectionMode,
		ChannelType:               channelType,
		Models:                    models,
		TokenCiphertext:           tokenCiphertext,
		ReverseUsername:           reverseUsername,
		ReverseGroupName:          reverseGroupName,
		ReversePasswordCiphertext: reversePasswordCiphertext,
		Enabled:                   enabled,
		IsSystemDefault:           isSystemDefault,
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
		"code":             code,
		"baseUrl":          baseURL,
		"connectionMode":   connectionMode,
		"channelType":      channelType,
		"models":           models,
		"reverseGroupName": reverseGroupName,
		"enabled":          enabled,
		"isSystemDefault":  isSystemDefault,
	})
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "code": code})
}

func (s *Server) generateAggregationTargetCode(ctx context.Context) (string, error) {
	for attempt := 0; attempt < 5; attempt++ {
		code, err := newAggregationTargetCode()
		if err != nil {
			return "", err
		}
		if _, err = database.GetAggregationTarget(ctx, s.db, code); errors.Is(err, database.ErrAggregationTargetNotFound) {
			return code, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("generate unique aggregation target ID")
}

func (s *Server) listAdminAggregationTargets(ctx context.Context) ([]adminAggregationTargetResponse, error) {
	records, err := database.ListAggregationTargets(ctx, s.db, false)
	if err != nil {
		return nil, err
	}
	if len(records) > 0 {
		items := make([]adminAggregationTargetResponse, 0, len(records))
		for _, record := range records {
			items = append(items, adminAggregationTargetResponse{
				Code:               record.Code,
				Name:               record.Name,
				BaseURL:            record.BaseURL,
				ConnectionMode:     normalizeAggregationTargetConnectionModeOrAPI(record.ConnectionMode),
				ChannelType:        normalizeAggregationTargetChannelTypeOrDefault(record.ChannelType),
				Models:             append([]string(nil), record.Models...),
				Enabled:            record.Enabled,
				IsSystemDefault:    record.IsSystemDefault,
				Source:             record.Source,
				HasToken:           record.HasToken,
				ReverseUsername:    record.ReverseUsername,
				ReverseGroupName:   normalizeAggregationTargetReverseGroupName(record.ReverseGroupName),
				HasReversePassword: record.HasReversePassword,
				CreatedAt:          record.CreatedAt,
				UpdatedAt:          record.UpdatedAt,
			})
		}
		return items, nil
	}

	items := make([]adminAggregationTargetResponse, 0, len(s.config.AggregationTargets))
	for _, target := range s.config.AggregationTargets {
		items = append(items, adminAggregationTargetResponse{
			Code:               target.Code,
			Name:               target.Name,
			BaseURL:            target.BaseURL,
			ConnectionMode:     normalizeAggregationTargetConnectionModeOrAPI(target.ConnectionMode),
			ChannelType:        normalizeAggregationTargetChannelTypeOrDefault(target.ChannelType),
			Models:             append([]string(nil), target.Models...),
			Enabled:            true,
			IsSystemDefault:    false,
			Source:             "env",
			HasToken:           target.Token != "",
			ReverseUsername:    target.ReverseUsername,
			ReverseGroupName:   normalizeAggregationTargetReverseGroupName(target.ReverseGroupName),
			HasReversePassword: target.ReversePassword != "",
		})
	}
	return items, nil
}

func (s *Server) validateAggregationTargetCapabilities(ctx context.Context, baseURL string, token string) error {
	if _, err := s.checkAggregationTargetCapabilities(ctx, baseURL, token); err != nil {
		return fmt.Errorf("接收平台必须实现 GET /api/keyhub/channels/usage，否则不允许接入: %w", err)
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

func (s *Server) resolveAggregationTargetUsernamePassword(ctx context.Context, code string, username string, password string, label string) (string, string, error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	label = strings.TrimSpace(label)
	if label == "" {
		label = "aggregation target"
	}
	if username == "" {
		record, err := database.GetAggregationTarget(ctx, s.db, strings.TrimSpace(code))
		if err == nil {
			username = strings.TrimSpace(record.ReverseUsername)
		} else if !errors.Is(err, database.ErrAggregationTargetNotFound) {
			return "", "", err
		}
	}
	if username == "" {
		return "", "", fmt.Errorf("%s username is required", label)
	}
	if password != "" {
		return username, password, nil
	}
	record, err := database.GetAggregationTarget(ctx, s.db, strings.TrimSpace(code))
	if errors.Is(err, database.ErrAggregationTargetNotFound) {
		return "", "", fmt.Errorf("%s password is required", label)
	}
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(record.ReversePasswordCiphertext) == "" {
		return "", "", fmt.Errorf("%s password is required", label)
	}
	encryptor, err := security.NewEncryptor(s.config.EncryptionKey)
	if err != nil {
		return "", "", err
	}
	password, err = encryptor.Decrypt(record.ReversePasswordCiphertext)
	if err != nil {
		return "", "", fmt.Errorf("decrypt aggregation target password: %w", err)
	}
	return username, password, nil
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
	case aggregationTargetConnectionModeAPI, aggregationTargetConnectionModeNewAPIReverse, aggregationTargetConnectionModeTokenSupply:
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

func normalizeAggregationTargetChannelType(channelType string) (string, error) {
	channelType = strings.ToLower(strings.TrimSpace(channelType))
	if channelType == "" {
		return aggregationTargetDefaultChannelType, nil
	}
	if _, ok := aggregationTargetChannelTypes[channelType]; !ok {
		return "", fmt.Errorf("unsupported aggregation target channelType: %s", channelType)
	}
	return channelType, nil
}

func normalizeAggregationTargetChannelTypeOrDefault(channelType string) string {
	normalized, err := normalizeAggregationTargetChannelType(channelType)
	if err != nil {
		return aggregationTargetDefaultChannelType
	}
	return normalized
}

func normalizeAWSCredentialType(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "", nil
	case awsCredentialTypeAKSK:
		return awsCredentialTypeAKSK, nil
	case awsCredentialTypeAPIKey:
		return awsCredentialTypeAPIKey, nil
	default:
		return "", fmt.Errorf("awsCredentialType must be ak_sk or api_key")
	}
}

func validateAWSCredentialLines(items []keys.ParsedKey, credentialType string) error {
	wantParts := 3
	format := "AccessKey|SecretAccessKey|Region"
	if credentialType == awsCredentialTypeAPIKey {
		wantParts = 2
		format = "APIKey|Region"
	}
	for _, item := range items {
		if item.Error != "" {
			continue
		}
		parts := strings.Split(item.Normalized, "|")
		valid := len(parts) == wantParts
		for _, part := range parts {
			if strings.TrimSpace(part) == "" {
				valid = false
				break
			}
		}
		if !valid {
			return fmt.Errorf("第 %d 行不符合 AWS Bedrock %s 格式：%s", item.LineNumber, credentialType, format)
		}
	}
	return nil
}

func normalizeAggregationTargetReverseGroupName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "default"
	}
	return value
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

func (s *Server) handleAPIKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	user := currentAdminUser(r)
	includePushBindings := !s.config.AuthEnabled || user != nil && isAdminRole(user.Role)
	owners, err := s.keyOwnerFilters(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result, err := database.ListAPIKeysForOwners(r.Context(), s.db, limit, offset, owners, includePushBindings)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.isRegularUserRequest(r) {
		for index := range result.Items {
			result.Items[index].UsageQuota30d = 0
		}
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
		if !s.requireAdmin(w, r) {
			return
		}
		s.handleKeyDelete(w, r, keyID)
		return
	}

	switch parts[3] {
	case "plaintext":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !s.requireAdmin(w, r) {
			return
		}
		s.handleKeyPlaintext(w, r, keyID)
	case "activate-associated":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleKeyActivateAssociated(w, r, keyID)
	case "activate":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !s.requireAdmin(w, r) {
			return
		}
		s.handleKeyActivate(w, r, keyID)
	case "retry-push":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !s.requireAdmin(w, r) {
			return
		}
		s.handleKeyRetryPush(w, r, keyID)
	case "disable":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !s.requireAdmin(w, r) {
			return
		}
		s.handleKeyDisable(w, r, keyID)
	default:
		writeError(w, http.StatusNotFound, "api route not found")
	}
}

func (s *Server) handleKeyPlaintext(w http.ResponseWriter, r *http.Request, keyID int64) {
	record, err := database.LoadAPIKeySecret(r.Context(), s.db, keyID)
	if errors.Is(err, database.ErrAPIKeyNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	encryptor, err := security.NewEncryptor(s.config.EncryptionKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "密钥解密服务不可用")
		return
	}
	plaintext, err := encryptor.Decrypt(record.KeyCiphertext)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "数据库中的密钥无法解密")
		return
	}

	if err := database.InsertAuditLog(r.Context(), s.db, actorFromRequest(r), "key.plaintext.view", "api_keys", &keyID, map[string]any{
		"categoryCode": record.CategoryCode,
		"keyHint":      record.KeyHint,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "记录密钥查看审计日志失败")
		return
	}

	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusOK, map[string]any{
		"id":        record.ID,
		"plaintext": plaintext,
	})
}

type keyImportRequest struct {
	RawText           string   `json:"rawText"`
	ChannelTypes      []string `json:"channelTypes"`
	AWSCredentialType string   `json:"awsCredentialType"`
	AWSBedrockRegions []string `json:"awsBedrockRegions"`
	Models            []string `json:"models"`
	Tag               string   `json:"tag"`
	GroupName         string   `json:"groupName"`
	Note              string   `json:"note"`
	ExpectedTPM       int64    `json:"expectedTpm"`
}

type keyActivateRequest struct {
	TargetCode string `json:"targetCode"`
}

type keyActivateAssociatedRequest struct {
	ChannelTypes []string `json:"channelTypes"`
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
	authorizedTargets, err := s.enabledAggregationTargetsForOwner(r.Context(), actorFromRequest(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(authorizedTargets) == 0 {
		writeError(w, http.StatusForbidden, "当前用户未关联任何已启用的接收平台，无权上传密钥")
		return
	}
	selectedTargets, err := filterAggregationTargetsByChannelTypes(authorizedTargets, request.ChannelTypes)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	awsCredentialType, err := normalizeAWSCredentialType(request.AWSCredentialType)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	requiresAWSCredentialType := false
	requiresAWSBedrockRegions := false
	for _, target := range selectedTargets {
		channelType := normalizeAggregationTargetChannelTypeOrDefault(target.ChannelType)
		if channelType == aggregationTargetChannelTypeAWSBedrockLegacy {
			requiresAWSCredentialType = true
		}
		if channelType == aggregationTargetChannelTypeAWSBedrockNew {
			requiresAWSBedrockRegions = true
		}
	}
	if requiresAWSCredentialType && awsCredentialType == "" {
		writeError(w, http.StatusBadRequest, "旧版 AWS Bedrock 密钥必须选择密钥类型")
		return
	}
	awsBedrockRegions := []string(nil)
	if requiresAWSBedrockRegions {
		awsBedrockRegions, err = normalizeAWSBedrockNewRegions(request.AWSBedrockRegions)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	modelOverrides := cleanModelList(request.Models)

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

	parsedItems, err := s.parseKeys(r.Context(), request.RawText, awsBedrockRegions...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(parsedItems) == 0 {
		writeError(w, http.StatusBadRequest, "请至少填写一行密钥")
		return
	}
	if awsCredentialType != "" {
		if err := validateAWSCredentialLines(parsedItems, awsCredentialType); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	parseErrors := make([]string, 0)
	for _, item := range parsedItems {
		if item.Error != "" && !item.Duplicate {
			parseErrors = append(parseErrors, fmt.Sprintf("第 %d 行：%s", item.LineNumber, item.Error))
		}
	}
	if len(parseErrors) > 0 {
		message := strings.Join(parseErrors[:min(len(parseErrors), 3)], "；")
		if len(parseErrors) > 3 {
			message = fmt.Sprintf("%s；另有 %d 行格式错误", message, len(parseErrors)-3)
		}
		writeError(w, http.StatusBadRequest, message)
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
		rows = append(rows, database.ImportAPIKey{
			CategoryCode:      "",
			AWSCredentialType: awsCredentialType,
			Ciphertext:        ciphertext,
			Fingerprint:       item.Fingerprint,
			KeyHint:           item.KeyHint,
			Region:            item.Region,
			BaseURL:           item.BaseURL,
			Models:            append([]string(nil), modelOverrides...),
			Tag:               strings.TrimSpace(request.Tag),
			GroupName:         groupName,
			Note:              strings.TrimSpace(request.Note),
			ExpectedTPM:       request.ExpectedTPM,
			Status:            status,
		})
	}

	result, err := database.ImportKeys(
		r.Context(),
		s.db,
		"",
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
	if strings.TrimSpace(request.TargetCode) == "" {
		writeError(w, http.StatusBadRequest, "targetCode is required")
		return
	}
	if err := s.validateKeyTargetPermission(r.Context(), keyID, request.TargetCode); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	newAPIChannelID, targetCode, err := s.activateKey(r.Context(), keyID, request.TargetCode)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"newApiChannelId": newAPIChannelID, "targetCode": targetCode})
}

func (s *Server) handleKeyActivateAssociated(w http.ResponseWriter, r *http.Request, keyID int64) {
	request := keyActivateAssociatedRequest{}
	if r.Body != nil {
		if err := readJSON(w, r, &request); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	owner := s.keyOwnerFilter(r)
	owned, err := database.APIKeyBelongsToOwner(r.Context(), s.db, keyID, owner)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !owned {
		writeError(w, http.StatusNotFound, database.ErrAPIKeyNotFound.Error())
		return
	}

	keyRecord, err := database.LoadAPIKeyForSync(r.Context(), s.db, keyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	targets, err := s.enabledAggregationTargetsForOwner(r.Context(), keyRecord.CreatedBy)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(targets) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"activated": false,
			"skipped":   true,
			"total":     0,
			"success":   0,
			"failed":    0,
		})
		return
	}
	targets, err = filterAggregationTargetsByChannelTypes(targets, request.ChannelTypes)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	success := 0
	failed := 0
	newAPIChannelIDs := make([]int64, 0, len(targets))
	for _, target := range targets {
		newAPIChannelID, _, err := s.activateKey(r.Context(), keyID, target.Code)
		if err != nil {
			failed++
			continue
		}
		success++
		if newAPIChannelID > 0 {
			newAPIChannelIDs = append(newAPIChannelIDs, newAPIChannelID)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"activated":        success > 0,
		"skipped":          false,
		"total":            len(targets),
		"success":          success,
		"failed":           failed,
		"newApiChannelIds": newAPIChannelIDs,
	})
}

func (s *Server) handleKeyRetryPush(w http.ResponseWriter, r *http.Request, keyID int64) {
	keyRecord, err := database.LoadAPIKeyForSync(r.Context(), s.db, keyID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, database.ErrAPIKeyNotFound.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if keyRecord.Status != "inventory" {
		writeError(w, http.StatusConflict, "只有库存状态的密钥可以重新推送")
		return
	}

	availableTargets, err := s.enabledAggregationTargetsForOwner(r.Context(), keyRecord.CreatedBy)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	failedTargetCodes, err := database.ListFailedActivationTargetCodes(r.Context(), s.db, keyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	availableByCode := make(map[string]config.AggregationTarget, len(availableTargets))
	for _, target := range availableTargets {
		availableByCode[target.Code] = target
	}
	targets := make([]config.AggregationTarget, 0, len(availableTargets))
	for _, targetCode := range failedTargetCodes {
		if target, ok := availableByCode[targetCode]; ok {
			targets = append(targets, target)
		}
	}
	source := "failed_events"
	if len(targets) == 0 {
		targets = availableTargets
		source = "current_associations"
	}

	if len(targets) == 0 {
		_ = database.InsertAuditLog(r.Context(), s.db, actorFromRequest(r), "key.retry_push", "api_keys", &keyID, map[string]any{
			"source":      source,
			"targetCodes": []string{},
			"success":     0,
			"failed":      0,
			"skipped":     true,
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"activated":   false,
			"skipped":     true,
			"total":       0,
			"success":     0,
			"failed":      0,
			"targetCodes": []string{},
			"source":      source,
		})
		return
	}

	success := 0
	failed := 0
	targetCodes := make([]string, 0, len(targets))
	newAPIChannelIDs := make([]int64, 0, len(targets))
	for _, target := range targets {
		targetCodes = append(targetCodes, target.Code)
		newAPIChannelID, _, activateErr := s.activateKey(r.Context(), keyID, target.Code)
		if activateErr != nil {
			failed++
			continue
		}
		success++
		if newAPIChannelID > 0 {
			newAPIChannelIDs = append(newAPIChannelIDs, newAPIChannelID)
		}
	}
	_ = database.InsertAuditLog(r.Context(), s.db, actorFromRequest(r), "key.retry_push", "api_keys", &keyID, map[string]any{
		"source":      source,
		"targetCodes": targetCodes,
		"success":     success,
		"failed":      failed,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"activated":        success > 0,
		"skipped":          false,
		"total":            len(targets),
		"success":          success,
		"failed":           failed,
		"targetCodes":      targetCodes,
		"newApiChannelIds": newAPIChannelIDs,
		"source":           source,
	})
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
	release, locked, lockErr := database.AcquireAPILock(r.Context(), s.db, fmt.Sprintf("key-operation:%d", keyID), 5)
	if lockErr != nil {
		writeError(w, http.StatusInternalServerError, "密钥操作暂不可用")
		return
	}
	if !locked {
		writeError(w, http.StatusConflict, "密钥正在处理中，请稍后重试")
		return
	}
	defer release()
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
			{"code": aggregationTargetChannelTypeAWSBedrockLegacy, "label": "旧版 AWS Bedrock", "newApiType": 33, "keyFormat": "AccessKey|SecretKey|Region"},
			{"code": aggregationTargetChannelTypeAWSBedrockNew, "label": "新版 AWS Bedrock", "newApiType": 14, "keyFormat": "sk-ant-..."},
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
			target.ChannelType = normalizeAggregationTargetChannelTypeOrDefault(target.ChannelType)
			targets = append(targets, target)
		}
		return targets, nil
	}

	targets := make([]config.AggregationTarget, 0, len(records))
	for _, record := range records {
		targets = append(targets, config.AggregationTarget{
			Code:             record.Code,
			Name:             record.Name,
			BaseURL:          record.BaseURL,
			ConnectionMode:   normalizeAggregationTargetConnectionModeOrAPI(record.ConnectionMode),
			ChannelType:      normalizeAggregationTargetChannelTypeOrDefault(record.ChannelType),
			Models:           append([]string(nil), record.Models...),
			ReverseGroupName: normalizeAggregationTargetReverseGroupName(record.ReverseGroupName),
		})
	}
	return targets, nil
}

func (s *Server) enabledAggregationTargetsForOwner(ctx context.Context, username string) ([]config.AggregationTarget, error) {
	if !s.config.AuthEnabled {
		return s.listEnabledAggregationTargetOptions(ctx)
	}
	records, err := database.ListUserAggregationTargets(ctx, s.db, strings.TrimSpace(username))
	if err != nil {
		return nil, err
	}
	targets := make([]config.AggregationTarget, 0, len(records))
	for _, record := range records {
		targets = append(targets, config.AggregationTarget{
			Code:             record.Code,
			Name:             record.Name,
			BaseURL:          record.BaseURL,
			ConnectionMode:   normalizeAggregationTargetConnectionModeOrAPI(record.ConnectionMode),
			ChannelType:      normalizeAggregationTargetChannelTypeOrDefault(record.ChannelType),
			Models:           append([]string(nil), record.Models...),
			ReverseUsername:  record.ReverseUsername,
			ReverseGroupName: normalizeAggregationTargetReverseGroupName(record.ReverseGroupName),
		})
	}
	return targets, nil
}

func filterAggregationTargetsByChannelTypes(
	targets []config.AggregationTarget,
	requestedChannelTypes []string,
) ([]config.AggregationTarget, error) {
	if len(requestedChannelTypes) == 0 {
		return targets, nil
	}

	availableChannelTypes := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		availableChannelTypes[normalizeAggregationTargetChannelTypeOrDefault(target.ChannelType)] = struct{}{}
	}

	selectedChannelTypes := make(map[string]struct{}, len(requestedChannelTypes))
	for _, requestedChannelType := range requestedChannelTypes {
		if strings.TrimSpace(requestedChannelType) == "" {
			return nil, fmt.Errorf("channelTypes contains an empty channel type")
		}
		channelType, err := normalizeAggregationTargetChannelType(requestedChannelType)
		if err != nil {
			return nil, err
		}
		if _, ok := availableChannelTypes[channelType]; !ok {
			return nil, fmt.Errorf("当前用户未关联渠道类型 %s 的已启用接收平台", channelType)
		}
		selectedChannelTypes[channelType] = struct{}{}
	}

	filteredTargets := make([]config.AggregationTarget, 0, len(targets))
	for _, target := range targets {
		channelType := normalizeAggregationTargetChannelTypeOrDefault(target.ChannelType)
		if _, ok := selectedChannelTypes[channelType]; ok {
			filteredTargets = append(filteredTargets, target)
		}
	}
	return filteredTargets, nil
}

func (s *Server) validateKeyTargetPermission(ctx context.Context, keyID int64, targetCode string) error {
	if !s.config.AuthEnabled {
		return nil
	}
	keyRecord, err := database.LoadAPIKeyForSync(ctx, s.db, keyID)
	if err != nil {
		return err
	}
	allowed, err := database.UserCanUseAggregationTarget(ctx, s.db, keyRecord.CreatedBy, targetCode)
	if err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf("密钥所属用户未关联已启用的接收平台 %s", strings.TrimSpace(targetCode))
	}
	return nil
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
			target.ChannelType = normalizeAggregationTargetChannelTypeOrDefault(target.ChannelType)
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
			Code:             record.Code,
			Name:             record.Name,
			BaseURL:          record.BaseURL,
			ConnectionMode:   connectionMode,
			ChannelType:      normalizeAggregationTargetChannelTypeOrDefault(record.ChannelType),
			Models:           append([]string(nil), record.Models...),
			ReverseGroupName: normalizeAggregationTargetReverseGroupName(record.ReverseGroupName),
		}
		switch connectionMode {
		case aggregationTargetConnectionModeAPI:
			token, err := encryptor.Decrypt(record.TokenCiphertext)
			if err != nil {
				return nil, fmt.Errorf("decrypt aggregation target %s token: %w", record.Code, err)
			}
			target.Token = token
		case aggregationTargetConnectionModeNewAPIReverse, aggregationTargetConnectionModeTokenSupply:
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
	if code == "" {
		return config.AggregationTarget{}, fmt.Errorf("aggregation target code is required")
	}
	for _, target := range targets {
		if target.Code == code {
			return target, nil
		}
	}
	return config.AggregationTarget{}, fmt.Errorf("unknown aggregation target: %s", code)
}

func (s *Server) buildTargetChannelPayload(ctx context.Context, keyRecord database.APIKeyForSync, plaintextKey string, target config.AggregationTarget) (newapi.ChannelPayload, error) {
	channelType := normalizeAggregationTargetChannelTypeOrDefault(target.ChannelType)
	payloadCategoryCode := aggregationTargetPayloadCategoryCode(channelType)
	category, err := database.LoadCategory(ctx, s.db, payloadCategoryCode)
	if err != nil {
		return newapi.ChannelPayload{}, fmt.Errorf("load receiver channel type %s via %s: %w", channelType, payloadCategoryCode, err)
	}
	models := channelModelsForTarget(keyRecord.Models, target.Models)
	if len(models) == 0 {
		return newapi.ChannelPayload{}, fmt.Errorf("receiver platform %s has no models configured", target.Code)
	}
	payload := buildNewAPIChannelPayload(keyRecord, plaintextKey, channelType, category.NewAPIType, models)
	return applyAWSBedrockNewRegion(target, keyRecord, channelType, payload), nil
}

func channelModelsForTarget(keyModels []string, targetModels []string) []string {
	if models := cleanModelList(keyModels); len(models) > 0 {
		return models
	}
	return cleanModelList(targetModels)
}

func applyAWSBedrockNewRegion(target config.AggregationTarget, keyRecord database.APIKeyForSync, channelType string, payload newapi.ChannelPayload) newapi.ChannelPayload {
	if normalizeAggregationTargetConnectionModeOrAPI(target.ConnectionMode) == aggregationTargetConnectionModeNewAPIReverse &&
		normalizeAggregationTargetChannelTypeOrDefault(channelType) == aggregationTargetChannelTypeAWSBedrockNew {
		payload.BaseURL = strings.TrimSpace(keyRecord.BaseURL)
	}
	return payload
}

func aggregationTargetPayloadCategoryCode(channelType string) string {
	channelType = normalizeAggregationTargetChannelTypeOrDefault(channelType)
	if channelType == aggregationTargetChannelTypeAWSBedrockNew {
		return aggregationTargetDefaultChannelType
	}
	return channelType
}

func buildNewAPIChannelPayload(keyRecord database.APIKeyForSync, plaintextKey string, channelType string, newAPIType int, models []string) newapi.ChannelPayload {
	autoBan := 1
	priority := int64(0)
	weight := uint(0)
	channelKey := strings.TrimSpace(plaintextKey)
	channelModels := channelModelsForPayload(models)
	group := strings.TrimSpace(keyRecord.GroupName)
	if group == "" {
		group = "default"
	}
	payload := newapi.ChannelPayload{
		Type:          newAPIType,
		Key:           channelKey,
		Status:        newapi.ChannelStatusEnabled,
		Name:          buildChannelName(keyRecord, channelType),
		Models:        strings.Join(channelModels, ","),
		ModelMapping:  buildAWSBedrockNewModelMapping(channelType, channelModels),
		Group:         group,
		OtherSettings: buildOtherSettings(channelType, keyRecord.AWSCredentialType, plaintextKey),
		AutoBan:       &autoBan,
		Priority:      &priority,
		Weight:        &weight,
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

func buildAWSBedrockNewModelMapping(channelType string, models []string) string {
	if normalizeAggregationTargetChannelTypeOrDefault(channelType) != aggregationTargetChannelTypeAWSBedrockNew {
		return ""
	}
	mapping := make(map[string]string, len(models))
	for _, model := range cleanModelList(models) {
		mapping[model] = "anthropic." + model
	}
	if len(mapping) == 0 {
		return ""
	}
	payload, err := json.Marshal(mapping)
	if err != nil {
		return ""
	}
	return string(payload)
}

func channelModelsForPayload(models []string) []string {
	return cleanModelList(models)
}

func buildChannelName(keyRecord database.APIKeyForSync, channelType string) string {
	username := strings.TrimSpace(keyRecord.CreatedBy)
	if username == "" {
		username = "keyhub"
	}
	channelType = normalizeAggregationTargetChannelTypeOrDefault(channelType)
	if channelType == "" {
		channelType = aggregationTargetDefaultChannelType
	}
	return fmt.Sprintf("%s_%s", username, channelType)
}

func buildOtherSettings(categoryCode string, awsCredentialType string, plaintextKey string) string {
	if categoryCode != aggregationTargetChannelTypeAWSBedrockLegacy && categoryCode != "claude_on_aws" {
		return "{}"
	}
	awsCredentialType, err := normalizeAWSCredentialType(awsCredentialType)
	if err != nil || awsCredentialType == "" {
		awsCredentialType = awsCredentialTypeAKSK
		if len(strings.Split(plaintextKey, "|")) == 2 {
			awsCredentialType = awsCredentialTypeAPIKey
		}
	}
	payload, _ := json.Marshal(map[string]string{"aws_key_type": awsCredentialType})
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
		"modelMapping": payload.ModelMapping,
		"group":        payload.Group,
		"baseUrl":      payload.BaseURL,
		"tag":          keyRecord.Tag,
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

func cleanModelList(models []string) []string {
	cleaned := make([]string, 0, len(models))
	seen := make(map[string]bool, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" || seen[model] {
			continue
		}
		seen[model] = true
		cleaned = append(cleaned, model)
	}
	return cleaned
}

func normalizeAWSBedrockNewRegions(values []string) ([]string, error) {
	regions := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		region := strings.TrimSpace(value)
		if region == "" {
			continue
		}
		if !strings.HasPrefix(region, awsBedrockNewRegionPrefix) {
			return nil, fmt.Errorf("区域第 %d 行必须以 %s 开头", index+1, awsBedrockNewRegionPrefix)
		}
		if _, exists := seen[region]; exists {
			continue
		}
		seen[region] = struct{}{}
		regions = append(regions, region)
	}
	if len(regions) == 0 {
		return nil, fmt.Errorf("新版 AWS Bedrock 请至少填写一行区域")
	}
	return regions, nil
}

func expandParsedKeysByBaseURLs(items []keys.ParsedKey, baseURLs []string) []keys.ParsedKey {
	if len(baseURLs) == 0 {
		return items
	}
	expanded := make([]keys.ParsedKey, 0, len(items)*len(baseURLs))
	for _, item := range items {
		if item.Error != "" {
			expanded = append(expanded, item)
			continue
		}
		for _, baseURL := range baseURLs {
			cloned := item
			cloned.BaseURL = baseURL
			cloned.Fingerprint = keys.Fingerprint(aggregationTargetChannelTypeAWSBedrockNew+"\x00"+baseURL, item.Normalized)
			expanded = append(expanded, cloned)
		}
	}
	return expanded
}

func (s *Server) parseKeys(ctx context.Context, rawText string, baseURLs ...string) ([]keys.ParsedKey, error) {
	items := keys.ParseLines(rawText)
	items = expandParsedKeysByBaseURLs(items, baseURLs)
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
