package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"keyhub/internal/database"
)

func TestModelGapIDFromPath(t *testing.T) {
	tests := []struct {
		path string
		want int64
		ok   bool
	}{
		{path: "/api/admin/model-gaps/12", want: 12, ok: true},
		{path: "/api/admin/model-gaps/12/", want: 12, ok: true},
		{path: "/api/admin/model-gaps/0", ok: false},
		{path: "/api/admin/model-gaps/not-an-id", ok: false},
		{path: "/api/admin/model-gaps/12/extra", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, ok := modelGapIDFromPath(tt.path)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("modelGapIDFromPath(%q) = (%d, %v), want (%d, %v)", tt.path, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestAdminModelGapsRejectsInvalidInputBeforeDatabaseAccess(t *testing.T) {
	server := &Server{}
	request := httptest.NewRequest(http.MethodPost, "/api/admin/model-gaps", strings.NewReader(
		`{"providerType":"Anthropic","modelName":"claude-test","rpmGap":-1,"tpmGap":1000}`,
	))
	response := httptest.NewRecorder()

	server.handleAdminModelGaps(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "rpmGap must not be negative") {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}

func TestRequireRootRejectsAdminAndAllowsRoot(t *testing.T) {
	server := &Server{}
	server.config.AuthEnabled = true

	for _, tt := range []struct {
		role string
		want bool
	}{
		{role: "admin", want: false},
		{role: "root", want: true},
	} {
		t.Run(tt.role, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/admin/model-gaps", nil)
			request = request.WithContext(context.WithValue(request.Context(), adminUserContextKey, &database.AdminUser{Role: tt.role}))
			response := httptest.NewRecorder()
			if got := server.requireRoot(response, request); got != tt.want {
				t.Fatalf("requireRoot(%q) = %v, want %v", tt.role, got, tt.want)
			}
			if !tt.want && response.Code != http.StatusForbidden {
				t.Fatalf("expected status %d, got %d", http.StatusForbidden, response.Code)
			}
		})
	}
}
