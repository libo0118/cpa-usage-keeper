package test

import (
	. "cpa-usage-keeper/internal/api"
	servicedto "cpa-usage-keeper/internal/service/dto"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQoderCreditsRequestDetailsAPI(t *testing.T) {
	credits := `{"credits":0.11793750000000001,"billable":false}`
	provider := &usageEventsStub{events: []servicedto.UsageEventRecord{{ID: 1, QoderCredits: &credits}, {ID: 2}}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/usage/events?cursor_mode=true&source=qoder&auth_type=1", nil))
	body := response.Body.String()
	if response.Code != 200 || !strings.Contains(body, `"qoder_credits":{"credits":0.11793750000000001,"billable":false}`) || strings.Count(body, `"qoder_credits"`) != 1 {
		t.Fatalf("billing contract failed: %s", body)
	}
}

func TestWorkBuddyCreditsRequestDetailsAPI(t *testing.T) {
	credits := `{"credits":0.11793750000000001}`
	provider := &usageEventsStub{events: []servicedto.UsageEventRecord{{ID: 1, WorkBuddyCredits: &credits}, {ID: 2}}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/usage/events?cursor_mode=true&source=workbuddy&auth_type=1", nil))
	body := response.Body.String()
	if response.Code != 200 || !strings.Contains(body, `"workbuddy_credits":{"credits":0.11793750000000001}`) || strings.Count(body, `"workbuddy_credits"`) != 1 {
		t.Fatalf("billing contract failed: %s", body)
	}
}
