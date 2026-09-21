package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestKeyPoliciesRequireAdminSession(t *testing.T) {
	router := NewRouter(nil, nil, nil, nil, AuthConfig{Enabled: true, LoginPassword: "test-password", BasePath: "/keeper"}, nil, "/keeper")
	request := httptest.NewRequest(http.MethodGet, "/keeper/api/v1/key-policies", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated report status = %d: %s", response.Code, response.Body.String())
	}
	router = NewRouter(nil, nil, nil, nil, AuthConfig{}, nil, "/keeper")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured report status = %d", response.Code)
	}
}
