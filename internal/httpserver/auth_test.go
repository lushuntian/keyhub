package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"keyhub/internal/database"
)

func TestAuthUserResponseDoesNotExposeAggregationTargets(t *testing.T) {
	response := authUserResponse(&database.AdminUser{
		ID:          7,
		Username:    "alice",
		DisplayName: "Alice",
		Role:        "user",
		AggregationTargets: []database.AdminUserAggregationTarget{
			{TargetCode: "private-target"},
		},
	})

	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal auth user response: %v", err)
	}
	text := string(payload)
	for _, privateValue := range []string{"aggregationTarget", "private-target"} {
		if strings.Contains(text, privateValue) {
			t.Fatalf("auth response leaked %q: %s", privateValue, text)
		}
	}
}

func TestAdminUserActionRejectsDisablingCurrentUser(t *testing.T) {
	server := &Server{}
	request := httptest.NewRequest(http.MethodPut, "/api/admin/users/7/status", strings.NewReader(`{"disabled":true}`))
	request = request.WithContext(context.WithValue(request.Context(), adminUserContextKey, &database.AdminUser{ID: 7}))
	response := httptest.NewRecorder()

	server.handleAdminUserAction(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "cannot disable the current user") {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}

func TestAdminUserActionRejectsShortResetPassword(t *testing.T) {
	server := &Server{}
	request := httptest.NewRequest(http.MethodPut, "/api/admin/users/7/password", strings.NewReader(`{"password":"short"}`))
	response := httptest.NewRecorder()

	server.handleAdminUserAction(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "password must be at least 8 characters") {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}

func TestAdminUserActionRejectsUnknownAction(t *testing.T) {
	server := &Server{}
	request := httptest.NewRequest(http.MethodPut, "/api/admin/users/7/unknown", nil)
	response := httptest.NewRecorder()

	server.handleAdminUserAction(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNotFound, response.Code, response.Body.String())
	}
}

func TestAccountManagerHasOnlyUserManagementRole(t *testing.T) {
	if !canManageUsers(database.AdminUserRoleAccountManager) {
		t.Fatal("account manager should be allowed to manage users")
	}
	if isAdminRole(database.AdminUserRoleAccountManager) {
		t.Fatal("account manager must not receive full administrator permissions")
	}
}

func TestAdminUserActionRejectsInvalidRole(t *testing.T) {
	server := &Server{}
	request := httptest.NewRequest(http.MethodPut, "/api/admin/users/7/role", strings.NewReader(`{"role":"admin"}`))
	response := httptest.NewRecorder()

	server.handleAdminUserAction(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), database.ErrInvalidAdminUserRole.Error()) {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}

func TestAccountManagerUserActionRejectsNonStatusAction(t *testing.T) {
	server := &Server{}
	request := httptest.NewRequest(http.MethodPut, "/api/admin/users/7/password", strings.NewReader(`{"password":"new-password"}`))
	request = request.WithContext(context.WithValue(request.Context(), adminUserContextKey, &database.AdminUser{
		ID:   6,
		Role: database.AdminUserRoleAccountManager,
	}))
	response := httptest.NewRecorder()

	server.handleAdminUserAction(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d: %s", http.StatusForbidden, response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "can only update managed user status") {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}
