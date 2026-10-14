package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"keyhub/internal/config"
	"keyhub/internal/database"
)

func TestAPIKeyPlaintextRequiresAdminRole(t *testing.T) {
	server := &Server{config: config.Config{AuthEnabled: true}}
	request := httptest.NewRequest(http.MethodPost, "/api/keys/12/plaintext", nil)
	request = request.WithContext(context.WithValue(request.Context(), adminUserContextKey, &database.AdminUser{
		Username: "member",
		Role:     "user",
	}))
	response := httptest.NewRecorder()

	server.handleAPIKeyAction(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestAPIKeyPlaintextOnlyAcceptsPOST(t *testing.T) {
	server := &Server{config: config.Config{AuthEnabled: true}}
	request := httptest.NewRequest(http.MethodGet, "/api/keys/12/plaintext", nil)
	request = request.WithContext(context.WithValue(request.Context(), adminUserContextKey, &database.AdminUser{
		Username: "admin",
		Role:     "admin",
	}))
	response := httptest.NewRecorder()

	server.handleAPIKeyAction(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

func TestAPIKeyRetryPushRequiresAdminRole(t *testing.T) {
	server := &Server{config: config.Config{AuthEnabled: true}}
	request := httptest.NewRequest(http.MethodPost, "/api/keys/12/retry-push", nil)
	request = request.WithContext(context.WithValue(request.Context(), adminUserContextKey, &database.AdminUser{
		Username: "member",
		Role:     "user",
	}))
	response := httptest.NewRecorder()

	server.handleAPIKeyAction(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestAPIKeyRetryPushOnlyAcceptsPOST(t *testing.T) {
	server := &Server{config: config.Config{AuthEnabled: true}}
	request := httptest.NewRequest(http.MethodGet, "/api/keys/12/retry-push", nil)
	request = request.WithContext(context.WithValue(request.Context(), adminUserContextKey, &database.AdminUser{
		Username: "admin",
		Role:     "admin",
	}))
	response := httptest.NewRecorder()

	server.handleAPIKeyAction(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}
