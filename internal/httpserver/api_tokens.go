package httpserver

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"keyhub/internal/database"
	"keyhub/internal/security"
)

const apiTokenContextKey authContextKey = "api_token_id"

func apiError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func (s *Server) protectPublicAPI(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !s.config.AuthEnabled {
			apiError(w, 503, "AUTH_REQUIRED", "开放 API 需要启用登录鉴权")
			return
		}
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || !validAPIToken(parts[1]) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			apiError(w, 401, "INVALID_TOKEN", "请提供有效的 KeyHub API Token")
			return
		}
		user, tokenID, err := database.AuthenticateAPIToken(r.Context(), s.db, security.SessionTokenHash(parts[1]))
		if errors.Is(err, sql.ErrNoRows) {
			apiError(w, 401, "INVALID_TOKEN", "Token 无效、已过期、已撤销或用户已禁用")
			return
		}
		if err != nil {
			apiError(w, 500, "INTERNAL_ERROR", "鉴权服务暂不可用")
			return
		}
		bucket, limit := "read", 120
		if r.Method != http.MethodGet {
			bucket, limit = "write", 10
		}
		allowed, err := database.AllowAPIRequest(r.Context(), s.db, user.ID, bucket, limit)
		if err != nil {
			apiError(w, 500, "INTERNAL_ERROR", "限流服务暂不可用")
			return
		}
		if !allowed {
			w.Header().Set("Retry-After", "60")
			apiError(w, 429, "RATE_LIMITED", "请求过于频繁，请稍后重试")
			return
		}
		ctx := context.WithValue(r.Context(), adminUserContextKey, user)
		ctx = context.WithValue(ctx, apiTokenContextKey, tokenID)
		next(w, r.WithContext(ctx))
	}
}

func validAPIToken(token string) bool {
	if !strings.HasPrefix(token, "kh_") {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, "kh_"))
	return err == nil && len(decoded) == 32
}

func (s *Server) handleAPITokens(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, private")
	user := currentAdminUser(r)
	if !s.config.AuthEnabled || user == nil {
		apiError(w, 403, "AUTH_REQUIRED", "请先启用鉴权并登录真实用户账号")
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := database.ListAPITokens(r.Context(), s.db, user.ID)
		if err != nil {
			apiError(w, 500, "INTERNAL_ERROR", "读取 Token 列表失败")
			return
		}
		writeJSON(w, 200, map[string]any{"items": items})
	case http.MethodPost:
		var request struct {
			Name          string `json:"name"`
			ExpiresInDays int    `json:"expiresInDays"`
		}
		if err := readAPIJSON(w, r, &request); err != nil {
			apiError(w, 400, "INVALID_REQUEST", "请求必须为合法 JSON，且只能包含 name、expiresInDays")
			return
		}
		request.Name = strings.TrimSpace(request.Name)
		if request.ExpiresInDays == 0 {
			request.ExpiresInDays = 90
		}
		if request.Name == "" || len([]rune(request.Name)) > 64 || request.ExpiresInDays < 1 || request.ExpiresInDays > 365 {
			apiError(w, 400, "INVALID_REQUEST", "名称限 1–64 字符，有效期为 1–365 天")
			return
		}
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			apiError(w, 500, "INTERNAL_ERROR", "生成 Token 失败")
			return
		}
		token := "kh_" + base64.RawURLEncoding.EncodeToString(raw)
		expires := time.Now().UTC().Add(time.Duration(request.ExpiresInDays) * 24 * time.Hour)
		id, err := database.CreateAPIToken(r.Context(), s.db, user.ID, request.Name, security.SessionTokenHash(token), token[:12], expires)
		if errors.Is(err, database.ErrAPITokenLimit) {
			apiError(w, 409, "TOKEN_LIMIT", "最多保留 10 个有效 Token，请先撤销不用的 Token")
			return
		}
		if err != nil {
			apiError(w, 500, "INTERNAL_ERROR", "创建 Token 失败")
			return
		}
		writeJSON(w, 201, map[string]any{"id": id, "token": token, "expiresAt": expires, "scope": "keys:import"})
	default:
		apiError(w, 405, "METHOD_NOT_ALLOWED", "仅支持 GET 和 POST")
	}
}

func (s *Server) handleAPITokenAction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, private")
	user := currentAdminUser(r)
	if !s.config.AuthEnabled || user == nil {
		apiError(w, 403, "AUTH_REQUIRED", "请先登录")
		return
	}
	if r.Method != http.MethodDelete {
		apiError(w, 405, "METHOD_NOT_ALLOWED", "仅支持 DELETE")
		return
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/account/api-tokens/"), 10, 64)
	if err != nil || id <= 0 {
		apiError(w, 404, "NOT_FOUND", "Token 不存在")
		return
	}
	err = database.RevokeAPIToken(r.Context(), s.db, user.ID, id)
	if errors.Is(err, sql.ErrNoRows) {
		apiError(w, 404, "NOT_FOUND", "Token 不存在")
		return
	}
	if err != nil {
		apiError(w, 500, "INTERNAL_ERROR", "撤销 Token 失败")
		return
	}
	writeJSON(w, 200, map[string]bool{"revoked": true})
}
