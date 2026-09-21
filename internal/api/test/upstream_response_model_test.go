package test

import (
	. "cpa-usage-keeper/internal/api"
	servicedto "cpa-usage-keeper/internal/service/dto"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpstreamModelRequestEventsAPI(t *testing.T) {
	provider := &usageEventsStub{events: []servicedto.UsageEventRecord{{ID: 1, Model: "sent", ModelAlias: "requested", UpstreamResponseModel: "reported"}, {ID: 2, Model: "old"}}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "")
	for _, query := range []string{"cursor_mode=true", "page=1&page_size=50", "cursor_mode=true&source=codex&auth_type=1"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/usage/events?range=24h&"+query, nil))
		body := response.Body.String()
		if response.Code != 200 || !strings.Contains(body, `"upstream_response_model":"reported"`) || strings.Count(body, `"upstream_response_model"`) != 1 {
			t.Fatalf("model metadata API contract: %s", body)
		}
	}
}
