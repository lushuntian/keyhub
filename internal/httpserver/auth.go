package httpserver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"keyhub/internal/database"
	"keyhub/internal/security"
)

const sessionCookieName = "keyhub_session"

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,64}$`)

type authContextKey string

const adminUserContextKey authContextKey = "admin_user"

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type registerRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
}

func (s *Server) protect(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.config.AuthEnabled {
			next(w, r)
			return
		}
		user, err := s.authenticateRequest(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		ctx := context.WithValue(r.Context(), adminUserContextKey, user)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) protectAdmin(next http.HandlerFunc) http.HandlerFunc {
	return s.protect(func(w http.ResponseWriter, r *http.Request) {
		if !s.requireAdmin(w, r) {
			return
		}
		next(w, r)
	})
}

func (s *Server) protectRoot(next http.HandlerFunc) http.HandlerFunc {
	return s.protect(func(w http.ResponseWriter, r *http.Request) {
		if !s.requireRoot(w, r) {
			return
		}
		next(w, r)
	})
}

func (s *Server) protectUserManager(next http.HandlerFunc) http.HandlerFunc {
	return s.protect(func(w http.ResponseWriter, r *http.Request) {
		if !s.requireUserManager(w, r) {
			return
		}
		next(w, r)
	})
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !s.config.AuthEnabled {
		return true
	}
	user := currentAdminUser(r)
	if user == nil || !isAdminRole(user.Role) {
		writeError(w, http.StatusForbidden, "admin role required")
		return false
	}
	return true
}

func (s *Server) requireRoot(w http.ResponseWriter, r *http.Request) bool {
	if !s.config.AuthEnabled {
		return true
	}
	user := currentAdminUser(r)
	if user == nil || !isRootRole(user.Role) {
		writeError(w, http.StatusForbidden, "root role required")
		return false
	}
	return true
}

func (s *Server) requireUserManager(w http.ResponseWriter, r *http.Request) bool {
	if !s.config.AuthEnabled {
		return true
	}
	user := currentAdminUser(r)
	if user == nil || !canManageUsers(user.Role) {
		writeError(w, http.StatusForbidden, "user management role required")
		return false
	}
	return true
}

func isAdminRole(role string) bool {
	role = strings.ToLower(strings.TrimSpace(role))
	return role == "admin" || role == "root"
}

func isRootRole(role string) bool {
	return strings.EqualFold(strings.TrimSpace(role), "root")
}

func isAccountManagerRole(role string) bool {
	return strings.EqualFold(strings.TrimSpace(role), database.AdminUserRoleAccountManager)
}

func canManageUsers(role string) bool {
	return isAdminRole(role) || isAccountManagerRole(role)
}

func (s *Server) authenticateRequest(r *http.Request) (*database.AdminUser, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return nil, errors.New("session cookie missing")
	}
	return database.LoadAdminSession(r.Context(), s.db, security.SessionTokenHash(cookie.Value))
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.config.AuthEnabled {
		writeJSON(w, http.StatusOK, s.authState(true, localUser()))
		return
	}

	var request loginRequest
	if err := readJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	user, err := database.AuthenticateAdmin(r.Context(), s.db, request.Username, request.Password)
	if err != nil {
		_ = database.InsertAuditLog(r.Context(), s.db, strings.TrimSpace(request.Username), "auth.login_failed", "admin_users", nil, map[string]any{
			"ip": clientIP(r),
		})
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	_ = database.MarkAdminLogin(r.Context(), s.db, user.ID)
	_ = database.InsertAuditLog(r.Context(), s.db, user.Username, "auth.login", "admin_users", &user.ID, map[string]any{
		"ip": clientIP(r),
	})
	s.writeAuthenticatedSession(w, r, user)
}

func (s *Server) handleAuthRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.config.AuthEnabled {
		writeError(w, http.StatusBadRequest, "auth is disabled")
		return
	}
	if !s.config.RegistrationEnabled {
		writeError(w, http.StatusForbidden, "registration is disabled")
		return
	}

	var request registerRequest
	if err := readJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateRegisterRequest(request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	user, err := database.CreateAdminUser(r.Context(), s.db, request.Username, request.Password, request.DisplayName)
	if errors.Is(err, database.ErrAdminUsernameExists) {
		writeError(w, http.StatusConflict, "username already exists")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	_ = database.MarkAdminLogin(r.Context(), s.db, user.ID)
	_ = database.InsertAuditLog(r.Context(), s.db, user.Username, "auth.register", "admin_users", &user.ID, map[string]any{
		"ip":   clientIP(r),
		"role": user.Role,
	})
	s.writeAuthenticatedSession(w, r, user)
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		_ = database.DeleteAdminSession(r.Context(), s.db, security.SessionTokenHash(cookie.Value))
	}
	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.config.AuthEnabled {
		writeJSON(w, http.StatusOK, s.authState(true, localUser()))
		return
	}
	user, err := s.authenticateRequest(r)
	if err != nil {
		writeJSON(w, http.StatusOK, s.authState(false, nil))
		return
	}
	writeJSON(w, http.StatusOK, s.authState(true, authUserResponse(user)))
}

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var managerUserID *int64
		if actor := currentAdminUser(r); actor != nil && isAccountManagerRole(actor.Role) {
			managerUserID = &actor.ID
		}
		users, err := database.ListAdminUsersForAccountManager(r.Context(), s.db, managerUserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": users})
	case http.MethodPost:
		var request registerRequest
		if err := readJSON(w, r, &request); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := validateRegisterRequest(request); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		var user *database.AdminUser
		var err error
		actor := currentAdminUser(r)
		if actor != nil && isAccountManagerRole(actor.Role) {
			user, err = database.CreateAdminUserForAccountManager(
				r.Context(),
				s.db,
				request.Username,
				request.Password,
				request.DisplayName,
				actor.ID,
			)
		} else {
			user, err = database.CreateAdminUser(r.Context(), s.db, request.Username, request.Password, request.DisplayName)
		}
		if errors.Is(err, database.ErrAdminUsernameExists) {
			writeError(w, http.StatusConflict, "username already exists")
			return
		}
		if errors.Is(err, database.ErrInvalidAccountManager) {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		_ = database.InsertAuditLog(r.Context(), s.db, actorFromRequest(r), "admin_user.create", "admin_users", &user.ID, map[string]any{
			"username": user.Username,
			"role":     user.Role,
		})
		writeJSON(w, http.StatusCreated, map[string]any{"user": user})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

type adminUserAggregationTargetsRequest struct {
	TargetCodes []string                            `json:"targetCodes,omitempty"`
	Targets     []adminUserAggregationTargetRequest `json:"targets,omitempty"`
}

type adminUserAggregationTargetRequest struct {
	TargetCode string `json:"targetCode"`
}

type adminUserStatusRequest struct {
	Disabled bool `json:"disabled"`
}

type adminUserPasswordRequest struct {
	Password string `json:"password"`
}

type adminUserRoleRequest struct {
	Role string `json:"role"`
}

func (s *Server) handleAdminUserAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 5 || parts[0] != "api" || parts[1] != "admin" || parts[2] != "users" {
		writeError(w, http.StatusNotFound, "api route not found")
		return
	}
	userID, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || userID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	action := parts[4]
	if actor := currentAdminUser(r); actor != nil && isAccountManagerRole(actor.Role) && action != "status" {
		writeError(w, http.StatusForbidden, "account managers can only update managed user status")
		return
	}

	switch action {
	case "aggregation-targets":
		s.handleAdminUserAggregationTargetsUpdate(w, r, userID)
	case "status":
		s.handleAdminUserStatusUpdate(w, r, userID)
	case "password":
		s.handleAdminUserPasswordReset(w, r, userID)
	case "role":
		s.handleAdminUserRoleUpdate(w, r, userID)
	default:
		writeError(w, http.StatusNotFound, "api route not found")
	}
}

func (s *Server) handleAdminUserAggregationTargetsUpdate(w http.ResponseWriter, r *http.Request, userID int64) {
	var request adminUserAggregationTargetsRequest
	if err := readJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	targets := make([]database.AdminUserAggregationTarget, 0)
	if request.Targets != nil {
		for _, target := range request.Targets {
			targets = append(targets, database.AdminUserAggregationTarget{
				TargetCode: target.TargetCode,
			})
		}
	} else {
		for _, targetCode := range request.TargetCodes {
			targets = append(targets, database.AdminUserAggregationTarget{TargetCode: targetCode})
		}
	}
	if err := database.ReplaceAdminUserAggregationTargets(r.Context(), s.db, userID, targets, actorFromRequest(r)); err != nil {
		switch {
		case errors.Is(err, database.ErrInheritedAggregationTargets):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, database.ErrAdminUserNotFound), errors.Is(err, database.ErrAggregationTargetNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	_ = database.InsertAuditLog(r.Context(), s.db, actorFromRequest(r), "admin_user.aggregation_targets.update", "admin_users", &userID, map[string]any{
		"targets": targets,
	})
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "targets": targets})
}

func (s *Server) handleAdminUserStatusUpdate(w http.ResponseWriter, r *http.Request, userID int64) {
	var request adminUserStatusRequest
	if err := readJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if request.Disabled {
		if actor := currentAdminUser(r); actor != nil && actor.ID == userID {
			writeError(w, http.StatusBadRequest, "cannot disable the current user")
			return
		}
	}
	actor := currentAdminUser(r)
	var err error
	if actor != nil && isAccountManagerRole(actor.Role) {
		err = database.SetManagedAdminUserDisabled(r.Context(), s.db, userID, actor.ID, request.Disabled)
	} else {
		err = database.SetAdminUserDisabled(r.Context(), s.db, userID, request.Disabled)
	}
	if err != nil {
		if errors.Is(err, database.ErrAdminUserNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
		} else if errors.Is(err, database.ErrAdminUserNotManaged) {
			writeError(w, http.StatusForbidden, err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	action := "admin_user.enable"
	if request.Disabled {
		action = "admin_user.disable"
	}
	_ = database.InsertAuditLog(r.Context(), s.db, actorFromRequest(r), action, "admin_users", &userID, map[string]any{
		"disabled": request.Disabled,
	})
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "disabled": request.Disabled})
}

func (s *Server) handleAdminUserPasswordReset(w http.ResponseWriter, r *http.Request, userID int64) {
	var request adminUserPasswordRequest
	if err := readJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len([]rune(request.Password)) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if err := database.ResetAdminUserPassword(r.Context(), s.db, userID, request.Password); err != nil {
		if errors.Is(err, database.ErrAdminUserNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	_ = database.InsertAuditLog(r.Context(), s.db, actorFromRequest(r), "admin_user.password.reset", "admin_users", &userID, nil)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleAdminUserRoleUpdate(w http.ResponseWriter, r *http.Request, userID int64) {
	var request adminUserRoleRequest
	if err := readJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	request.Role = strings.ToLower(strings.TrimSpace(request.Role))
	if request.Role != "user" && request.Role != database.AdminUserRoleAccountManager {
		writeError(w, http.StatusBadRequest, database.ErrInvalidAdminUserRole.Error())
		return
	}
	if err := database.SetAdminUserRole(r.Context(), s.db, userID, request.Role); err != nil {
		switch {
		case errors.Is(err, database.ErrAdminUserNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, database.ErrInvalidAdminUserRole),
			errors.Is(err, database.ErrProtectedAdminUserRole),
			errors.Is(err, database.ErrNestedAccountManager):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	_ = database.InsertAuditLog(r.Context(), s.db, actorFromRequest(r), "admin_user.role.update", "admin_users", &userID, map[string]any{
		"role": request.Role,
	})
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "role": request.Role})
}

func (s *Server) writeAuthenticatedSession(w http.ResponseWriter, r *http.Request, user *database.AdminUser) {
	token, tokenHash, err := security.NewSessionToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	expiresAt := time.Now().Add(s.config.SessionTTL)
	if err := database.CreateAdminSession(r.Context(), s.db, tokenHash, user.ID, expiresAt, r.UserAgent(), clientIP(r)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.setSessionCookie(w, token, expiresAt)
	writeJSON(w, http.StatusOK, s.authState(true, authUserResponse(user)))
}

func (s *Server) authState(authenticated bool, user any) map[string]any {
	state := map[string]any{
		"authEnabled":         s.config.AuthEnabled,
		"registrationEnabled": s.config.AuthEnabled && s.config.RegistrationEnabled,
		"authenticated":       authenticated,
	}
	if user != nil {
		state["user"] = user
	}
	return state
}

func validateRegisterRequest(request registerRequest) error {
	username := strings.TrimSpace(request.Username)
	if !usernamePattern.MatchString(username) {
		return errors.New("username must be 3-64 characters and use only letters, numbers, dots, underscores, or hyphens")
	}
	if len([]rune(request.Password)) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	if len([]rune(strings.TrimSpace(request.DisplayName))) > 128 {
		return errors.New("display name must be 128 characters or fewer")
	}
	return nil
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.config.CookieSecure,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.config.CookieSecure,
	})
}

func currentAdminUser(r *http.Request) *database.AdminUser {
	user, _ := r.Context().Value(adminUserContextKey).(*database.AdminUser)
	return user
}

func (s *Server) isRegularUserRequest(r *http.Request) bool {
	if !s.config.AuthEnabled {
		return false
	}
	user := currentAdminUser(r)
	return user != nil && !isAdminRole(user.Role)
}

func authUserResponse(user *database.AdminUser) map[string]any {
	if user == nil {
		return nil
	}
	return map[string]any{
		"id":          user.ID,
		"username":    user.Username,
		"displayName": user.DisplayName,
		"role":        user.Role,
	}
}

func (s *Server) keyOwnerFilter(r *http.Request) string {
	if !s.config.AuthEnabled {
		return ""
	}
	user := currentAdminUser(r)
	if user == nil || isAdminRole(user.Role) {
		return ""
	}
	return strings.TrimSpace(user.Username)
}

func (s *Server) keyOwnerFilters(r *http.Request) ([]string, error) {
	if !s.config.AuthEnabled {
		return nil, nil
	}
	user := currentAdminUser(r)
	if user == nil || isAdminRole(user.Role) {
		return nil, nil
	}
	if isAccountManagerRole(user.Role) {
		return database.ListAccountManagerOwnerUsernames(r.Context(), s.db, user.ID)
	}
	username := strings.TrimSpace(user.Username)
	if username == "" {
		return []string{}, nil
	}
	return []string{username}, nil
}

func localUser() map[string]any {
	return map[string]any{
		"id":          0,
		"username":    "local",
		"displayName": "Local",
		"role":        "root",
	}
}

func clientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
