package cpa

import (
	"context"
	"encoding/json"
	"net/http"
)

func (c *Client) FetchKeyPolicies(ctx context.Context) (json.RawMessage, int, error) {
	var report json.RawMessage
	status, _, err := c.doManagementJSONRequest(ctx, "/v0/management/key-policies/report", &report, "key policies")
	return report, status, err
}

func (c *Client) SyncKeyPolicies(ctx context.Context, payload any) (int, error) {
	status, _, err := c.doManagementJSONRequestWithBody(ctx, http.MethodPut, "/v0/management/key-policies/sync", payload, nil, "key policy sync")
	return status, err
}
