package httpserver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"regexp"
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
		if !s.config.AuthEnabled {
			next(w, r)
			return
		}
		user := currentAdminUser(r)
		if user == nil || !isAdminRole(user.Role) {
			writeError(w, http.StatusForbidden, "admin role required")
			return
		}
		next(w, r)
	})
}

func isAdminRole(role string) bool {
	role = strings.ToLower(strings.TrimSpace(role))
	return role == "admin" || role == "root"
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
	writeJSON(w, http.StatusOK, s.authState(true, user))
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
	writeJSON(w, http.StatusOK, s.authState(true, user))
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
